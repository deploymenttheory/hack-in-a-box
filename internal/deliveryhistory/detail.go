package deliveryhistory

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func (s Service) Detail(
	ctx context.Context,
	actor, eventID, deliveryID uuid.UUID,
	beforeAttempt int32,
	beforeReview *uuid.UUID,
) (Detail, error) {
	if beforeAttempt < 0 {
		return Detail{}, access.ErrInvalid
	}
	out := Detail{Attempts: []Attempt{}, Reviews: []Review{}, Actions: []string{}}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		event, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock delivery detail: %w", err)
		}
		if err = authorize(ctx, q, actor, eventID); err != nil {
			return err
		}
		row, err := lookup(ctx, q, eventID, deliveryID)
		if err != nil {
			return err
		}
		out.Item = item(row)
		out.Archived = !event.WritesAllowed
		out.Paused, err = q.AutomationPaused(ctx, eventID)
		if err != nil {
			return fmt.Errorf("read delivery pause: %w", err)
		}
		if row.OperationID == uuid.Nil {
			return nil
		}
		if row.Channel == "smtp" {
			if err = readSMTPAttempts(ctx, q, row.OperationID, beforeAttempt, &out); err != nil {
				return err
			}
		}
		if row.Channel == "teams" {
			if err = readTeamsAttempts(ctx, q, eventID, row.ID, beforeAttempt, &out); err != nil {
				return err
			}
		}
		reviews, err := q.DeliveryReviews(
			ctx,
			dbgen.DeliveryReviewsParams{
				EventID:     eventID,
				OperationID: row.OperationID,
				BeforeID:    beforeReview,
				PageSize:    21,
			},
		)
		if err != nil {
			return fmt.Errorf("read delivery decisions: %w", err)
		}
		if len(reviews) > 20 {
			out.NextReviewID = &reviews[19].ID
			reviews = reviews[:20]
		}
		for _, r := range reviews {
			out.Reviews = append(
				out.Reviews,
				Review{
					ExpectedRevision: r.ExpectedRevision,
					ID:               r.ID,
					ActorID:          r.ActorID,
					Action:           r.Action,
					Reason:           r.Reason,
					CreatedAt:        r.CreatedAt,
				},
			)
		}
		out.Actions = s.availableActions(ctx, tx, q, row, out.Paused, out.Archived)
		return nil
	})
	if err != nil {
		return Detail{}, fmt.Errorf("delivery detail: %w", err)
	}
	return out, nil
}

func readSMTPAttempts(
	ctx context.Context,
	q *dbgen.Queries,
	operationID uuid.UUID,
	before int32,
	out *Detail,
) error {
	rows, err := q.SMTPDeliveryAttempts(
		ctx,
		dbgen.SMTPDeliveryAttemptsParams{
			OperationID:  operationID,
			BeforeNumber: before,
			PageSize:     51,
		},
	)
	if err != nil {
		return fmt.Errorf("read SMTP attempt evidence: %w", err)
	}
	if len(rows) > 50 {
		n := rows[49].AttemptNumber
		out.NextAttemptNumber = &n
		rows = rows[:50]
	}
	for _, r := range rows {
		a := Attempt{
			ID:        r.ID,
			Number:    r.AttemptNumber,
			StartedAt: r.PreparedAt,
			State:     "ambiguous",
			Reason:    "acceptance_unknown",
		}
		if r.Outcome.Valid {
			a.State = r.Outcome.String
			a.Reason = reason("smtp", r.ReceiptCode.String)
			if a.State == "accepted" {
				a.State = "provider_accepted"
			}
		}
		if r.RecordedAt.Valid {
			a.RecordedAt = &r.RecordedAt.Time
		}
		out.Attempts = append(out.Attempts, a)
	}
	return nil
}

func readTeamsAttempts(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, deliveryID uuid.UUID,
	before int32,
	out *Detail,
) error {
	rows, err := q.TeamsDeliveryAttempts(
		ctx,
		dbgen.TeamsDeliveryAttemptsParams{
			EventID:      eventID,
			DeliveryID:   deliveryID,
			BeforeNumber: before,
			PageSize:     51,
		},
	)
	if err != nil {
		return fmt.Errorf("read Teams attempt evidence: %w", err)
	}
	if len(rows) > 50 {
		n := rows[49].AttemptNumber
		out.NextAttemptNumber = &n
		rows = rows[:50]
	}
	for _, r := range rows {
		a := Attempt{
			ID:        r.ID,
			Number:    r.AttemptNumber,
			StartedAt: r.StartedAt,
			State:     "ambiguous",
			Reason:    "process_interrupted",
		}
		if r.Outcome.Valid {
			a.State = r.Outcome.String
			a.Reason = reason("teams", r.Reason.String)
		}
		if r.RecordedAt.Valid {
			a.RecordedAt = &r.RecordedAt.Time
		}
		if r.HttpStatus.Valid {
			a.HTTPStatus = &r.HttpStatus.Int32
		}
		if r.RetryAt.Valid {
			a.RetryAt = &r.RetryAt.Time
		}
		out.Attempts = append(out.Attempts, a)
	}
	return nil
}
