package httpapi

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/feedback"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

func feedbackLimit(value *int) int32 {
	if value == nil {
		return 50
	}
	if *value < 1 || *value > 100 {
		return -1
	}
	return int32(*value)
}

func feedbackCursor(value *uuid.UUID) uuid.UUID {
	if value == nil {
		return uuid.Nil
	}
	return *value
}

func feedbackBefore(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func (h handlers) GetSharedFeedback(
	ctx context.Context,
	r contract.GetSharedFeedbackRequestObject,
) (contract.GetSharedFeedbackResponseObject, error) {
	p, err := h.Feedback.List(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.TeamId,
		feedbackCursor(r.Params.Cursor),
		feedbackLimit(r.Params.Limit),
	)
	if err != nil {
		return nil, fmt.Errorf("team shared feedback: %w", err)
	}
	return convertDTO[contract.GetSharedFeedback200JSONResponse](p)
}

func (h handlers) GetSharedFeedbackRecord(
	ctx context.Context,
	r contract.GetSharedFeedbackRecordRequestObject,
) (contract.GetSharedFeedbackRecordResponseObject, error) {
	p, err := h.Feedback.Read(ctx, actor(ctx).ID, r.EventId, r.FeedbackId)
	if err != nil {
		return nil, fmt.Errorf("shared feedback: %w", err)
	}
	return convertDTO[contract.GetSharedFeedbackRecord200JSONResponse](p)
}

func (h handlers) ChangeSharedFeedback(
	ctx context.Context,
	r contract.ChangeSharedFeedbackRequestObject,
) (contract.ChangeSharedFeedbackResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	in, err := convertDTO[feedback.Command](r.Body)
	if err != nil {
		return nil, err
	}
	p, err := h.Feedback.Change(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.FeedbackId,
		revision(r.Params.IfMatch),
		r.Params.IdempotencyKey,
		in,
	)
	if err != nil {
		return nil, fmt.Errorf("reviewed feedback mutation: %w", err)
	}
	return convertDTO[contract.ChangeSharedFeedback200JSONResponse](p)
}

func (h handlers) GetSharedFeedbackHistory(
	ctx context.Context,
	r contract.GetSharedFeedbackHistoryRequestObject,
) (contract.GetSharedFeedbackHistoryResponseObject, error) {
	p, err := h.Feedback.History(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.FeedbackId,
		feedbackBefore(r.Params.Before),
		feedbackLimit(r.Params.Limit),
	)
	if err != nil {
		return nil, fmt.Errorf("shared feedback history: %w", err)
	}
	return convertDTO[contract.GetSharedFeedbackHistory200JSONResponse](p)
}

func (h handlers) AcknowledgeSharedFeedback(
	ctx context.Context,
	r contract.AcknowledgeSharedFeedbackRequestObject,
) (contract.AcknowledgeSharedFeedbackResponseObject, error) {
	p, err := h.Feedback.Acknowledge(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.FeedbackId,
		revision(r.Params.IfMatch),
		r.Params.IdempotencyKey,
	)
	if err != nil {
		return nil, fmt.Errorf("acknowledge feedback release: %w", err)
	}
	return convertDTO[contract.AcknowledgeSharedFeedback200JSONResponse](p)
}

func (h handlers) GetSupportActionOwners(
	ctx context.Context,
	r contract.GetSupportActionOwnersRequestObject,
) (contract.GetSupportActionOwnersResponseObject, error) {
	p, err := h.Feedback.Candidates(ctx, actor(ctx).ID, r.EventId, r.FeedbackId)
	if err != nil {
		return nil, fmt.Errorf("current action owners: %w", err)
	}
	return convertDTO[contract.GetSupportActionOwners200JSONResponse](p)
}

func (h handlers) GetSupportActions(
	ctx context.Context,
	r contract.GetSupportActionsRequestObject,
) (contract.GetSupportActionsResponseObject, error) {
	p, err := h.Feedback.Actions(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.FeedbackId,
		feedbackCursor(r.Params.Cursor),
		feedbackLimit(r.Params.Limit),
	)
	if err != nil {
		return nil, fmt.Errorf("shared support actions: %w", err)
	}
	return convertDTO[contract.GetSupportActions200JSONResponse](p)
}

func (h handlers) ChangeSupportAction(
	ctx context.Context,
	r contract.ChangeSupportActionRequestObject,
) (contract.ChangeSupportActionResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	in, err := convertDTO[feedback.ActionCommand](r.Body)
	if err != nil {
		return nil, err
	}
	p, err := h.Feedback.ChangeAction(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.FeedbackId,
		r.ActionId,
		revision(r.Params.IfMatch),
		r.Params.IdempotencyKey,
		in,
	)
	if err != nil {
		return nil, fmt.Errorf("shared support action mutation: %w", err)
	}
	return convertDTO[contract.ChangeSupportAction200JSONResponse](p)
}

func (h handlers) GetSupportActionHistory(
	ctx context.Context,
	r contract.GetSupportActionHistoryRequestObject,
) (contract.GetSupportActionHistoryResponseObject, error) {
	p, err := h.Feedback.ActionHistory(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.FeedbackId,
		r.ActionId,
		feedbackBefore(r.Params.Before),
		feedbackLimit(r.Params.Limit),
	)
	if err != nil {
		return nil, fmt.Errorf("shared support action history: %w", err)
	}
	return convertDTO[contract.GetSupportActionHistory200JSONResponse](p)
}

func (h handlers) GetFeedbackSettings(
	ctx context.Context,
	r contract.GetFeedbackSettingsRequestObject,
) (contract.GetFeedbackSettingsResponseObject, error) {
	p, err := h.Feedback.Settings(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("feedback thresholds: %w", err)
	}
	return convertDTO[contract.GetFeedbackSettings200JSONResponse](p)
}

func (h handlers) ConfigureFeedbackSettings(
	ctx context.Context,
	r contract.ConfigureFeedbackSettingsRequestObject,
) (contract.ConfigureFeedbackSettingsResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	in, err := convertDTO[feedback.Settings](r.Body)
	if err != nil {
		return nil, err
	}
	p, err := h.Feedback.Configure(ctx, actor(ctx).ID, r.EventId, revision(r.Params.IfMatch), in)
	if err != nil {
		return nil, fmt.Errorf("configure feedback thresholds: %w", err)
	}
	return convertDTO[contract.ConfigureFeedbackSettings200JSONResponse](p)
}

func (h handlers) GetFeedbackFollowup(
	ctx context.Context,
	r contract.GetFeedbackFollowupRequestObject,
) (contract.GetFeedbackFollowupResponseObject, error) {
	p, err := h.Feedback.Followup(ctx, actor(ctx).ID, r.EventId, r.FollowupId)
	if err != nil {
		return nil, fmt.Errorf("current feedback follow-up: %w", err)
	}
	return convertDTO[contract.GetFeedbackFollowup200JSONResponse](p)
}
