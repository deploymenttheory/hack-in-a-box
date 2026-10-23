package deliveryhistory

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
)

func (s Service) Change(
	ctx context.Context,
	actor, eventID, deliveryID uuid.UUID,
	revision int64,
	input Command,
) (Detail, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	if !validCommand(revision, input) {
		return Detail{}, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		// Resolve only a currently authorised typed delivery, then take the same
		// nonblocking advisory guard as execution before Event→Operation locks.
		if err := authorize(ctx, q, actor, eventID); err != nil {
			return err
		}
		row, err := lookup(ctx, q, eventID, deliveryID)
		if err != nil {
			return err
		}
		if row.OperationID == uuid.Nil {
			return access.ErrInvalid
		}
		if replayed, replayErr := reviewReplay(
			ctx,
			q,
			actor,
			eventID,
			row.OperationID,
			revision,
			input,
		); replayed ||
			replayErr != nil {
			return replayErr
		}
		var locked bool
		if err = tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))", row.OperationID.String()).
			Scan(&locked); err != nil {
			return fmt.Errorf("guard delivery review: %w", err)
		}
		if !locked {
			return access.ErrConflict
		}
		if _, err = q.LockEvent(ctx, eventID); err != nil {
			return fmt.Errorf("lock delivery review event: %w", err)
		}
		if err = authorize(ctx, q, actor, eventID); err != nil {
			return err
		}
		row, err = lookup(ctx, q, eventID, deliveryID)
		if err != nil {
			return err
		}
		if replayed, replayErr := reviewReplay(
			ctx,
			q,
			actor,
			eventID,
			row.OperationID,
			revision,
			input,
		); replayed ||
			replayErr != nil {
			return replayErr
		}
		op, err := q.LockOperation(ctx, row.OperationID)
		if err != nil {
			return fmt.Errorf("lock delivery review operation: %w", err)
		}
		if op.Revision != revision || op.EventID != eventID || op.State == "running" ||
			op.LeaseToken != nil ||
			op.Cancelled {
			return access.ErrConflict
		}
		if err = s.reviewAction(ctx, tx, q, row, op, input); err != nil {
			return err
		}
		var priorAttempt *uuid.UUID
		if input.Action == "resend_acknowledged" {
			id, err := lastAttempt(ctx, q, row)
			if err != nil {
				return err
			}
			priorAttempt = &id
		}
		if err = q.RecordDeliveryReview(
			ctx,
			dbgen.RecordDeliveryReviewParams{
				ID:               input.RequestID,
				EventID:          eventID,
				OperationID:      op.ID,
				ActorID:          actor,
				ExpectedRevision: revision,
				Action:           input.Action,
				Reason:           input.Reason,
				PriorAttemptID:   priorAttempt,
			},
		); err != nil {
			return fmt.Errorf("retain delivery recovery decision: %w", err)
		}
		if input.Action == "retry" || input.Action == "resend_acknowledged" {
			if s.Jobs == nil {
				return database.ErrDatabase
			}
			if err = s.Jobs.EnqueueReviewedInTransaction(
				ctx,
				tx,
				op.ID,
				input.RequestID,
			); err != nil {
				return fmt.Errorf("queue reviewed delivery: %w", err)
			}
		}
		result := "success"
		if err = audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &eventID,
				TargetID: deliveryID,
				Action:   audit.CommunicationChanged,
				Reason:   audit.OrganiserDecision,
				Metadata: audit.Metadata{
					ReferenceID:      &input.RequestID,
					Result:           &result,
					PreviousRevision: &revision,
				},
			},
		); err != nil {
			return fmt.Errorf("audit delivery decision: %w", err)
		}
		return nil
	})
	if err != nil {
		return Detail{}, fmt.Errorf("review delivery: %w", err)
	}
	return s.Detail(ctx, actor, eventID, deliveryID, 0, nil)
}

func (s Service) reviewAction(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	row dbgen.CommunicationDeliveryHistoryRow,
	op dbgen.Operation,
	input Command,
) error {
	projection := item(row)
	switch input.Action {
	case "cancel_resend":
		return cancelResend(ctx, q, row, op)
	case "cancel":
		if op.State != "pending" ||
			slices.Contains(
				[]string{"provider_accepted", "ambiguous", "in_progress"},
				projection.State,
			) {
			return access.ErrConflict
		}
		changed, err := q.CancelOperation(ctx, op.ID)
		if err != nil {
			return fmt.Errorf("cancel unattempted delivery: %w", err)
		}
		if changed != 1 {
			return access.ErrConflict
		}
		return nil
	case "resolve_no_resend":
		if projection.State != "ambiguous" || op.State != "ambiguous" {
			return access.ErrConflict
		}
		changed, err := q.ResolveUnknownDelivery(
			ctx,
			dbgen.ResolveUnknownDeliveryParams{ID: op.ID, Revision: op.Revision},
		)
		if err != nil {
			return fmt.Errorf("resolve uncertain delivery without resend: %w", err)
		}
		if changed != 1 {
			return access.ErrConflict
		}
		return nil
	case "retry":
		if projection.State != "retryable" || op.State != "pending" {
			return access.ErrConflict
		}
	case "resend_acknowledged":
		if !slices.Contains([]string{"ambiguous", "provider_accepted"}, projection.State) ||
			!slices.Contains([]string{"ambiguous", "delivered"}, op.State) {
			return access.ErrConflict
		}
	}
	if row.Purpose != "campaign" && (row.Channel != "teams" || row.Purpose != "broadcast") {
		return access.ErrInvalid
	}
	paused, err := q.AutomationPaused(ctx, op.EventID)
	if err != nil {
		return fmt.Errorf("read current recovery pause: %w", err)
	}
	if paused {
		return access.ErrConflict
	}
	provider := s.Providers[row.Channel]
	if provider == nil {
		return access.ErrForbidden
	}
	if err = provider.ValidateRecoveryInTransaction(
		ctx,
		tx,
		jobs.Delivery{ID: op.ID, EventID: op.EventID, TargetID: op.TargetID, Key: op.ID.String()},
	); err != nil {
		return fmt.Errorf("validate current provider recovery: %w", err)
	}
	changed, err := q.ReviewRetryOperation(
		ctx,
		dbgen.ReviewRetryOperationParams{ID: op.ID, Revision: op.Revision},
	)
	if err != nil {
		return fmt.Errorf("prepare reviewed operation: %w", err)
	}
	if changed != 1 {
		return access.ErrConflict
	}
	return nil
}

func lastAttempt(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.CommunicationDeliveryHistoryRow,
) (uuid.UUID, error) {
	if row.Channel == "smtp" {
		a, err := q.LatestSMTPSubmissionAttempt(ctx, row.OperationID)
		if err != nil {
			return uuid.Nil, fmt.Errorf("read reviewed SMTP attempt: %w", err)
		}
		return a.ID, nil
	}
	if row.Channel == "teams" {
		a, err := q.TeamsWorkflowLatestAttempt(
			ctx,
			dbgen.TeamsWorkflowLatestAttemptParams{EventID: row.EventID, DeliveryID: row.ID},
		)
		if err != nil {
			return uuid.Nil, fmt.Errorf("read reviewed Teams attempt: %w", err)
		}
		return a.ID, nil
	}
	return uuid.Nil, access.ErrInvalid
}

func validCommand(revision int64, input Command) bool {
	if input.RequestID == uuid.Nil || revision < 1 || input.Reason == "" ||
		len(input.Reason) > 2000 ||
		strings.ContainsRune(input.Reason, 0) ||
		!slices.Contains(
			[]string{
				"retry",
				"cancel",
				"cancel_resend",
				"resolve_no_resend",
				"resend_acknowledged",
			},
			input.Action,
		) {
		return false
	}
	return true
}

func reviewReplay(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID, operationID uuid.UUID,
	revision int64,
	input Command,
) (bool, error) {
	prior, err := q.DeliveryReview(
		ctx,
		dbgen.DeliveryReviewParams{EventID: eventID, ID: input.RequestID},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read delivery review idempotency: %w", err)
	}
	if prior.OperationID != operationID || prior.ActorID != actor ||
		prior.ExpectedRevision != revision ||
		prior.Action != input.Action ||
		prior.Reason != input.Reason {
		return false, access.ErrConflict
	}
	return true, nil
}

func cancelResend(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.CommunicationDeliveryHistoryRow,
	op dbgen.Operation,
) error {
	if op.State != "pending" {
		return access.ErrConflict
	}
	id, err := lastAttempt(ctx, q, row)
	if err != nil {
		return err
	}
	grant, err := q.PendingDeliveryResend(
		ctx,
		dbgen.PendingDeliveryResendParams{OperationID: op.ID, PriorAttemptID: &id},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return access.ErrConflict
	}
	if err != nil {
		return fmt.Errorf("read pending resend cancellation: %w", err)
	}
	if grant.ExpectedRevision+1 != op.Revision {
		return access.ErrConflict
	}
	changed, err := q.CancelOperation(ctx, op.ID)
	if err != nil {
		return fmt.Errorf("cancel unspent resend authorisation: %w", err)
	}
	if changed != 1 {
		return access.ErrConflict
	}
	return nil
}
