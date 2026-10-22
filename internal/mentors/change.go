package mentors

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

var ErrCoverage = errors.New(
	"resolve invalid mentor assignments or explicitly record approval for coverage gaps",
)

type Command struct {
	Action          string    `json:"action"`
	Reason          string    `json:"reason"`
	PersonID        uuid.UUID `json:"person_id"`
	TeamID          uuid.UUID `json:"team_id"`
	CompetitorID    uuid.UUID `json:"competitor_id"`
	Slot            string    `json:"slot"`
	Window          Window    `json:"window"`
	Skills          []string  `json:"skills"`
	Availability    []Window  `json:"availability"`
	MaximumTeams    *int      `json:"maximum_teams"`
	DefaultCapacity int       `json:"default_capacity"`
	Kind            string    `json:"kind"`
	Enabled         bool      `json:"enabled"`
	AllowGaps       bool      `json:"allow_gaps"`
}

func (s Service) Change(
	ctx context.Context,
	actor, eventID uuid.UUID,
	revision int64,
	input Command,
) (Page, error) {
	var result Page
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, eventID)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock mentor change: %w", err)
		}
		if err = (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.EventEdit,
		); err != nil {
			return fmt.Errorf("authorize mentor change: %w", err)
		}
		if row.Revision != revision {
			return access.ErrConflict
		}
		if strings.TrimSpace(input.Reason) == "" || len(input.Reason) > 2000 {
			return access.ErrInvalid
		}
		page, snapshot, err := load(ctx, q, row)
		if err != nil {
			return err
		}
		if !validWindow(snapshot.Live) {
			return access.ErrInvalid
		}
		if err = change(ctx, q, row, page, snapshot, input); err != nil {
			return err
		}
		if err = audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &eventID,
				TargetID: eventID,
				Action:   audit.MentorCoverageChanged,
				Reason:   audit.OrganiserDecision,
			},
		); err != nil {
			return fmt.Errorf("audit mentor coverage: %w", err)
		}
		if _, err = q.RecordEventDecision(
			ctx,
			dbgen.RecordEventDecisionParams{
				EventID:  eventID,
				ActorID:  actor,
				Kind:     "mentor_" + input.Action,
				Reason:   input.Reason,
				Revision: revision + 1,
			},
		); err != nil {
			return fmt.Errorf("record mentor decision: %w", err)
		}
		row, err = q.BumpEventRevision(ctx, eventID)
		if err != nil {
			return fmt.Errorf("advance mentor revision: %w", err)
		}
		result, _, err = load(ctx, q, row)
		result.CanManage = true
		return err
	})
	if err != nil {
		return result, fmt.Errorf("change mentor coverage: %w", err)
	}
	return result, nil
}

func change(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	page Page,
	snapshot Snapshot,
	input Command,
) error {
	switch input.Action {
	case "settings", "profile":
		if err := configure(ctx, q, row.ID, snapshot, input); err != nil {
			return err
		}
		updated, current, err := load(ctx, q, row)
		if err != nil {
			return err
		}
		// Capacity reductions cannot silently overload an already published mentor.
		for _, gap := range Validate(current, updated.Published) {
			if gap.Code == "mentor_capacity" {
				return ErrCoverage
			}
		}
		return nil
	case "team_skills":
		if !hasTeam(snapshot, input.TeamID) {
			return access.ErrInvalid
		}
		skills, err := cleanSkills(input.Skills)
		if err != nil {
			return err
		}
		if err = q.SetMentorTeamSkills(
			ctx,
			dbgen.SetMentorTeamSkillsParams{EventID: row.ID, TeamID: input.TeamID, Skills: skills},
		); err != nil {
			return fmt.Errorf("save mentor team skills: %w", err)
		}
		return nil
	case "conflict":
		return changeConflict(ctx, q, row.ID, snapshot, input)
	case "assign":
		return assign(ctx, q, row.ID, page, snapshot, input)
	case "apply_suggestion":
		proposed := Suggest(snapshot)
		for _, gap := range proposed.Gaps {
			if gap.Hard && gap.Code != "unpublished_team" {
				return ErrCoverage
			}
		}
		if err := q.ClearMentorDraft(ctx, row.ID); err != nil {
			return fmt.Errorf("replace mentor draft: %w", err)
		}
		for _, a := range proposed.Assignments {
			if err := saveAssignment(ctx, q, row.ID, a); err != nil {
				return err
			}
		}
		return nil
	case "publish":
		return publish(ctx, q, row, page, snapshot, input.AllowGaps)
	default:
		return access.ErrInvalid
	}
}

func publish(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	page Page,
	snapshot Snapshot,
	allowGaps bool,
) error {
	for _, gap := range Validate(snapshot, page.Draft) {
		if gap.Hard || !allowGaps {
			return ErrCoverage
		}
	}
	if reflect.DeepEqual(page.Draft, page.Published) {
		return nil
	}
	if err := q.DeletePublishedMentors(ctx, row.ID); err != nil {
		return fmt.Errorf("replace published mentors: %w", err)
	}
	if err := q.PublishMentors(ctx, row.ID); err != nil {
		return fmt.Errorf("publish mentors: %w", err)
	}
	if _, err := q.InsertOperation(
		ctx,
		dbgen.InsertOperationParams{
			EventID:      row.ID,
			TargetID:     row.ID,
			OperationKey: fmt.Sprintf("mentors-published:%d", row.Revision+1),
			Kind:         "mentors.published",
			Approved:     true,
			DueAt:        time.Now().UTC(),
		},
	); err != nil {
		return fmt.Errorf("record mentor publication: %w", err)
	}
	return nil
}
