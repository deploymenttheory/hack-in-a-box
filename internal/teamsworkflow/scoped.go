package teamsworkflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

type QueueReport struct {
	Queued      int
	Preexisting map[string]int
}

func optionalOccurrence(scope communications.DeliveryScope) *uuid.UUID {
	if scope.OccurrenceID == uuid.Nil {
		return nil
	}
	return &scope.OccurrenceID
}

func (s Service) resolveScopedBroadcast(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	version int64,
	scope communications.DeliveryScope,
) (communications.BroadcastMessage, communications.ScopeAudience, error) {
	if s.ScopePolicy == nil || scope.OccurrenceID == uuid.Nil {
		return communications.BroadcastMessage{}, communications.ScopeAudience{}, access.ErrForbidden
	}
	audience, err := s.ScopePolicy.ResolveInTransaction(
		ctx,
		tx,
		eventID,
		campaignID,
		version,
		scope,
	)
	if err != nil {
		return communications.BroadcastMessage{}, audience, fmt.Errorf(
			"resolve Teams occurrence: %w",
			err,
		)
	}
	message, err := s.Composer.ResolveApprovedScopedBroadcastInTransaction(
		ctx,
		tx,
		eventID,
		campaignID,
		version,
		scope,
		s.ScopePolicy,
	)
	if err != nil {
		return message, audience, fmt.Errorf("resolve current public Teams projection: %w", err)
	}
	return message, audience, nil
}

func (s Service) QueueApprovedOccurrenceReportInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	version int64,
	scope communications.DeliveryScope,
) (QueueReport, error) {
	if s.Jobs == nil {
		return QueueReport{}, database.ErrDatabase
	}
	message, audience, err := s.resolveScopedBroadcast(ctx, tx, eventID, campaignID, version, scope)
	if err != nil {
		return QueueReport{}, err
	}
	id, created, err := s.queueApprovedBroadcast(
		ctx,
		tx,
		eventID,
		campaignID,
		version,
		message,
		scope,
		audience,
	)
	if err != nil {
		return QueueReport{}, err
	}
	report := QueueReport{Preexisting: map[string]int{}}
	if created {
		report.Queued = 1
		return report, nil
	}
	operation, err := dbgen.New(tx).Operation(ctx, id)
	if err != nil {
		return report, fmt.Errorf("read prior Teams operation: %w", err)
	}
	intent, err := dbgen.New(tx).
		TeamsWorkflowOccurrenceDelivery(ctx, dbgen.TeamsWorkflowOccurrenceDeliveryParams{EventID: eventID, OccurrenceID: optionalOccurrence(scope)})
	if err != nil {
		return report, fmt.Errorf("read prior Teams intent: %w", err)
	}
	state := operation.State
	last, err := dbgen.New(tx).
		TeamsWorkflowLatestAttempt(ctx, dbgen.TeamsWorkflowLatestAttemptParams{EventID: eventID, DeliveryID: intent.ID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return report, fmt.Errorf("read prior Teams receipt: %w", err)
	}
	if err == nil && last.Outcome.Valid {
		state = last.Outcome.String
	} else if state == "delivered" {
		state = "ambiguous"
	}
	report.Preexisting[state] = 1
	return report, nil
}

func (s Service) checkSettled(
	ctx context.Context,
	q *dbgen.Queries,
	intent dbgen.TeamsWorkflowDelivery,
) error {
	operation, err := q.Operation(ctx, intent.OperationID)
	if err != nil {
		return fmt.Errorf("review Teams operation: %w", err)
	}
	if operation.State == "pending" || operation.State == "running" {
		return access.ErrConflict
	}
	last, err := q.TeamsWorkflowLatestAttempt(
		ctx,
		dbgen.TeamsWorkflowLatestAttemptParams{EventID: intent.EventID, DeliveryID: intent.ID},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("review Teams effect barrier: %w", err)
	}
	if !last.Outcome.Valid {
		return access.ErrConflict
	}
	return nil
}

func (s Service) CheckOccurrenceReviewInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	version int64,
	scope communications.DeliveryScope,
) error {
	q := dbgen.New(tx)
	if _, err := q.LockEvent(ctx, eventID); err != nil {
		return fmt.Errorf("lock Teams occurrence review: %w", err)
	}
	intent, err := q.TeamsWorkflowOccurrenceDelivery(
		ctx,
		dbgen.TeamsWorkflowOccurrenceDeliveryParams{
			EventID:      eventID,
			OccurrenceID: optionalOccurrence(scope),
		},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read Teams review intent: %w", err)
	}
	if intent.CampaignID == nil || *intent.CampaignID != campaignID ||
		intent.CampaignVersion.Int64 != version {
		return access.ErrConflict
	}
	return s.checkSettled(ctx, q, intent)
}

func (s Service) resolveIntentBroadcast(
	ctx context.Context,
	tx pgx.Tx,
	intent dbgen.TeamsWorkflowDelivery,
) (communications.BroadcastMessage, error) {
	if intent.CampaignID == nil {
		return communications.BroadcastMessage{}, access.ErrForbidden
	}
	if intent.OccurrenceID == nil {
		message, err := s.Composer.ResolveApprovedBroadcastInTransaction(
			ctx,
			tx,
			intent.EventID,
			*intent.CampaignID,
		)
		if err != nil {
			return message, fmt.Errorf("resolve one-shot Teams source: %w", err)
		}
		return message, nil
	}
	message, audience, err := s.resolveScopedBroadcast(
		ctx,
		tx,
		intent.EventID,
		*intent.CampaignID,
		intent.CampaignVersion.Int64,
		communications.DeliveryScope{OccurrenceID: *intent.OccurrenceID},
	)
	if err != nil {
		return message, err
	}
	var retained *communications.ScopeApproval
	if json.Unmarshal(intent.ScopeApproval, &retained) != nil ||
		audience.Binding != intent.ScopeBinding ||
		!communications.SameScopeApproval(retained, message.ScopeApproval) {
		return message, access.ErrConflict
	}
	return message, nil
}
