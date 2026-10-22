package rubric

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

type PublicPage struct {
	Reference   Reference        `json:"reference"`
	Definition  PublicDefinition `json:"definition"`
	PublishedAt time.Time        `json:"published_at"`
	Notice      string           `json:"notice"`
}

// ValidateReference never substitutes the latest rubric for an authored exact
// reference. Approval also rechecks the current approving organiser.
func ValidateReference(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	ref *Reference,
	approved bool,
) error {
	if ref == nil || ref.ID == uuid.Nil || ref.Version < 1 || len(ref.Digest) != 64 {
		return blocked("rubric_required", "Reference the exact approved common rubric snapshot.")
	}
	h, err := q.EventRubric(ctx, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return blocked(
			"rubric_required",
			"Configure and approve the common rubric before challenge publication.",
		)
	}
	if err != nil {
		return fmt.Errorf("read referenced rubric: %w", err)
	}
	if h.ID != ref.ID {
		return access.ErrConflict
	}
	_, version, err := loadDefinition(ctx, q, h, ref.Version)
	if err != nil {
		return err
	}
	if version.PublicDigest != ref.Digest {
		return access.ErrConflict
	}
	if !approved {
		return nil
	}
	if !h.ApprovedVersion.Valid || h.ApprovedVersion.Int64 != ref.Version {
		return blocked(
			"rubric_approval_required",
			"Approve and reference the exact intended rubric version.",
		)
	}
	approval, err := q.RubricApproval(
		ctx,
		dbgen.RubricApprovalParams{EventID: eventID, RubricID: h.ID, Version: ref.Version},
	)
	if err != nil {
		return fmt.Errorf("read rubric approval: %w", err)
	}
	roles, _, err := access.ContentReader(ctx, q, approval.ActorID, eventID)
	if err != nil {
		return fmt.Errorf("authorize current rubric approver: %w", err)
	}
	if !slices.Contains(roles, "organiser") {
		return access.ErrForbidden
	}
	return nil
}

// PublishInTransaction is called only inside the guarded content-release event
// transaction. Rubric visibility and any reviewed correction notice commit with
// that exact challenge version, so a separate early publication route cannot exist.
func PublishInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	event dbgen.Event,
	challenge dbgen.ContentItem,
	ref *Reference,
	now time.Time,
) error {
	if challenge.EventID != event.ID || challenge.Kind != "challenge" {
		return access.ErrInvalid
	}
	q := dbgen.New(tx)
	if err := ValidateReference(ctx, q, event.ID, ref, true); err != nil {
		return err
	}
	h, err := q.EventRubric(ctx, event.ID)
	if err != nil {
		return fmt.Errorf("read rubric publication state: %w", err)
	}
	notice := ""
	replacement := event.PublishedAt.Valid &&
		(!h.PublishedVersion.Valid || h.PublishedVersion.Int64 != ref.Version)
	if replacement {
		review, err := q.RubricImpactReview(
			ctx,
			dbgen.RubricImpactReviewParams{EventID: event.ID, RubricID: h.ID, Version: ref.Version},
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return blocked(
				"rubric_impact_review_required",
				"Review the impact and approve a common attendee notice before changing published criteria.",
			)
		}
		if err != nil {
			return fmt.Errorf("read reviewed rubric correction: %w", err)
		}
		roles, _, err := access.ContentReader(ctx, q, review.ActorID, event.ID)
		if err != nil {
			return fmt.Errorf("authorize rubric correction reviewer: %w", err)
		}
		if !slices.Contains(roles, "organiser") {
			return access.ErrForbidden
		}
		notice = review.Notice
	}
	runs, err := q.CurrentRubricRuns(ctx, event.ID)
	if err != nil {
		return fmt.Errorf("check frozen rubric runs: %w", err)
	}
	for _, run := range runs {
		if run.StartedAt.Valid && run.RubricVersion != ref.Version {
			return blocked(
				"rubric_restart_required",
				"Explicitly restart affected assessment runs before changing the criteria; existing score versions stay retained.",
			)
		}
	}
	if err = q.RecordRubricPublication(
		ctx,
		dbgen.RecordRubricPublicationParams{
			EventID:          event.ID,
			RubricID:         ref.ID,
			Version:          ref.Version,
			ChallengeID:      challenge.ID,
			ChallengeVersion: challenge.Version,
			PublicDigest:     ref.Digest,
			PublishedAt:      now,
			ReviewedNotice:   notice,
		},
	); err != nil {
		return fmt.Errorf("retain common rubric publication: %w", err)
	}
	if replacement {
		if _, err = q.NotifyRubricAttendees(
			ctx,
			dbgen.NotifyRubricAttendeesParams{
				EventID:          event.ID,
				ChallengeID:      challenge.ID,
				ChallengeVersion: challenge.Version,
			},
		); err != nil {
			return fmt.Errorf("record affected rubric notice recipients: %w", err)
		}
	}
	h.PublishedVersion = pgtype.Int8{Int64: ref.Version, Valid: true}
	return record(ctx, q, &h, nil, ref.Version, "published", "", now)
}

func (s Service) Public(ctx context.Context, actor, eventID uuid.UUID) (PublicPage, error) {
	var out PublicPage
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock common rubric view: %w", err)
		}
		_, confirmed, err := access.ContentReader(ctx, q, actor, eventID)
		if err != nil {
			return fmt.Errorf("authorize common rubric reader: %w", err)
		}
		if !confirmed || !row.PublishedAt.Valid {
			return access.ErrNotFound
		}
		challenge, err := q.RubricChallenge(ctx, eventID)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read common rubric challenge: %w", err)
		}
		version, err := access.ContentVersionForReader(ctx, q, actor, challenge, false)
		if err != nil {
			return fmt.Errorf("authorize released rubric challenge: %w", err)
		}
		if version == 0 {
			return access.ErrNotFound
		}
		publication, err := q.RubricPublication(
			ctx,
			dbgen.RubricPublicationParams{
				EventID:          eventID,
				ChallengeID:      challenge.ID,
				ChallengeVersion: version,
			},
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read released rubric publication: %w", err)
		}
		content, err := q.ContentVersion(
			ctx,
			dbgen.ContentVersionParams{EventID: eventID, ContentID: challenge.ID, Version: version},
		)
		if err != nil {
			return fmt.Errorf("read released challenge reference: %w", err)
		}
		var document struct {
			Rubric *Reference `json:"rubric"`
		}
		if err = json.Unmarshal(content.Document, &document); err != nil {
			return fmt.Errorf("decode common rubric reference: %w", err)
		}
		ref := document.Rubric
		if ref == nil || ref.ID != publication.RubricID || ref.Version != publication.Version ||
			ref.Digest != publication.PublicDigest {
			return access.ErrConflict
		}
		rubric, err := q.RubricVersion(
			ctx,
			dbgen.RubricVersionParams{EventID: eventID, RubricID: ref.ID, Version: ref.Version},
		)
		if err != nil {
			return fmt.Errorf("read released common rubric: %w", err)
		}
		var definition PublicDefinition
		if err = json.Unmarshal(rubric.PublicDocument, &definition); err != nil {
			return fmt.Errorf("decode released common criteria: %w", err)
		}
		if (Definition{PublicDefinition: definition}).PublicDigest() != ref.Digest {
			return access.ErrConflict
		}
		out = PublicPage{
			Reference:   *ref,
			Definition:  definition,
			PublishedAt: publication.PublishedAt,
			Notice:      publication.ReviewedNotice,
		}
		return nil
	})
	if err != nil {
		return out, fmt.Errorf("common rubric: %w", err)
	}
	return out, nil
}
