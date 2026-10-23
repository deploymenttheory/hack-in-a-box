package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// InstallNativeOutcomePlanInTransaction retains an owning domain's exact
// approval in the ordinary orchestration pipeline. It grants no generic campaign
// approval, writes no provider receipt and does not dispatch before the due time.
func (s Service) InstallNativeOutcomePlanInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, id uuid.UUID,
	version int64,
) error {
	q := dbgen.New(tx)
	event, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("lock native review schedule: %w", err)
	}
	if s.Composer == nil || s.Composer.OutcomePlans == nil {
		return access.ErrForbidden
	}
	state, err := s.Composer.OutcomePlans.InspectOutcomePlanInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		id,
		version,
	)
	if err != nil {
		return fmt.Errorf("verify native review schedule: %w", err)
	}
	if state.ID != id || state.Version != version || state.ActorID != actor {
		return access.ErrConflict
	}
	b := Binding{
		Definition: Definition{
			ID:                     id,
			CampaignID:             id,
			CampaignVersion:        version,
			Trigger:                "outcome.follow_up",
			SourceID:               &id,
			SourceVersion:          version,
			MaximumLatenessSeconds: int64(time.Hour / time.Second),
		},
		Version:   version,
		ActorID:   actor,
		DueAt:     state.DueAt,
		Cancelled: state.State != "approved",
	}
	if state.State == "approved" {
		if state.Approval == nil || state.Approval.Kind != "native_outcome_review" ||
			state.Approval.ID != id ||
			state.Approval.SourceID != id ||
			state.Approval.SourceVersion != version ||
			state.Approval.Binding == "" {
			return access.ErrForbidden
		}
		b.NativeOutcomeApproval = state.Approval
		b.CampaignApproval = state.Approval.Binding
		now := s.now()
		b.ApprovedAt = &now
		if event.Stage == "completed" || event.Stage == "archived" {
			b.Definition.RetainAfterArchive = true
			b.Definition.RetainUntil = &state.RetainUntil
		}
	}
	raw, err := json.Marshal(b)
	if err != nil {
		return fmt.Errorf("encode native review schedule: %w", err)
	}
	if err = q.SaveAutomationBinding(
		ctx,
		dbgen.SaveAutomationBindingParams{
			EventID:         eventID,
			ID:              id,
			Version:         version,
			CampaignID:      id,
			CampaignVersion: version,
			Document:        raw,
			ActorID:         actor,
			Reason:          "Native outcome review decision",
		},
	); err != nil {
		return fmt.Errorf("retain native review schedule: %w", err)
	}
	if err = q.CancelAutomationBindingOccurrences(
		ctx,
		dbgen.CancelAutomationBindingOccurrencesParams{
			EventID:        eventID,
			BindingID:      id,
			BindingVersion: version,
		},
	); err != nil {
		return fmt.Errorf("retire previous native review occurrences: %w", err)
	}
	return history(ctx, q, eventID, &id, nil, &actor, "native_outcome_review", state.State)
}
