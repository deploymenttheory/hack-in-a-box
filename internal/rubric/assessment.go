package rubric

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

// FinalistPolicy belongs to the final-selection owner. A qualification final
// fails closed until that owner supplies current recorded finalist eligibility.
type (
	FinalistPolicy interface {
		AuthorizeFinalistInTransaction(context.Context, pgx.Tx, uuid.UUID, uuid.UUID) error
	}
	// AssessmentSnapshot is a private consuming contract for #101/#108, not a score
	// or award. The consumer retains its run/reference/submission version beside its
	// own immutable assessment record in the SAME transaction as this call.
	AssessmentSnapshot struct {
		RunID              uuid.UUID
		Reference          Reference
		Purpose            string
		Definition         Definition
		SubmissionRevision *int64
	}
)

func (s Service) StartAssessmentInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID, runID uuid.UUID,
) (AssessmentSnapshot, error) {
	return s.assessmentInTransaction(ctx, tx, actor, eventID, teamID, runID, true)
}

// ReadAssessmentInTransaction revalidates current final authority without
// starting a run or granting a scoring write after completion.
func (s Service) ReadAssessmentInTransaction(ctx context.Context, tx pgx.Tx,
	actor, eventID, teamID, runID uuid.UUID,
) (AssessmentSnapshot, error) {
	return s.assessmentInTransaction(ctx, tx, actor, eventID, teamID, runID, false)
}

func (s Service) assessmentInTransaction(ctx context.Context, tx pgx.Tx,
	actor, eventID, teamID, runID uuid.UUID, write bool,
) (AssessmentSnapshot, error) {
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return AssessmentSnapshot{}, fmt.Errorf("lock assessment rubric: %w", err)
	}
	run, err := q.RubricRun(ctx, dbgen.RubricRunParams{EventID: eventID, ID: runID})
	if err != nil {
		return AssessmentSnapshot{}, fmt.Errorf("read assessment run: %w", err)
	}
	h, err := q.EventRubric(ctx, eventID)
	if err != nil {
		return AssessmentSnapshot{}, fmt.Errorf("read active common rubric: %w", err)
	}
	if run.Status != "current" || run.RubricID != h.ID || !h.PublishedVersion.Valid ||
		run.RubricVersion != h.PublishedVersion.Int64 {
		return AssessmentSnapshot{}, access.ErrConflict
	}
	if err := currentRunPublication(ctx, q, eventID, run); err != nil {
		return AssessmentSnapshot{}, err
	}
	d, version, err := loadDefinition(ctx, q, h, run.RubricVersion)
	if err != nil {
		return AssessmentSnapshot{}, err
	}
	var c event.Configuration
	if err = json.Unmarshal(row.Configuration, &c); err != nil {
		return AssessmentSnapshot{}, fmt.Errorf("read assessment dates: %w", err)
	}
	now := s.now()
	if err := assessmentReadable(row, c, now, write); err != nil {
		return AssessmentSnapshot{}, err
	}

	authority, err := access.TeamPermissions(ctx, q, actor, eventID, teamID)
	if err != nil {
		return AssessmentSnapshot{}, fmt.Errorf("authorize assessment entry: %w", err)
	}
	team, err := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: eventID, ID: teamID})
	if err != nil {
		return AssessmentSnapshot{}, fmt.Errorf("read assessment team: %w", err)
	}
	if !authority.Active || team.State != "published" {
		return AssessmentSnapshot{}, access.ErrForbidden
	}
	out := AssessmentSnapshot{
		RunID:      run.ID,
		Reference:  Reference{ID: h.ID, Version: run.RubricVersion, Digest: version.PublicDigest},
		Purpose:    run.Purpose,
		Definition: d,
	}
	if run.Purpose == "checkpoint" {
		if !write {
			return AssessmentSnapshot{}, access.ErrForbidden
		}
		if (!authority.Mentor && !authority.Reviewer) || row.Stage != "live" ||
			!now.Before(c.SubmissionCutoff) {
			return AssessmentSnapshot{}, access.ErrForbidden
		}
	} else {
		if now.Before(c.SubmissionCutoff) {
			return AssessmentSnapshot{}, blocked(
				"final_judging_not_open",
				"Final assessment requires the submission cutoff and frozen final evidence.",
			)
		}
		submission, err := q.SubmissionState(
			ctx,
			dbgen.SubmissionStateParams{EventID: eventID, TeamID: teamID},
		)
		if err != nil {
			return AssessmentSnapshot{}, fmt.Errorf("read final evidence freeze: %w", err)
		}
		if submission.Outcome != "frozen" || !submission.FrozenAt.Valid ||
			!submission.ActiveRevision.Valid {
			return AssessmentSnapshot{}, blocked(
				"final_entry_not_frozen",
				"Final assessment requires a frozen final submission.",
			)
		}
		frozen := submission.ActiveRevision.Int64
		out.SubmissionRevision = &frozen
		if err = s.finalAuthority(
			ctx,
			tx,
			q,
			eventID,
			teamID,
			run,
			d,
			authority,
		); err != nil {
			return AssessmentSnapshot{}, err
		}
	}
	if !write {
		return out, nil
	}
	if err = q.StartRubricRun(
		ctx,
		dbgen.StartRubricRunParams{
			EventID:   eventID,
			ID:        run.ID,
			StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
	); err != nil {
		return AssessmentSnapshot{}, fmt.Errorf("freeze active rubric run: %w", err)
	}
	return out, nil
}

func (s Service) finalAuthority(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	eventID, teamID uuid.UUID,
	run dbgen.RubricAssessmentRun,
	d Definition,
	a access.TeamAuthority,
) error {
	if run.Purpose == "mentor_final" {
		if d.Mode != MentorQualification || (!a.Mentor && !a.Reviewer) {
			return access.ErrForbidden
		}
		return nil
	}
	if run.Purpose != "judge_final" || !a.Judge {
		return access.ErrForbidden
	}
	if d.Mode == MentorQualification {
		if s.Finalists == nil {
			return blocked(
				"finalists_required",
				"Record the current finalist selection before a fresh judged final.",
			)
		}
		if err := s.Finalists.AuthorizeFinalistInTransaction(ctx, tx, eventID, teamID); err != nil {
			return fmt.Errorf("authorize current finalist: %w", err)
		}
	}
	entry, err := q.CompetitionEntry(
		ctx,
		dbgen.CompetitionEntryParams{EventID: eventID, TeamID: teamID},
	)
	if err != nil {
		return fmt.Errorf("read judged entry: %w", err)
	}
	judges, err := q.RubricAssignedJudges(
		ctx,
		dbgen.RubricAssignedJudgesParams{EventID: eventID, ResourceID: entry.ID},
	)
	if err != nil {
		return fmt.Errorf("read required judge assignments: %w", err)
	}
	eligible := 0
	for _, judge := range judges {
		allowed, err := q.ResourceAssigned(
			ctx,
			dbgen.ResourceAssignedParams{EventID: eventID, ResourceID: entry.ID, PersonID: judge},
		)
		if err != nil {
			return fmt.Errorf("check current judge conflicts: %w", err)
		}
		if allowed {
			eligible++
		}
	}
	if eligible < d.RequiredJudgeCount {
		return blocked(
			"required_judges_missing",
			"Assign the required number of current conflict-free judges before final assessment.",
		)
	}
	return nil
}

func currentRunPublication(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	run dbgen.RubricAssessmentRun,
) error {
	challenge, err := q.RubricChallenge(ctx, eventID)
	if err != nil {
		return fmt.Errorf("read assessment challenge publication: %w", err)
	}
	if challenge.State == "withdrawn" || !challenge.ReleasedVersion.Valid {
		return access.ErrForbidden
	}
	publication, err := q.RubricPublication(
		ctx,
		dbgen.RubricPublicationParams{
			EventID:          eventID,
			ChallengeID:      challenge.ID,
			ChallengeVersion: challenge.ReleasedVersion.Int64,
		},
	)
	if err != nil {
		return fmt.Errorf("read assessment common rubric: %w", err)
	}
	if publication.RubricID != run.RubricID || publication.Version != run.RubricVersion {
		return access.ErrConflict
	}
	return nil
}

func assessmentOpen(row dbgen.Event, c event.Configuration, now time.Time) error {
	if !row.WritesAllowed || !row.PublishedAt.Valid || now.Before(row.PublishedAt.Time) ||
		c.LiveStart.IsZero() ||
		!c.SubmissionCutoff.After(c.LiveStart) ||
		now.Before(c.LiveStart) ||
		!slices.Contains([]string{"live", "assessment"}, row.Stage) {
		return blocked(
			"assessment_not_open",
			"Assessment requires the actual live-start gate and an active event.",
		)
	}
	return nil
}

func assessmentReadable(row dbgen.Event, c event.Configuration, now time.Time, write bool) error {
	if write {
		return assessmentOpen(row, c, now)
	}
	if !row.PublishedAt.Valid || now.Before(row.PublishedAt.Time) || c.LiveStart.IsZero() || !c.SubmissionCutoff.After(c.LiveStart) || now.Before(c.SubmissionCutoff) || !slices.Contains([]string{"live", "assessment", "completed"}, row.Stage) {
		return access.ErrForbidden
	}
	return nil
}
