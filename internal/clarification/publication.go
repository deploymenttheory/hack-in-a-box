package clarification

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

func (s Service) preparePublication(
	ctx context.Context,
	q *dbgen.Queries,
	h dbgen.ClarificationAnswer,
	actor uuid.UUID,
	now time.Time,
) (dbgen.ClarificationAnswer, error) {
	if !h.ApprovedVersion.Valid || h.ApprovedVersion.Int64 != h.LatestVersion {
		return h, access.ErrConflict
	}
	approval, approvalErr := q.ClarificationApproval(
		ctx,
		dbgen.ClarificationApprovalParams{
			EventID:  h.EventID,
			AnswerID: h.ID,
			Version:  h.LatestVersion,
		},
	)
	if approvalErr != nil {
		return h, fmt.Errorf("read retained answer approval: %w", approvalErr)
	}
	if approvalErr = staff(ctx, q, approval.ActorID, h.EventID); approvalErr != nil {
		return h, approvalErr
	}
	if s.Notifications == nil {
		return h, &event.Blocked{
			Issues: []event.Issue{
				{
					Field:   "clarification",
					Code:    "clarification_notifications_unavailable",
					Message: "Configure the native inbox before publishing a common answer.",
				},
			},
		}
	}
	if err := q.InsertClarificationPublication(
		ctx,
		dbgen.InsertClarificationPublicationParams{
			EventID:     h.EventID,
			AnswerID:    h.ID,
			Version:     h.LatestVersion,
			ActorID:     actor,
			PublishedAt: now,
		},
	); err != nil {
		return h, fmt.Errorf("retain official answer publication: %w", err)
	}
	h.PublishedVersion = pgtype.Int8{Int64: h.LatestVersion, Valid: true}
	return h, nil
}

func (s Service) notifyPublication(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	h dbgen.ClarificationAnswer,
	alreadyAnswered bool,
) error {
	if !alreadyAnswered {
		if _, err := q.UpdateClarificationQuestion(
			ctx,
			dbgen.UpdateClarificationQuestionParams{
				EventID: h.EventID,
				ID:      h.QuestionID,
				Status:  "answered",
			},
		); err != nil {
			return fmt.Errorf("mark question officially answered: %w", err)
		}
	}
	people, recipientErr := q.ClarificationRecipients(ctx, h.EventID)
	if recipientErr != nil {
		return fmt.Errorf("resolve common answer recipients: %w", recipientErr)
	}
	if err := s.Notifications.RecordClarificationInTransaction(
		ctx,
		tx,
		h.EventID,
		h.ID,
		h.LatestVersion,
		people,
	); err != nil {
		return fmt.Errorf("record common answer inbox references: %w", err)
	}
	return nil
}
