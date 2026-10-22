// Package teams owns draft allocations, published membership and the hard freeze.
package teams

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

var (
	ErrFrozen   = errors.New("team membership is frozen")
	ErrLocked   = errors.New("explicitly unlock the team before moving its members")
	ErrCapacity = errors.New("the team has reached its configured hard capacity")
	ErrUnready  = errors.New(
		"resolve missing captains, unassigned competitors and teams below three before publication or freeze",
	)
)

type Service struct {
	Pool *pgxpool.Pool
	Now  func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

type (
	Person struct {
		Preference string    `json:"preference,omitempty"`
		ID         uuid.UUID `json:"id"`
		Name       string    `json:"name"`
	}
	Team struct {
		ID               uuid.UUID  `json:"id"`
		Name             string     `json:"name"`
		State            string     `json:"state"`
		Captain          *uuid.UUID `json:"captain"`
		PublishedCaptain *uuid.UUID `json:"published_captain"`
		Code             *uuid.UUID `json:"invitation_code"`
		Draft            []Person   `json:"draft"`
		Published        []Person   `json:"published"`
		Frozen           bool       `json:"frozen"`
		AllocationLocked bool       `json:"allocation_locked"`
		Capacity         *int       `json:"capacity"`
	}
	Request struct {
		ID        uuid.UUID `json:"id"`
		TeamID    uuid.UUID `json:"team_id"`
		Person    Person    `json:"person"`
		Confirmed bool      `json:"confirmed"`
	}
	Page struct {
		CanCreate bool      `json:"can_create"`
		Revision  int64     `json:"revision"`
		CanManage bool      `json:"can_manage"`
		Frozen    bool      `json:"frozen"`
		FreezeAt  time.Time `json:"freeze_at"`
		Teams     []Team    `json:"teams"`
		People    []Person  `json:"people"`
		Requests  []Request `json:"requests"`
	}
	Command struct {
		Action    string    `json:"action"`
		TeamID    uuid.UUID `json:"team_id"`
		PersonID  uuid.UUID `json:"person_id"`
		RequestID uuid.UUID `json:"request_id"`
		Code      uuid.UUID `json:"invitation_code"`
		Name      string    `json:"name"`
		Reason    string    `json:"reason"`
		Capacity  *int      `json:"capacity"`
	}
)

func config(row dbgen.Event) (event.Configuration, error) {
	c := event.DefaultConfiguration()
	if err := json.Unmarshal(row.Configuration, &c); err != nil {
		return c, fmt.Errorf("decode team dates: %w", err)
	}
	return c, nil
}

func frozen(row dbgen.Event, c event.Configuration, now time.Time) bool {
	return row.TeamFrozenAt.Valid || !now.Before(c.TeamFreeze)
}

func confirmed(ctx context.Context, q *dbgen.Queries, eventID, person uuid.UUID) error {
	p, err := q.Person(ctx, person)
	if err != nil || !p.Active {
		return access.ErrForbidden
	}
	state, err := q.CompetitionState(
		ctx,
		dbgen.CompetitionStateParams{EventID: eventID, PersonID: person},
	)
	if err != nil {
		return fmt.Errorf("check team place: %w", err)
	}
	if state != "confirmed" {
		return access.ErrForbidden
	}
	return nil
}

func (s Service) Read(ctx context.Context, actor, eventID uuid.UUID) (Page, error) {
	var result Page
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return access.ErrNotFound
		}
		result, err = s.page(ctx, q, actor, row)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("read teams: %w", err)
	}
	return result, nil
}

func (s Service) page(
	ctx context.Context,
	q *dbgen.Queries,
	actor uuid.UUID,
	row dbgen.Event,
) (Page, error) {
	result := Page{
		Revision: row.Revision,
		Teams:    []Team{},
		People:   []Person{},
		Requests: []Request{},
	}
	p, err := q.Person(ctx, actor)
	if err != nil || !p.Active {
		return result, access.ErrForbidden
	}
	policy := access.EventPolicy{Queries: q}
	manageErr := policy.Authorize(ctx, actor, row.ID, row.ID, access.StaffRead)
	result.CanManage = manageErr == nil
	if manageErr != nil && !errors.Is(manageErr, access.ErrForbidden) {
		return result, fmt.Errorf("check team management: %w", manageErr)
	}
	claim, err := q.CompetitionState(
		ctx,
		dbgen.CompetitionStateParams{EventID: row.ID, PersonID: actor},
	)
	if err != nil {
		return result, fmt.Errorf("read team applicant: %w", err)
	}
	if !result.CanManage && !slices.Contains([]string{"confirmed", "waitlisted"}, claim) {
		return result, access.ErrForbidden
	}
	c, err := config(row)
	if err != nil {
		return result, err
	}
	result.CanCreate = result.CanManage || claim == "confirmed"
	result.Frozen = frozen(row, c, s.now())
	result.FreezeAt = c.TeamFreeze
	records, err := q.HackTeams(ctx, row.ID)
	if err != nil {
		return result, fmt.Errorf("list teams: %w", err)
	}
	members, err := q.HackTeamMembers(ctx, row.ID)
	if err != nil {
		return result, fmt.Errorf("list team memberships: %w", err)
	}
	visible := map[uuid.UUID]bool{}
	captains := map[uuid.UUID]bool{}
	for _, r := range records {
		if r.CaptainID != nil && *r.CaptainID == actor {
			captains[r.ID] = true
			visible[r.ID] = true
		}
	}
	for _, m := range members {
		if m.PersonID == actor {
			visible[m.TeamID] = true
		}
	}
	for _, r := range records {
		if !result.CanManage && !visible[r.ID] {
			continue
		}
		t := teamView(r, members, result.CanManage || captains[r.ID])
		result.Teams = append(result.Teams, t)
	}
	requests, err := q.PendingTeamRequests(ctx, row.ID)
	if err != nil {
		return result, fmt.Errorf("read join requests: %w", err)
	}
	for _, r := range requests {
		if result.CanManage || captains[r.TeamID] || r.PersonID == actor {
			result.Requests = append(
				result.Requests,
				Request{
					ID:        r.ID,
					TeamID:    r.TeamID,
					Person:    Person{ID: r.PersonID, Name: r.DisplayName},
					Confirmed: r.RegistrationState == "confirmed",
				},
			)
		}
	}
	if result.CanManage {
		people, err := q.ConfirmedTeamPeople(ctx, row.ID)
		if err != nil {
			return result, fmt.Errorf("read placement pool: %w", err)
		}
		for _, p := range people {
			result.People = append(
				result.People,
				Person{ID: p.ID, Name: p.DisplayName, Preference: p.TeamPreference},
			)
		}
	}
	return result, nil
}

func (s Service) Act(
	ctx context.Context,
	actor, eventID uuid.UUID,
	revision int64,
	input Command,
) (Page, error) {
	var result Page
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return access.ErrNotFound
		}
		p, err := q.Person(ctx, actor)
		if err != nil || !p.Active {
			return access.ErrForbidden
		}
		if row.Revision != revision {
			return access.ErrConflict
		}
		if !row.WritesAllowed {
			return access.ErrForbidden
		}
		c, err := config(row)
		if err != nil {
			return err
		}
		if frozen(row, c, s.now()) && input.Action != "captain" {
			return ErrFrozen
		}
		manage := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.EventEdit,
		) == nil
		if !manage &&
			!slices.Contains([]string{"create", "join", "approve", "decline"}, input.Action) {
			return access.ErrForbidden
		}
		if manage && !slices.Contains([]string{"create", "join"}, input.Action) &&
			(strings.TrimSpace(input.Reason) == "" || len(input.Reason) > 2000) {
			return access.ErrInvalid
		}
		if !slices.Contains([]string{"registration", "preparation"}, row.Stage) &&
			input.Action != "captain" {
			return access.ErrInvalid
		}
		if err := s.command(ctx, tx, q, actor, row, manage, input); err != nil {
			return err
		}
		action := audit.TeamRebalanced
		if input.Action == "freeze" {
			action = audit.TeamFrozen
		}
		if err := audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &eventID,
				TargetID: eventID,
				Action:   action,
				Reason:   audit.UserRequest,
			},
		); err != nil {
			return fmt.Errorf("audit team change: %w", err)
		}
		if manage && input.Reason != "" {
			if _, err := q.RecordEventDecision(
				ctx,
				dbgen.RecordEventDecisionParams{
					EventID:  eventID,
					ActorID:  actor,
					Kind:     "team_" + input.Action,
					Reason:   input.Reason,
					Revision: row.Revision + 1,
				},
			); err != nil {
				return fmt.Errorf("record allocation reason: %w", err)
			}
		}
		row, err = q.BumpEventRevision(ctx, eventID)
		if err != nil {
			return fmt.Errorf("advance team revision: %w", err)
		}
		if err := recordFacts(ctx, tx, q, row); err != nil {
			return err
		}
		result, err = s.page(ctx, q, actor, row)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("change teams: %w", err)
	}
	return result, nil
}

func (s Service) command(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	actor uuid.UUID,
	row dbgen.Event,
	manage bool,
	input Command,
) error {
	switch input.Action {
	case "create":
		return create(ctx, q, actor, row.ID, manage, input)
	case "join":
		return join(ctx, q, actor, row.ID, input.Code)
	case "approve", "decline":
		return decideRequest(ctx, q, actor, row.ID, manage, input)
	case "move":
		return move(ctx, q, row.ID, input.TeamID, input.PersonID)
	case "lock", "unlock", "capacity":
		return allocationControl(ctx, q, row.ID, input)
	case "captain":
		return captain(ctx, q, row, input.TeamID, input.PersonID)
	case "publish":
		return publish(ctx, tx, q, row, false, s.now())
	case "freeze":
		if row.Stage != "preparation" {
			return access.ErrInvalid
		}
		return publish(ctx, tx, q, row, true, s.now())
	case "disband":
		members, err := q.HackTeamMembers(ctx, row.ID)
		if err != nil {
			return fmt.Errorf("read retirement publication: %w", err)
		}
		for _, member := range members {
			if member.TeamID == input.TeamID && member.Snapshot == "published" {
				return ErrUnready
			}
		}
		return disband(ctx, q, row.ID, input.TeamID)
	default:
		return access.ErrInvalid
	}
}

func create(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID uuid.UUID,
	manage bool,
	input Command,
) error {
	person := actor
	if manage && input.PersonID != uuid.Nil {
		person = input.PersonID
	}
	if strings.TrimSpace(input.Name) == "" || len(input.Name) > 160 {
		return access.ErrInvalid
	}
	if err := confirmed(ctx, q, eventID, person); err != nil {
		return err
	}
	if _, err := q.TeamAssignment(
		ctx,
		dbgen.TeamAssignmentParams{EventID: eventID, PersonID: person, Snapshot: "draft"},
	); err == nil {
		// Organisers can create a reviewed replacement group from an existing
		// unlocked draft. move below checks the source lock in this transaction.
		if !manage {
			return access.ErrConflict
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("check captain allocation: %w", err)
	}
	id := uuid.New()
	if _, err := q.CreateHackTeam(
		ctx,
		dbgen.CreateHackTeamParams{
			ID:        id,
			EventID:   eventID,
			Name:      strings.TrimSpace(input.Name),
			CaptainID: &person,
		},
	); err != nil {
		return fmt.Errorf("create team: %w", err)
	}
	if err := q.CreateCompetitionEntry(
		ctx,
		dbgen.CreateCompetitionEntryParams{EventID: eventID, TeamID: id},
	); err != nil {
		return fmt.Errorf("create single team entry: %w", err)
	}
	return move(ctx, q, eventID, id, person)
}

func move(ctx context.Context, q *dbgen.Queries, eventID, teamID, person uuid.UUID) error {
	if err := confirmed(ctx, q, eventID, person); err != nil {
		return err
	}
	t, err := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: eventID, ID: teamID})
	if err != nil || t.State == "disbanded" {
		return access.ErrNotFound
	}
	if err := checkAllocationMove(ctx, q, eventID, t, person); err != nil {
		return err
	}
	if err := q.SetDraftTeamMember(
		ctx,
		dbgen.SetDraftTeamMemberParams{EventID: eventID, TeamID: teamID, PersonID: person},
	); err != nil {
		return fmt.Errorf("assign draft member: %w", err)
	}
	return nil
}

func join(ctx context.Context, q *dbgen.Queries, actor, eventID, code uuid.UUID) error {
	claim, err := q.CompetitionState(
		ctx,
		dbgen.CompetitionStateParams{EventID: eventID, PersonID: actor},
	)
	if err != nil {
		return fmt.Errorf("check join registration: %w", err)
	}
	if !slices.Contains([]string{"confirmed", "waitlisted"}, claim) {
		return access.ErrForbidden
	}
	t, err := q.TeamByInvitation(
		ctx,
		dbgen.TeamByInvitationParams{EventID: eventID, InvitationCode: code},
	)
	if err != nil {
		return access.ErrNotFound
	}
	old, err := q.PendingTeamRequest(
		ctx,
		dbgen.PendingTeamRequestParams{EventID: eventID, PersonID: actor},
	)
	if err == nil {
		if old.TeamID == t.ID {
			return nil
		}
		return access.ErrConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("check previous join request: %w", err)
	}
	if err := q.NewTeamRequest(
		ctx,
		dbgen.NewTeamRequestParams{EventID: eventID, TeamID: t.ID, PersonID: actor},
	); err != nil {
		return fmt.Errorf("request team place: %w", err)
	}
	return nil
}

func decideRequest(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID uuid.UUID,
	manage bool,
	input Command,
) error {
	r, err := q.TeamRequest(ctx, dbgen.TeamRequestParams{EventID: eventID, ID: input.RequestID})
	if err != nil {
		return access.ErrNotFound
	}
	t, err := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: eventID, ID: r.TeamID})
	if err != nil {
		return access.ErrNotFound
	}
	if !manage && (t.CaptainID == nil || *t.CaptainID != actor) {
		return access.ErrForbidden
	}
	if r.State != "pending" {
		return access.ErrConflict
	}
	state := "declined"
	if input.Action == "approve" {
		if !manage {
			for _, snapshot := range []string{"draft", "published"} {
				if _, err := q.TeamAssignment(
					ctx,
					dbgen.TeamAssignmentParams{
						EventID:  eventID,
						PersonID: r.PersonID,
						Snapshot: snapshot,
					},
				); err == nil {
					return access.ErrForbidden
				} else if !errors.Is(
					err,
					pgx.ErrNoRows,
				) {
					return fmt.Errorf("check existing team: %w", err)
				}
			}
		}
		if err := move(ctx, q, eventID, r.TeamID, r.PersonID); err != nil {
			return err
		}
		state = "approved"
	}
	if err := q.ResolveTeamRequest(
		ctx,
		dbgen.ResolveTeamRequestParams{EventID: eventID, ID: r.ID, State: state},
	); err != nil {
		return fmt.Errorf("resolve join request: %w", err)
	}
	return nil
}

func captain(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	teamID, person uuid.UUID,
) error {
	if err := confirmed(ctx, q, row.ID, person); err != nil {
		return err
	}
	snapshot := "draft"
	if row.TeamFrozenAt.Valid {
		snapshot = "published"
	}
	member, err := q.TeamAssignment(
		ctx,
		dbgen.TeamAssignmentParams{EventID: row.ID, PersonID: person, Snapshot: snapshot},
	)
	if err != nil || member.TeamID != teamID {
		return access.ErrInvalid
	}
	if err := q.SetTeamCaptain(
		ctx,
		dbgen.SetTeamCaptainParams{EventID: row.ID, ID: teamID, CaptainID: &person},
	); err != nil {
		return fmt.Errorf("remediate captain: %w", err)
	}
	return nil
}

func disband(ctx context.Context, q *dbgen.Queries, eventID, teamID uuid.UUID) error {
	if _, err := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: eventID, ID: teamID}); err != nil {
		return access.ErrNotFound
	}

	members, err := q.HackTeamMembers(ctx, eventID)
	if err != nil {
		return fmt.Errorf("check disbanding team: %w", err)
	}
	for _, m := range members {
		if m.TeamID == teamID && m.Snapshot == "draft" {
			return ErrUnready
		}
	}
	if err := q.DisbandHackTeam(
		ctx,
		dbgen.DisbandHackTeamParams{EventID: eventID, ID: teamID},
	); err != nil {
		return fmt.Errorf("disband team: %w", err)
	}
	if err := q.RetireCompetitionEntry(
		ctx,
		dbgen.RetireCompetitionEntryParams{EventID: eventID, TeamID: teamID},
	); err != nil {
		return fmt.Errorf("retire unused entry: %w", err)
	}
	return nil
}

func teamView(r dbgen.HackTeam, members []dbgen.HackTeamMembersRow, showCode bool) Team {
	t := Team{
		ID:               r.ID,
		Name:             r.Name,
		State:            r.State,
		Captain:          r.CaptainID,
		PublishedCaptain: r.PublishedCaptainID,
		Frozen:           r.Frozen,
		AllocationLocked: r.AllocationLocked,
		Draft:            []Person{},
		Published:        []Person{},
	}
	if r.Capacity.Valid {
		capacity := int(r.Capacity.Int32)
		t.Capacity = &capacity
	}
	if showCode {
		t.Code = &r.InvitationCode
	}
	for _, m := range members {
		if m.TeamID != r.ID {
			continue
		}
		person := Person{ID: m.PersonID, Name: m.DisplayName}
		if m.Snapshot == "draft" {
			t.Draft = append(t.Draft, person)
		} else {
			t.Published = append(t.Published, person)
		}
	}
	return t
}
