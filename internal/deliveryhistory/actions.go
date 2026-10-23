package deliveryhistory

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
)

// These are current native capabilities only, not durable permission to act.
// Change serializes and rechecks each decision against its expected revision.
func (s Service) availableActions(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	row dbgen.CommunicationDeliveryHistoryRow,
	paused, archived bool,
) []string {
	allowed := []string{}
	if archived {
		return allowed
	}
	op, err := q.Operation(ctx, row.OperationID)
	if err != nil || op.Cancelled || op.State == "running" || op.LeaseToken != nil {
		return allowed
	}
	state := item(row).State
	if op.State == "pending" {
		if state == "never_attempted" || state == "retryable" {
			allowed = append(allowed, "cancel")
		}
		if slices.Contains([]string{"provider_accepted", "ambiguous"}, state) {
			id, readErr := lastAttempt(ctx, q, row)
			if readErr == nil {
				grant, readErr := q.PendingDeliveryResend(
					ctx,
					dbgen.PendingDeliveryResendParams{OperationID: op.ID, PriorAttemptID: &id},
				)
				if readErr == nil && grant.ExpectedRevision+1 == op.Revision {
					allowed = append(allowed, "cancel_resend")
				}
			}
		}
	}
	if state == "ambiguous" && op.State == "ambiguous" {
		allowed = append(allowed, "resolve_no_resend")
	}
	if paused || s.Jobs == nil || s.Providers[row.Channel] == nil ||
		(row.Purpose != "campaign" && row.Purpose != "broadcast") {
		return allowed
	}
	if err = s.Providers[row.Channel].ValidateRecoveryInTransaction(
		ctx,
		tx,
		jobs.Delivery{ID: op.ID, EventID: op.EventID, TargetID: op.TargetID, Key: op.ID.String()},
	); err != nil {
		return allowed
	}
	if op.State == "pending" && state == "retryable" {
		allowed = append(allowed, "retry")
	}
	if slices.Contains([]string{"ambiguous", "delivered"}, op.State) &&
		slices.Contains([]string{"ambiguous", "provider_accepted"}, state) {
		allowed = append(allowed, "resend_acknowledged")
	}
	return allowed
}
