package scorecards

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// ModerationCheckpointInTransaction supplies the native current configured
// snapshot to the huddle owner. This internal seam is not a reader permission;
// the owner must authorise its persisted moderator/panel before exposing it.
func ModerationCheckpointInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, id uuid.UUID,
) (Checkpoint, error) {
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return Checkpoint{}, fmt.Errorf("lock moderation checkpoint: %w", err)
	}
	cp, err := checkpoint(ctx, q, eventID, id)
	if err != nil {
		return Checkpoint{}, err
	}
	current, err := resolveWindow(row, cp)
	if err != nil {
		return Checkpoint{}, err
	}
	if current.Timezone != cp.Timezone || !current.OpensAt.Equal(cp.OpensAt) ||
		!current.ClosesAt.Equal(cp.ClosesAt) {
		return Checkpoint{}, access.ErrConflict
	}
	return cp, nil
}

// SubmittedReviewInTransaction returns only one author's exact current submitted
// review for the native checkpoint. The huddle owner must independently enforce
// its intended panel policy; drafts never pass this internal read boundary.
func (s Service) SubmittedReviewInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, teamID, author uuid.UUID,
	cp Checkpoint,
) (Card, error) {
	q := dbgen.New(tx)
	row, err := q.LatestMentorScorecard(
		ctx,
		dbgen.LatestMentorScorecardParams{EventID: eventID, ID: cardID(cp, teamID, author)},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Card{}, access.ErrConflict
	}
	if err != nil {
		return Card{}, fmt.Errorf("read required independent review: %w", err)
	}
	card, err := decodeCard(row.Document)
	if err != nil {
		return Card{}, err
	}
	if card.AuthorID != author || card.TeamID != teamID || card.State != "submitted" ||
		card.CheckpointID != cp.Definition.ID ||
		card.CheckpointVersion != cp.Version ||
		card.RunID != cp.Definition.RunID ||
		card.Rubric != cp.Rubric {
		return Card{}, access.ErrConflict
	}
	if err = s.cardAuthority(ctx, q, author, eventID, teamID, card); err != nil {
		return Card{}, err
	}
	if err = s.evidence(
		ctx,
		tx,
		author,
		eventID,
		teamID,
		card.Scores,
		true,
		false,
		card.SubmissionRevision,
	); err != nil {
		return Card{}, err
	}
	return card, nil
}

// AuthorizeModerationEvidenceInTransaction applies the native evidence policy
// to a new explicit moderation reason. It never grants note access via a panel.
func (s Service) AuthorizeModerationEvidenceInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
	refs []EvidenceReference,
	frozen *int64,
) error {
	if len(refs) < 1 || len(refs) > 50 {
		return access.ErrInvalid
	}
	for _, r := range refs {
		if len(r.Note) > 10000 {
			return access.ErrInvalid
		}
		switch r.Kind {
		case "note":
			if strings.TrimSpace(r.Note) == "" || r.ID != nil || r.Version != nil {
				return access.ErrInvalid
			}
		case "observation", "submission":
			if r.ID == nil || *r.ID == uuid.Nil || r.Version == nil || *r.Version < 1 {
				return access.ErrInvalid
			}
		default:
			return access.ErrInvalid
		}
	}
	return s.evidence(
		ctx,
		tx,
		actor,
		eventID,
		teamID,
		[]CategoryScore{{Evidence: refs}},
		true,
		false,
		frozen,
	)
}

// AuthorizeSubmittedHistoryInTransaction validates a retained submitted card's
// exact identity and its author's current native evidence authority. It does
// not require that immutable version to remain the latest independent card.
func (s Service) AuthorizeSubmittedHistoryInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, teamID, author uuid.UUID,
	ref CardReference,
) error {
	q := dbgen.New(tx)
	row, err := q.MentorScorecardVersion(
		ctx,
		dbgen.MentorScorecardVersionParams{EventID: eventID, ID: ref.ID, Version: ref.Version},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return access.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read retained submitted review: %w", err)
	}
	card, err := decodeCard(row.Document)
	if err != nil {
		return err
	}
	if card.TeamID != teamID || card.AuthorID != author || card.State != "submitted" {
		return access.ErrForbidden
	}
	if err = s.cardAuthority(ctx, q, author, eventID, teamID, card); err != nil {
		return err
	}
	return s.evidence(
		ctx,
		tx,
		author,
		eventID,
		teamID,
		card.Scores,
		true,
		false,
		card.SubmissionRevision,
	)
}
