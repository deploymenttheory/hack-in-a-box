package moderation

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/feedback"
)

// FeedbackActions delegates to the real #103 current published-feedback/action
// policy. It carries metadata only and never derives authority from a huddle.
type FeedbackActions struct{ Service *feedback.Service }

func (p FeedbackActions) ResolveModerationActionInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID, id uuid.UUID,
	version int64,
) (ActionState, error) {
	if p.Service == nil {
		return ActionState{}, access.ErrForbidden
	}
	source, err := p.Service.ResolveActionInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		teamID,
		id,
		version,
	)
	if err != nil {
		return ActionState{}, fmt.Errorf("resolve native feedback action: %w", err)
	}
	out := ActionState{
		ID:              source.ActionID,
		Version:         source.Version,
		FeedbackID:      source.FeedbackID,
		FeedbackVersion: source.FeedbackVersion,
		Status:          source.Status,
		OwnerEligible:   source.OwnerEligible,
	}
	out.Binding = binding(out)
	return out, nil
}

// AuthorizeModerationActionHistoryInTransaction preserves immutable metadata
// while delegating current reader authority to its real native owner.
func (p FeedbackActions) AuthorizeModerationActionHistoryInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
	state ActionState,
) error {
	if p.Service == nil {
		return access.ErrForbidden
	}
	if err := p.Service.AuthorizeRetainedActionInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		teamID,
		state.FeedbackID,
		state.ID,
		state.FeedbackVersion,
		state.Version,
	); err != nil {
		return fmt.Errorf("authorize retained native action: %w", err)
	}
	return nil
}
