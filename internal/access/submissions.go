package access

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// TeamAuthority is resolved afresh on every submission/file request. Role grants
// alone never imply reviewer access to a team's work.
type TeamAuthority struct{ Member, Organiser, Mentor, Reviewer, Judge, Active bool }

func TeamPermissions(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID, teamID uuid.UUID,
) (TeamAuthority, error) {
	var a TeamAuthority
	// Resolve the basic current scope atomically. Nested assessment/source
	// validators still call this policy afresh; no authority is cached between
	// requests, actors or transactions. Assignment-specific checks remain below.
	current, err := q.SubmissionTeamAuthorityScope(
		ctx,
		dbgen.SubmissionTeamAuthorityScopeParams{EventID: eventID, TeamID: teamID, PersonID: actor},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrForbidden
	}
	if err != nil {
		return a, fmt.Errorf("read current submission authority: %w", err)
	}
	if !current.PersonActive {
		return a, ErrForbidden
	}
	if !current.EntryExists {
		return a, ErrNotFound
	}
	if current.TeamState == "" {
		return a, fmt.Errorf("load submission team: %w", pgx.ErrNoRows)
	}
	a.Active = current.EntryActive && current.TeamState != "disbanded"
	roles := current.Roles
	a.Organiser = slices.Contains(roles, "organiser")
	if current.PublishedMember && a.Active {
		a.Member = current.CompetitionState == "confirmed"
	}
	if slices.Contains(roles, "mentor") && a.Active {
		a.Mentor, err = q.ResourceAssigned(
			ctx,
			dbgen.ResourceAssignedParams{EventID: eventID, ResourceID: teamID, PersonID: actor},
		)
		if err != nil {
			return a, fmt.Errorf("check assigned mentor: %w", err)
		}
	}
	if slices.Contains(roles, "mentor") && a.Active {
		a.Reviewer, err = q.ResourceAssigned(
			ctx,
			dbgen.ResourceAssignedParams{
				EventID:    eventID,
				ResourceID: current.EntryID,
				PersonID:   actor,
			},
		)
		if err != nil {
			return a, fmt.Errorf("check assigned second reviewer: %w", err)
		}
	}
	if slices.Contains(roles, "judge") && a.Active {
		a.Judge, err = q.ResourceAssigned(
			ctx,
			dbgen.ResourceAssignedParams{
				EventID:    eventID,
				ResourceID: current.EntryID,
				PersonID:   actor,
			},
		)
		if err != nil {
			return a, fmt.Errorf("check assigned judge: %w", err)
		}
		if a.Judge {
			a.Judge, err = q.CurrentJudgeAssignment(
				ctx,
				dbgen.CurrentJudgeAssignmentParams{
					EventID:  eventID,
					TeamID:   teamID,
					PersonID: actor,
				},
			)
			if err != nil {
				return a, fmt.Errorf("check managed judge assignment: %w", err)
			}
		}
	}
	// A competing reviewer cannot obtain privileged access to their own entry.
	if a.Member {
		a.Mentor, a.Reviewer, a.Judge = false, false, false
	}
	return a, nil
}

type submissionTimes struct {
	LiveStart        time.Time `json:"live_start"`
	SubmissionCutoff time.Time `json:"submission_cutoff"`
}

func (p EventPolicy) teamFileWrite(
	ctx context.Context,
	actor uuid.UUID,
	row dbgen.Event,
	team uuid.UUID,
) error {
	a, err := TeamPermissions(ctx, p.Queries, actor, row.ID, team)
	if err != nil {
		return err
	}
	var c submissionTimes
	if err := json.Unmarshal(row.Configuration, &c); err != nil {
		return fmt.Errorf("read team upload dates: %w", err)
	}
	now := p.now()
	if !row.WritesAllowed || !a.Active || !row.PublishedAt.Valid ||
		now.Before(row.PublishedAt.Time) ||
		c.LiveStart.IsZero() ||
		c.SubmissionCutoff.IsZero() {
		return ErrForbidden
	}
	if a.Member && now.Before(c.SubmissionCutoff) {
		return nil
	}
	if a.Organiser && !now.Before(c.LiveStart) {
		return nil
	}
	return ErrForbidden
}

func (p EventPolicy) teamFileRead(
	ctx context.Context,
	actor uuid.UUID,
	row dbgen.Event,
	file dbgen.File,
) error {
	a, err := TeamPermissions(ctx, p.Queries, actor, row.ID, *file.TeamID)
	if err != nil {
		return err
	}
	if !row.PublishedAt.Valid || p.now().Before(row.PublishedAt.Time) {
		return ErrForbidden
	}
	if a.Member {
		return nil
	}
	var c submissionTimes
	if err := json.Unmarshal(row.Configuration, &c); err != nil {
		return fmt.Errorf("read submission file dates: %w", err)
	}
	if c.LiveStart.IsZero() || p.now().Before(c.LiveStart) {
		return ErrForbidden
	}
	if a.Organiser && file.OwnerID == actor {
		return nil
	}
	audiences, err := p.Queries.SubmissionFileAudiences(
		ctx,
		dbgen.SubmissionFileAudiencesParams{EventID: row.ID, TeamID: *file.TeamID, FileID: file.ID},
	)
	if err != nil {
		return fmt.Errorf("read submitted file references: %w", err)
	}
	for _, audience := range audiences {
		if a.Organiser ||
			((a.Mentor || a.Reviewer) && slices.Contains(audience.Audience, "mentor")) {
			return nil
		}
		if a.Judge && !c.SubmissionCutoff.IsZero() && !p.now().Before(c.SubmissionCutoff) &&
			slices.Contains(audience.Audience, "judge") {
			state, err := p.Queries.SubmissionState(
				ctx,
				dbgen.SubmissionStateParams{EventID: row.ID, TeamID: *file.TeamID},
			)
			if err != nil {
				return fmt.Errorf("read frozen file outcome: %w", err)
			}
			if state.Outcome == "frozen" && state.ActiveRevision.Valid &&
				state.ActiveRevision.Int64 == audience.SubmissionRevision {
				return nil
			}
		}
	}
	return ErrForbidden
}
