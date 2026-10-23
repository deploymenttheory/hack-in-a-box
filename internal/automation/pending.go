package automation

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func (s Service) reconcilePending(
	ctx context.Context,
	tx pgx.Tx,
	event dbgen.Event,
	paused bool,
) error {
	q := dbgen.New(tx)
	cursor := uuid.Nil
	for {
		rows, err := q.PendingAutomationOccurrences(
			ctx,
			dbgen.PendingAutomationOccurrencesParams{EventID: event.ID, AfterID: cursor},
		)
		if err != nil {
			return fmt.Errorf("read persisted pending communications: %w", err)
		}
		if len(rows) == 0 {
			return nil
		}
		for _, row := range rows {
			if err = s.reconcilePendingOccurrence(ctx, tx, event, paused, row); err != nil {
				return err
			}
			cursor = row.ID
		}
	}
}

func (s Service) recordPendingState(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.CommunicationOccurrence,
	state, reason string,
) error {
	if row.State == state && row.Reason == reason {
		return nil
	}
	if err := q.SetAutomationOccurrenceState(
		ctx,
		dbgen.SetAutomationOccurrenceStateParams{
			EventID:     row.EventID,
			ID:          row.ID,
			State:       state,
			Reason:      reason,
			ReviewUntil: row.ReviewUntil,
		},
	); err != nil {
		return fmt.Errorf("retain pending communication review state: %w", err)
	}
	if state == "blocked" || state == "missed" || state == "cancelled" {
		if row.OperationID != nil {
			if _, err := q.CancelOperation(ctx, *row.OperationID); err != nil {
				return fmt.Errorf("retire obsolete pending orchestration: %w", err)
			}
		}
	}
	return history(ctx, q, row.EventID, &row.BindingID, &row.ID, nil, state, reason)
}

func (s Service) reconcilePendingOccurrence(
	ctx context.Context,
	tx pgx.Tx,
	event dbgen.Event,
	paused bool,
	row dbgen.CommunicationOccurrence,
) error {
	q := dbgen.New(tx)
	binding, err := loadBinding(ctx, q, event.ID, row.BindingID)
	if err != nil {
		return err
	}
	campaign, err := currentCampaign(ctx, q, event.ID, binding.Definition.CampaignID)
	if err != nil {
		return err
	}
	if campaign.Cancelled || binding.Cancelled ||
		!event.WritesAllowed && !binding.Definition.RetainAfterArchive {
		return s.recordPendingState(ctx, q, row, "cancelled", "campaign_or_event_closed")
	}
	if campaign.Version != binding.Definition.CampaignVersion ||
		binding.Version != row.BindingVersion {
		return s.recordPendingState(
			ctx,
			q,
			row,
			"blocked",
			"current_approval_or_source_review_required",
		)
	}
	until := row.Deadline
	if row.ReviewUntil.Valid {
		until = row.ReviewUntil.Time
	}
	if s.now().After(until) {
		return s.recordPendingState(ctx, q, row, "missed", "dispatch_window_missed")
	}
	if paused {
		return s.recordPendingState(ctx, q, row, "paused", "event_automation_paused")
	}
	if row.State == "paused" {
		if err = s.recordPendingState(ctx, q, row, "pending", ""); err != nil {
			return err
		}
		row.State = "pending"
		row.Reason = ""
	}
	err = s.checkPendingSource(ctx, tx, binding, row)
	if errors.Is(err, communications.ErrUnresolved) || errors.Is(err, access.ErrForbidden) ||
		errors.Is(err, access.ErrConflict) ||
		errors.Is(err, access.ErrNotFound) ||
		errors.Is(err, pgx.ErrNoRows) {
		return s.recordPendingState(
			ctx,
			q,
			row,
			"blocked",
			"current_approval_or_source_review_required",
		)
	}
	return err
}

func (s Service) checkPendingSource(
	ctx context.Context,
	tx pgx.Tx,
	binding Binding,
	row dbgen.CommunicationOccurrence,
) error {
	scope := communications.DeliveryScope{OccurrenceID: row.ID}
	if _, err := s.resolveScope(
		ctx,
		tx,
		row.EventID,
		binding.Definition.CampaignID,
		binding.Definition.CampaignVersion,
		scope,
		false,
	); err != nil {
		return err
	}
	if !s.now().Before(row.DueAt) {
		_, err := s.Composer.ResolveApprovedScopedCampaignInTransaction(
			ctx,
			tx,
			row.EventID,
			binding.Definition.CampaignID,
			binding.Definition.CampaignVersion,
			scope,
			s,
		)
		if err != nil {
			return fmt.Errorf("recheck pending scoped execution: %w", err)
		}
		return nil
	}
	if binding.PublicationApprovalID == nil && binding.NativeOutcomeApproval == nil &&
		!binding.Definition.RetainAfterArchive {
		_, err := s.Composer.ReviewCampaignApprovalInTransaction(
			ctx,
			tx,
			row.EventID,
			binding.Definition.CampaignID,
		)
		if err != nil {
			return fmt.Errorf("recheck pending campaign approval: %w", err)
		}
		return nil
	}
	return nil
}
