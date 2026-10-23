package jobs

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// ResendGrantInTransaction proves a current operator explicitly authorised one
// additional attempt after this exact retained provider attempt. Each provider
// still validates its current source, audience, configuration and archive policy.
func ResendGrantInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	operationID, priorAttempt uuid.UUID,
) (uuid.UUID, error) {
	q := dbgen.New(tx)
	review, err := q.PendingDeliveryResend(
		ctx,
		dbgen.PendingDeliveryResendParams{OperationID: operationID, PriorAttemptID: &priorAttempt},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, nil
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("read reviewed resend: %w", err)
	}
	op, err := q.Operation(ctx, operationID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("read reviewed operation: %w", err)
	}
	if op.EventID != review.EventID || op.Revision != review.ExpectedRevision+1 || !op.Approved ||
		op.Cancelled ||
		(op.State != "pending" && op.State != "running") {
		return uuid.Nil, nil
	}
	if err = (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		review.ActorID,
		review.EventID,
		review.EventID,
		access.StaffRead,
	); err != nil {
		return uuid.Nil, fmt.Errorf("authorize current resend reviewer: %w", err)
	}
	paused, err := q.AutomationPaused(ctx, review.EventID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("read reviewed resend pause: %w", err)
	}
	if paused {
		return uuid.Nil, ErrRetry
	}
	return review.ID, nil
}

// EnqueueReviewedInTransaction adds a new River identity only for a durable
// current reviewed operation. Normal operations retain their existing uniqueness.
// A retry only nudges an already-authorised, definitely rejected delivery; its
// original approval remains the execution authority. It grants no resend after
// acceptance or uncertainty. Resend grants separately recheck their reviewer
// inside the provider's attempt transaction through ResendGrantInTransaction.
func (s *Service) EnqueueReviewedInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	operationID, reviewID uuid.UUID,
) error {
	q := dbgen.New(tx)
	op, err := q.Operation(ctx, operationID)
	if err != nil {
		return fmt.Errorf("read operation to enqueue review: %w", err)
	}
	review, err := q.DeliveryReview(
		ctx,
		dbgen.DeliveryReviewParams{EventID: op.EventID, ID: reviewID},
	)
	if err != nil {
		return fmt.Errorf("read durable queue review: %w", err)
	}
	if review.OperationID != op.ID ||
		(review.Action != "retry" && review.Action != "resend_acknowledged") ||
		op.Revision != review.ExpectedRevision+1 ||
		op.State != "pending" ||
		!op.Approved ||
		op.Cancelled {
		return access.ErrConflict
	}
	if _, err = s.Client.InsertTx(
		ctx,
		tx,
		Args{OperationID: op.ID, ReviewID: review.ID},
		nil,
	); err != nil {
		return fmt.Errorf("enqueue reviewed operation: %w", err)
	}
	return nil
}

func (s *Service) currentReview(
	ctx context.Context,
	operationID, reviewID uuid.UUID,
) (bool, error) {
	q := dbgen.New(s.Pool)
	op, err := q.Operation(ctx, operationID)
	if err != nil {
		return false, fmt.Errorf("read reviewed job operation: %w", err)
	}
	review, err := q.DeliveryReview(
		ctx,
		dbgen.DeliveryReviewParams{EventID: op.EventID, ID: reviewID},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read reviewed job grant: %w", err)
	}
	return review.OperationID == op.ID &&
		(review.Action == "retry" || review.Action == "resend_acknowledged") &&
		op.Revision == review.ExpectedRevision+1, nil
}
