package scorecards

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

func (s Service) guard(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID, teamID uuid.UUID,
) (dbgen.Event, error) {
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return row, fmt.Errorf("lock private scorecards: %w", err)
	}
	a, err := access.TeamPermissions(ctx, q, actor, eventID, teamID)
	if err != nil {
		return row, fmt.Errorf("read current scorecard assignment: %w", err)
	}
	team, err := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: eventID, ID: teamID})
	if err != nil {
		return row, fmt.Errorf("read scorecard team: %w", err)
	}
	if !a.Active || team.State != "published" || (!a.Mentor && !a.Reviewer) {
		return row, access.ErrForbidden
	}
	var c event.Configuration
	if err = json.Unmarshal(row.Configuration, &c); err != nil {
		return row, fmt.Errorf("read current scorecard clock: %w", err)
	}
	if !row.PublishedAt.Valid || s.now().Before(row.PublishedAt.Time) || c.LiveStart.IsZero() ||
		s.now().Before(c.LiveStart) {
		return row, access.ErrForbidden
	}
	return row, nil
}

func (s Service) cardAuthority(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID, teamID uuid.UUID,
	c Card,
) error {
	row, err := s.guard(ctx, q, actor, eventID, teamID)
	if err != nil {
		return err
	}
	if c.TeamID != teamID {
		return access.ErrNotFound
	}
	if c.Purpose == "mentor_final" {
		var dates event.Configuration
		if err := json.Unmarshal(row.Configuration, &dates); err != nil {
			return fmt.Errorf("read final scorecard dates: %w", err)
		}
		if dates.SubmissionCutoff.IsZero() || s.now().Before(dates.SubmissionCutoff) {
			return access.ErrForbidden
		}
		state, err := q.SubmissionState(
			ctx,
			dbgen.SubmissionStateParams{EventID: eventID, TeamID: teamID},
		)
		if err != nil {
			return fmt.Errorf("recheck final scorecard evidence: %w", err)
		}
		if c.SubmissionRevision == nil || state.Outcome != "frozen" || !state.FrozenAt.Valid ||
			!state.ActiveRevision.Valid ||
			state.ActiveRevision.Int64 != *c.SubmissionRevision {
			return access.ErrForbidden
		}
	}
	return nil
}

func recordChange(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, actor, target uuid.UUID,
	previous int64,
	action audit.Action,
) error {
	row, err := q.BumpEventRevision(ctx, eventID)
	if err != nil {
		return fmt.Errorf("advance scorecard revision: %w", err)
	}
	if err = audit.Append(
		ctx,
		q,
		audit.Record{
			ActorID:  &actor,
			EventID:  &eventID,
			TargetID: target,
			Action:   action,
			Reason:   audit.UserRequest,
			Metadata: audit.Metadata{PreviousRevision: &previous, Revision: &row.Revision},
		},
	); err != nil {
		return fmt.Errorf("audit private scorecard decision: %w", err)
	}
	return nil
}
