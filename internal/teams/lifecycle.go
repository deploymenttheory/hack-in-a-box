package teams

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

func publish(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	row dbgen.Event,
	freeze bool,
	now time.Time,
) error {
	if err := verifyAllocationLocks(ctx, q, row.ID, uuid.Nil); err != nil {
		return err
	}
	records, err := q.HackTeams(ctx, row.ID)
	if err != nil {
		return fmt.Errorf("read publication teams: %w", err)
	}
	members, err := q.HackTeamMembers(ctx, row.ID)
	if err != nil {
		return fmt.Errorf("read publication membership: %w", err)
	}
	changed := publicationChanged(records, members)
	assigned := map[uuid.UUID]bool{}
	total := 0
	var retire []uuid.UUID
	for _, t := range records {
		if t.State == "disbanded" {
			continue
		}
		count := 0
		hasCaptain := false
		for _, m := range members {
			if m.TeamID != t.ID || m.Snapshot != "draft" {
				continue
			}
			if err := confirmed(ctx, q, row.ID, m.PersonID); err != nil {
				return err
			}
			assigned[m.PersonID] = true
			count++
			if t.CaptainID != nil && *t.CaptainID == m.PersonID {
				hasCaptain = true
			}
		}
		if count == 0 && !t.AllocationLocked {
			retire = append(retire, t.ID)
			continue
		}
		total++
		if count < 3 || !hasCaptain || (t.Capacity.Valid && count > int(t.Capacity.Int32)) {
			return ErrUnready
		}
	}
	if total == 0 {
		return ErrUnready
	}
	if freeze {
		count, err := q.ConfirmedAttendees(ctx, row.ID)
		if err != nil {
			return fmt.Errorf("count freezing competitors: %w", err)
		}
		if int64(len(assigned)) != count {
			return ErrUnready
		}
	}
	for _, id := range retire {
		if err := disband(ctx, q, row.ID, id); err != nil {
			return err
		}
	}
	if err := q.DeletePublishedTeamMembers(ctx, row.ID); err != nil {
		return fmt.Errorf("replace previous allocations: %w", err)
	}
	if err := q.CopyPublishedTeamMembers(ctx, row.ID); err != nil {
		return fmt.Errorf("publish allocated members: %w", err)
	}
	if err := q.PublishHackTeams(ctx, row.ID); err != nil {
		return fmt.Errorf("publish team records: %w", err)
	}
	return finishPublication(ctx, tx, q, row, freeze, changed, now)
}

func finishPublication(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	row dbgen.Event,
	freeze, changed bool,
	now time.Time,
) error {
	if changed {
		if _, err := q.InsertOperation(ctx, dbgen.InsertOperationParams{
			EventID:      row.ID,
			OperationKey: fmt.Sprintf("teams-published:%d", row.Revision+1),
			Kind:         "teams.published",
			TargetID:     row.ID,
			DueAt:        now.UTC(),
			Approved:     true,
		}); err != nil {
			return fmt.Errorf("record published allocation change: %w", err)
		}
	}
	if freeze {
		if err := q.CloseTeamRequests(ctx, row.ID); err != nil {
			return fmt.Errorf("close pending joins at freeze: %w", err)
		}
		if err := q.FreezeHackTeams(ctx, row.ID); err != nil {
			return fmt.Errorf("freeze team records: %w", err)
		}
		if _, err := tx.Exec(
			ctx,
			"UPDATE events SET team_frozen_at=$2 WHERE id=$1",
			row.ID,
			now,
		); err != nil {
			return fmt.Errorf("record hard freeze: %w", err)
		}
	}
	return nil
}

func recordFacts(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, row dbgen.Event) error {
	records, err := q.HackTeams(ctx, row.ID)
	if err != nil {
		return fmt.Errorf("read team facts: %w", err)
	}
	members, err := q.HackTeamMembers(ctx, row.ID)
	if err != nil {
		return fmt.Errorf("read member facts: %w", err)
	}
	var assigned, total, frozen int32
	for _, t := range records {
		if t.State == "disbanded" {
			continue
		}
		total++
		if t.Frozen {
			frozen++
		}
		for _, m := range members {
			if m.TeamID == t.ID && m.Snapshot == "published" {
				assigned++
			}
		}
	}
	if err := event.RecordTeamFacts(ctx, tx, row.ID, assigned, total, frozen, nil); err != nil {
		return fmt.Errorf("supply lifecycle team facts: %w", err)
	}
	return nil
}

// Withdraw removes the competitor from both snapshots in registration's own
// transaction. It never moves anyone. Before freeze the organiser repairs small
// teams; after freeze depleted teams may continue with their remaining members.
func Withdraw(ctx context.Context, tx pgx.Tx, eventID, person uuid.UUID) error {
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("lock team withdrawal: %w", err)
	}
	if err := q.RemoveTeamMember(
		ctx,
		dbgen.RemoveTeamMemberParams{EventID: eventID, PersonID: person},
	); err != nil {
		return fmt.Errorf("remove withdrawn member: %w", err)
	}
	if err := q.ClearDepartingCaptain(
		ctx,
		dbgen.ClearDepartingCaptainParams{EventID: eventID, CaptainID: &person},
	); err != nil {
		return fmt.Errorf("clear departed captain: %w", err)
	}
	if err := q.DeclinePersonTeamRequests(
		ctx,
		dbgen.DeclinePersonTeamRequestsParams{EventID: eventID, PersonID: person},
	); err != nil {
		return fmt.Errorf("close withdrawn join requests: %w", err)
	}
	records, err := q.HackTeams(ctx, eventID)
	if err != nil {
		return fmt.Errorf("read depleted teams: %w", err)
	}
	members, err := q.HackTeamMembers(ctx, eventID)
	if err != nil {
		return fmt.Errorf("read remaining team members: %w", err)
	}
	for _, t := range records {
		empty := true
		for _, m := range members {
			if m.TeamID == t.ID {
				empty = false
				break
			}
		}
		if empty {
			if err := disband(ctx, q, eventID, t.ID); err != nil {
				return err
			}
		}
	}
	return recordFacts(ctx, tx, q, row)
}

// One event per changed publication, with the committed event revision in its
// unique key. Downstream communications/repository owners reconcile the latest
// published roster; drafts and repeated unchanged publication enqueue nothing.
func publicationChanged(groups []dbgen.HackTeam, members []dbgen.HackTeamMembersRow) bool {
	for _, group := range groups {
		if group.State == "draft" {
			return true
		}
		if (group.CaptainID == nil) != (group.PublishedCaptainID == nil) {
			return true
		}
		if group.CaptainID != nil && *group.CaptainID != *group.PublishedCaptainID {
			return true
		}
	}
	draft, published := map[uuid.UUID]uuid.UUID{}, map[uuid.UUID]uuid.UUID{}
	for _, member := range members {
		if member.Snapshot == "draft" {
			draft[member.PersonID] = member.TeamID
		} else {
			published[member.PersonID] = member.TeamID
		}
	}
	if len(draft) != len(published) {
		return true
	}
	for person, team := range draft {
		if published[person] != team {
			return true
		}
	}
	return false
}
