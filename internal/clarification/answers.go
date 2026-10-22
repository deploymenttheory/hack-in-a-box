package clarification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

func configuration(
	ctx context.Context,
	q *dbgen.Queries,
	h dbgen.ClarificationAnswer,
) (AnswerConfiguration, error) {
	v, err := q.ClarificationVersion(
		ctx,
		dbgen.ClarificationVersionParams{
			EventID:  h.EventID,
			AnswerID: h.ID,
			Version:  h.LatestVersion,
		},
	)
	if err != nil {
		return AnswerConfiguration{}, fmt.Errorf("read authored answer version: %w", err)
	}
	var document AnswerDocument
	var ref Source
	if err = json.Unmarshal(v.Document, &document); err != nil {
		return AnswerConfiguration{}, fmt.Errorf("decode authored answer document: %w", err)
	}
	if err = json.Unmarshal(v.Source, &ref); err != nil {
		return AnswerConfiguration{}, fmt.Errorf("decode authored answer source: %w", err)
	}
	return AnswerConfiguration{
		ID:         h.ID,
		QuestionID: h.QuestionID,
		Revision:   h.Revision,
		Version:    v.Version,
		Document:   document,
		Source:     ref,
		Approved:   h.ApprovedVersion.Valid && h.ApprovedVersion.Int64 == v.Version,
		Published:  h.PublishedVersion.Valid && h.PublishedVersion.Int64 == v.Version,
	}, nil
}

func owningChange(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	original, current Source,
	kind string,
) error {
	if kind == "clarification" {
		if original != current {
			return access.ErrConflict
		}
		return nil
	}
	if current.ChallengeID != original.ChallengeID ||
		current.ChallengeVersion <= original.ChallengeVersion {
		return access.ErrConflict
	}
	released, err := q.ClarificationOwningRelease(ctx, dbgen.ClarificationOwningReleaseParams{
		EventID: eventID, ContentID: current.ChallengeID, Version: current.ChallengeVersion,
	})
	if err != nil {
		return fmt.Errorf("verify actual material challenge release: %w", err)
	}
	if !released {
		return access.ErrConflict
	}
	if kind == "challenge_change" {
		if current.Rubric != original.Rubric {
			return access.ErrConflict
		}
		return nil
	}
	if kind != "rubric_change" || current.Rubric.ID != original.Rubric.ID ||
		current.Rubric.Version <= original.Rubric.Version {
		return access.ErrConflict
	}
	review, err := q.RubricImpactReview(
		ctx,
		dbgen.RubricImpactReviewParams{
			EventID:  eventID,
			RubricID: current.Rubric.ID,
			Version:  current.Rubric.Version,
		},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return access.ErrConflict
	}
	if err != nil {
		return fmt.Errorf("verify owning rubric impact review: %w", err)
	}
	// A later challenge-only release keeps the same immutable criteria without
	// repeating its change notice. Retain the actual owning release as evidence.
	publication, err := q.ClarificationReviewedRubricRelease(
		ctx,
		dbgen.ClarificationReviewedRubricReleaseParams{
			EventID:          eventID,
			RubricID:         current.Rubric.ID,
			Version:          current.Rubric.Version,
			PublicDigest:     current.Rubric.Digest,
			ChallengeID:      current.ChallengeID,
			ChallengeVersion: current.ChallengeVersion,
			ReviewedNotice:   review.Notice,
		},
	)
	if err != nil {
		return fmt.Errorf("verify material rubric publication: %w", err)
	}
	if strings.TrimSpace(review.ImpactReview) == "" || strings.TrimSpace(review.Notice) == "" ||
		!publication {
		return access.ErrConflict
	}
	return nil
}

func saveHeader(
	ctx context.Context,
	q *dbgen.Queries,
	h dbgen.ClarificationAnswer,
) (dbgen.ClarificationAnswer, error) {
	row, err := q.UpdateClarificationAnswer(ctx, dbgen.UpdateClarificationAnswerParams{
		EventID: h.EventID, ID: h.ID, Revision: h.Revision, LatestVersion: h.LatestVersion,
		ApprovedVersion: h.ApprovedVersion, PublishedVersion: h.PublishedVersion,
	})
	if err != nil {
		return row, fmt.Errorf("persist official answer state: %w", err)
	}
	return row, nil
}

func (s Service) SaveAnswer(
	ctx context.Context,
	actor, eventID, questionID uuid.UUID,
	expected int64,
	document AnswerDocument,
) (AnswerConfiguration, error) {
	if err := document.validate(); err != nil || expected < 0 {
		return AnswerConfiguration{}, access.ErrInvalid
	}
	var out AnswerConfiguration
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		now := s.now()
		ref, err := rubric.ReleasedSourceForReaderInTransaction(ctx, tx, actor, eventID, now)
		if err != nil {
			return fmt.Errorf("SaveAnswer: %w", err)
		}
		if err = staff(ctx, q, actor, eventID); err != nil {
			return err
		}
		e, err := q.Event(ctx, eventID)
		if err != nil {
			return fmt.Errorf("read answer authoring policy: %w", err)
		}
		if !e.WritesAllowed {
			return access.ErrForbidden
		}
		questionRow, err := loadQuestion(ctx, q, eventID, questionID)
		if err != nil {
			return err
		}
		original, err := question(questionRow)
		if err != nil {
			return err
		}
		bound := source(ref)
		if err = owningChange(ctx, q, eventID, original.Source, bound, document.Kind); err != nil {
			return err
		}
		h, err := q.ClarificationAnswerForQuestion(
			ctx,
			dbgen.ClarificationAnswerForQuestionParams{EventID: eventID, QuestionID: questionID},
		)
		if errors.Is(err, pgx.ErrNoRows) {
			if expected != 0 {
				return access.ErrConflict
			}
			h, err = q.InsertClarificationAnswer(
				ctx,
				dbgen.InsertClarificationAnswerParams{
					EventID:    eventID,
					ID:         uuid.New(),
					QuestionID: questionID,
				},
			)
		} else if err == nil {
			if h.Revision != expected {
				return access.ErrConflict
			}
			h.Revision++
			h.LatestVersion++
			h.ApprovedVersion = pgtype.Int8{}
			h, err = saveHeader(ctx, q, h)
		}
		if err != nil {
			return fmt.Errorf("retain answer version identity: %w", err)
		}
		encoded, err := json.Marshal(document)
		if err != nil {
			return fmt.Errorf("encode answer document: %w", err)
		}
		encodedSource, err := json.Marshal(bound)
		if err != nil {
			return fmt.Errorf("encode answer owning source: %w", err)
		}
		if err = q.InsertClarificationVersion(ctx, dbgen.InsertClarificationVersionParams{
			EventID: eventID, AnswerID: h.ID, Version: h.LatestVersion,
			Document: encoded, Source: encodedSource, AuthorID: actor, AuthoredAt: now,
		}); err != nil {
			return fmt.Errorf("retain immutable answer draft: %w", err)
		}
		if err = record(
			ctx,
			q,
			eventID,
			actor,
			&questionID,
			&h.ID,
			h.Revision,
			h.LatestVersion,
			"authored",
			now,
		); err != nil {
			return err
		}
		out, err = configuration(ctx, q, h)
		return err
	})
	if err != nil {
		return AnswerConfiguration{}, fmt.Errorf("SaveAnswer: %w", err)
	}
	return out, nil
}

func (s Service) ActAnswer(
	ctx context.Context,
	actor, eventID, answerID uuid.UUID,
	expected int64,
	command AnswerCommand,
) (AnswerConfiguration, error) {
	if err := command.validate(expected); err != nil {
		return AnswerConfiguration{}, err
	}
	var out AnswerConfiguration
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		now := s.now()
		ref, err := rubric.ReleasedSourceForReaderInTransaction(ctx, tx, actor, eventID, now)
		if err != nil {
			return fmt.Errorf("ActAnswer: %w", err)
		}
		if err = staff(ctx, q, actor, eventID); err != nil {
			return err
		}
		e, err := q.Event(ctx, eventID)
		if err != nil {
			return fmt.Errorf("read answer review policy: %w", err)
		}
		if !e.WritesAllowed {
			return access.ErrForbidden
		}
		h, err := q.ClarificationAnswer(
			ctx,
			dbgen.ClarificationAnswerParams{EventID: eventID, ID: answerID},
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read reviewed answer: %w", err)
		}
		if command.Action == "publish" && h.PublishedVersion.Valid &&
			h.PublishedVersion.Int64 == command.Version {
			out, err = configuration(ctx, q, h)
			return err
		}
		if expected != h.Revision || command.Version != h.LatestVersion {
			return access.ErrConflict
		}
		page, err := configuration(ctx, q, h)
		if err != nil {
			return err
		}
		if page.Source != source(ref) {
			return access.ErrConflict
		}
		originalRow, err := loadQuestion(ctx, q, eventID, h.QuestionID)
		if err != nil {
			return err
		}
		original, err := question(originalRow)
		if err != nil {
			return err
		}
		if err = owningChange(
			ctx,
			q,
			eventID,
			original.Source,
			page.Source,
			page.Document.Kind,
		); err != nil {
			return err
		}
		if command.Action == "approve" {
			if h.ApprovedVersion.Valid && h.ApprovedVersion.Int64 == command.Version {
				out = page
				return nil
			}
			if err = q.InsertClarificationApproval(
				ctx,
				dbgen.InsertClarificationApprovalParams{
					EventID:    eventID,
					AnswerID:   answerID,
					Version:    command.Version,
					ActorID:    actor,
					ApprovedAt: now,
				},
			); err != nil {
				return fmt.Errorf("retain exact answer approval: %w", err)
			}
			h.ApprovedVersion = pgtype.Int8{Int64: command.Version, Valid: true}
		} else {
			h, err = s.preparePublication(ctx, q, h, actor, now)
			if err != nil {
				return err
			}
		}
		h.Revision++
		h, err = saveHeader(ctx, q, h)
		if err != nil {
			return fmt.Errorf("retain answer review state: %w", err)
		}
		action := "approved"
		if command.Action == "publish" {
			action = "published"
			if err = s.notifyPublication(
				ctx,
				tx,
				q,
				h,
				originalRow.Status == "answered",
			); err != nil {
				return err
			}
		}
		if err = record(
			ctx,
			q,
			eventID,
			actor,
			&h.QuestionID,
			&answerID,
			h.Revision,
			command.Version,
			action,
			now,
		); err != nil {
			return err
		}
		out, err = configuration(ctx, q, h)
		return err
	})
	if err != nil {
		return AnswerConfiguration{}, fmt.Errorf("ActAnswer: %w", err)
	}
	return out, nil
}
