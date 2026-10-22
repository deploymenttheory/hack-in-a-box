package moderation

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
)

func (s Service) guard(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
	write bool,
) (dbgen.Event, bool, error) {
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return row, false, fmt.Errorf("lock moderation event: %w", err)
	}
	authority, err := access.TeamPermissions(ctx, q, actor, eventID, teamID)
	if err != nil {
		return row, false, fmt.Errorf("authorize current moderation source: %w", err)
	}
	roles, _, err := access.ContentReader(ctx, q, actor, eventID)
	if err != nil {
		return row, false, fmt.Errorf("authorize current moderation source: %w", err)
	}
	organiser := slices.Contains(roles, "organiser")
	if !authority.Active || (!organiser && !authority.Mentor && !authority.Reviewer) {
		return row, false, access.ErrForbidden
	}
	var dates event.Configuration
	if err = json.Unmarshal(row.Configuration, &dates); err != nil {
		return row, false, fmt.Errorf("read current moderation dates: %w", err)
	}
	if !row.PublishedAt.Valid || s.now().Before(row.PublishedAt.Time) || dates.LiveStart.IsZero() ||
		s.now().Before(dates.LiveStart) {
		return row, false, access.ErrForbidden
	}
	if write &&
		(!row.WritesAllowed || !slices.Contains([]string{"live", "assessment"}, row.Stage)) {
		return row, false, access.ErrForbidden
	}
	if err = mentors.AuthorizeModerationPanelInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		teamID,
	); err != nil {
		return row, false, fmt.Errorf("authorize current moderation source: %w", err)
	}
	return row, organiser, nil
}

func (s Service) current(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	h Huddle,
	write bool,
) (dbgen.Event, error) {
	row, _, err := s.guard(ctx, tx, actor, eventID, h.TeamID, write)
	if err != nil {
		return row, fmt.Errorf("authorize current moderation source: %w", err)
	}
	if !slices.Contains(h.PanelIDs, actor) {
		return row, access.ErrForbidden
	}
	if err = mentors.AuthorizeModerationPanelInTransaction(
		ctx,
		tx,
		h.Definition.ModeratorID,
		eventID,
		h.TeamID,
	); err != nil {
		return row, fmt.Errorf("authorize current moderation source: %w", err)
	}
	roles, _, err := access.ContentReader(ctx, dbgen.New(tx), h.Definition.ModeratorID, eventID)
	if err != nil {
		return row, fmt.Errorf("authorize current moderation source: %w", err)
	}
	if !slices.Contains(roles, "organiser") {
		return row, access.ErrForbidden
	}
	coverage, err := mentors.ReviewCoverageInTransaction(ctx, tx, eventID, h.TeamID)
	if err != nil {
		return row, fmt.Errorf("authorize current moderation source: %w", err)
	}
	if coverage.Primary.PersonID != h.PrimaryID || coverage.Second.PersonID != h.Review.ReviewerID {
		return row, access.ErrConflict
	}
	cp, err := scorecards.ModerationCheckpointInTransaction(
		ctx,
		tx,
		eventID,
		h.Definition.CheckpointID,
	)
	if err != nil {
		return row, fmt.Errorf("authorize current moderation source: %w", err)
	}
	if cp.Version != h.Definition.CheckpointVersion || binding(cp) != binding(h.Checkpoint) {
		return row, access.ErrConflict
	}
	source, err := rubric.ReleasedSourceForReaderInTransaction(
		ctx,
		tx,
		h.Definition.ModeratorID,
		eventID,
		s.now(),
	)
	if err != nil {
		return row, fmt.Errorf("authorize current moderation source: %w", err)
	}
	if source.Reference != h.Checkpoint.Rubric {
		return row, access.ErrConflict
	}
	return row, nil
}

func (s Service) newHuddle(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
	d Definition,
) (Huddle, error) {
	_, organiser, err := s.guard(ctx, tx, actor, eventID, teamID, true)
	if err != nil {
		return Huddle{}, fmt.Errorf("authorize current moderation source: %w", err)
	}
	if !organiser {
		return Huddle{}, access.ErrForbidden
	}
	if d.ID == uuid.Nil || d.ModeratorID == uuid.Nil || d.SecondReviewerID == uuid.Nil ||
		d.CheckpointID == uuid.Nil ||
		d.CheckpointVersion < 1 ||
		d.ReviewDueAt.IsZero() {
		return Huddle{}, access.ErrInvalid
	}
	roles, _, err := access.ContentReader(ctx, dbgen.New(tx), d.ModeratorID, eventID)
	if err != nil {
		return Huddle{}, fmt.Errorf("authorize current moderation source: %w", err)
	}
	if !slices.Contains(roles, "organiser") {
		return Huddle{}, access.ErrForbidden
	}
	if err = mentors.AuthorizeModerationPanelInTransaction(
		ctx,
		tx,
		d.ModeratorID,
		eventID,
		teamID,
	); err != nil {
		return Huddle{}, fmt.Errorf("authorize current moderation source: %w", err)
	}
	coverage, err := mentors.ReviewCoverageInTransaction(ctx, tx, eventID, teamID)
	if err != nil {
		return Huddle{}, fmt.Errorf("authorize current moderation source: %w", err)
	}
	if coverage.Second.PersonID != d.SecondReviewerID ||
		d.SecondReviewerID == coverage.Primary.PersonID {
		return Huddle{}, access.ErrConflict
	}
	cp, err := scorecards.ModerationCheckpointInTransaction(ctx, tx, eventID, d.CheckpointID)
	if err != nil {
		return Huddle{}, fmt.Errorf("authorize current moderation source: %w", err)
	}
	if cp.Version != d.CheckpointVersion ||
		!slices.Contains([]string{"checkpoint", "mentor_final"}, cp.Purpose) {
		return Huddle{}, access.ErrConflict
	}
	if d.ReviewDueAt.Before(s.now()) || d.ReviewDueAt.Before(cp.OpensAt) ||
		!d.ReviewDueAt.Before(cp.ClosesAt) {
		return Huddle{}, access.ErrInvalid
	}
	source, err := rubric.ReleasedSourceForReaderInTransaction(ctx, tx, actor, eventID, s.now())
	if err != nil {
		return Huddle{}, fmt.Errorf("authorize current moderation source: %w", err)
	}
	if source.Reference != cp.Rubric {
		return Huddle{}, access.ErrConflict
	}
	panel := []uuid.UUID{d.ModeratorID}
	for _, person := range []uuid.UUID{coverage.Primary.PersonID, d.SecondReviewerID} {
		if !slices.Contains(panel, person) {
			panel = append(panel, person)
		}
	}
	return Huddle{
		Definition: d,
		TeamID:     teamID,
		State:      "planned",
		Checkpoint: cp,
		PrimaryID:  coverage.Primary.PersonID,
		PanelIDs:   panel,
		Review:     SecondReview{ReviewerID: d.SecondReviewerID, DueAt: d.ReviewDueAt},
		Cards:      []scorecards.CardReference{},
		Gaps:       []Gap{},
		Proposals:  []Proposal{},
	}, nil
}
