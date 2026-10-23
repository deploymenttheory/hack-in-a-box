package mentors

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func (s Service) Read(ctx context.Context, actor, eventID uuid.UUID) (Page, error) {
	var result Page
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		var readErr error
		result, readErr = s.ReadInTransaction(ctx, tx, actor, eventID)
		return readErr
	})
	return result, err
}

// ReadInTransaction preserves the roster's current-person/role and restricted
// audience policy while composing metadata inside the caller's event lock.
func (s Service) ReadInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
) (Page, error) {
	var result Page
	q := dbgen.New(tx)
	err := func() error {
		row, err := q.LockEvent(ctx, eventID)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock mentor roster: %w", err)
		}
		person, err := q.Person(ctx, actor)
		if err != nil || !person.Active {
			return access.ErrForbidden
		}
		roles, err := q.ActiveRoles(ctx, dbgen.ActiveRolesParams{EventID: eventID, PersonID: actor})
		if err != nil {
			return fmt.Errorf("read current mentor roles: %w", err)
		}
		manager := slices.Contains(roles, "organiser")
		own, err := ownTeam(ctx, q, eventID, actor)
		if err != nil {
			return err
		}
		if !manager && !slices.Contains(roles, "mentor") && own == uuid.Nil {
			return access.ErrForbidden
		}
		result, _, err = load(ctx, q, row)
		if err != nil {
			return err
		}
		result.CanManage = manager
		if !manager {
			result = restricted(result, actor, own, slices.Contains(roles, "mentor"))
		}
		return nil
	}()
	if err != nil {
		return result, fmt.Errorf("read mentor coverage: %w", err)
	}
	return result, nil
}

func ownTeam(ctx context.Context, q *dbgen.Queries, eventID, actor uuid.UUID) (uuid.UUID, error) {
	claim, err := q.CompetitionState(
		ctx,
		dbgen.CompetitionStateParams{EventID: eventID, PersonID: actor},
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("read mentor roster competitor: %w", err)
	}
	if claim != "confirmed" {
		return uuid.Nil, nil
	}
	member, err := q.TeamAssignment(
		ctx,
		dbgen.TeamAssignmentParams{EventID: eventID, PersonID: actor, Snapshot: "published"},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, nil
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("read published competitor team: %w", err)
	}
	return member.TeamID, nil
}

func restricted(page Page, actor, own uuid.UUID, mentor bool) Page {
	conflicts := page.Conflicts
	page.Draft = []Assignment{}
	page.Conflicts = []Conflict{}
	page.Gaps = []Gap{}
	page.PublishedGaps = []Gap{}
	teams := map[uuid.UUID]bool{}
	people := map[uuid.UUID]bool{}
	visible := []Assignment{}
	active := map[uuid.UUID]bool{}
	for _, p := range page.Profiles {
		active[p.PersonID] = p.Active
	}
	for _, a := range page.Published {
		index := slices.IndexFunc(page.Teams, func(t Team) bool { return t.ID == a.TeamID })
		if index < 0 || conflicted(conflicts, page.Teams[index], a.PersonID) {
			continue
		}
		if active[a.PersonID] && (a.TeamID == own || (mentor && a.PersonID == actor)) {
			visible = append(visible, a)
			teams[a.TeamID] = true
			people[a.PersonID] = true
		}
	}
	page.Published = visible
	page.Teams = slices.DeleteFunc(page.Teams, func(t Team) bool { return !teams[t.ID] })
	page.Profiles = slices.DeleteFunc(
		page.Profiles,
		func(p Profile) bool { return !people[p.PersonID] },
	)
	for i := range page.Profiles {
		if page.Profiles[i].PersonID != actor {
			page.Profiles[i].Availability = []Window{}
			page.Profiles[i].MaximumTeams = nil
		}
	}
	return page
}

// Propose shares the event lock and current revision with every registration,
// role and assignment writer. Competitor freeze never blocks mentor planning.
func (s Service) Propose(
	ctx context.Context,
	actor, eventID uuid.UUID,
	revision int64,
) (Proposal, error) {
	var result Proposal
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, eventID)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock mentor proposal: %w", err)
		}
		if err = (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.EventEdit,
		); err != nil {
			return fmt.Errorf("authorize mentor proposal: %w", err)
		}
		if row.Revision != revision {
			return access.ErrConflict
		}
		_, snapshot, err := load(ctx, q, row)
		if err != nil {
			return err
		}
		if !validWindow(snapshot.Live) {
			return access.ErrInvalid
		}
		result = Suggest(snapshot)
		result.Revision = row.Revision
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("suggest mentor coverage: %w", err)
	}
	return result, nil
}
