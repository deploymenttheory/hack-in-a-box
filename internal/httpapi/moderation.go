package httpapi

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/moderation"
)

func (h handlers) GetModerationHuddles(
	ctx context.Context,
	r contract.GetModerationHuddlesRequestObject,
) (contract.GetModerationHuddlesResponseObject, error) {
	if h.Moderation == nil {
		return nil, access.ErrForbidden
	}
	after := uuid.Nil
	if r.Params.Cursor != nil {
		after = *r.Params.Cursor
	}
	limit := int32(50)
	if r.Params.Limit != nil {
		if *r.Params.Limit < 1 || *r.Params.Limit > 100 {
			return nil, access.ErrInvalid
		}
		limit = int32(*r.Params.Limit)
	}
	result, err := h.Moderation.List(ctx, actor(ctx).ID, r.EventId, r.TeamId, after, limit)
	if err != nil {
		return nil, fmt.Errorf("read current intended huddles: %w", err)
	}
	return convertDTO[contract.GetModerationHuddles200JSONResponse](result)
}

func (h handlers) GetModerationHuddle(
	ctx context.Context,
	r contract.GetModerationHuddleRequestObject,
) (contract.GetModerationHuddleResponseObject, error) {
	if h.Moderation == nil {
		return nil, access.ErrForbidden
	}
	result, err := h.Moderation.Read(ctx, actor(ctx).ID, r.EventId, r.TeamId, r.HuddleId)
	if err != nil {
		return nil, fmt.Errorf("read current private huddle: %w", err)
	}
	return convertDTO[contract.GetModerationHuddle200JSONResponse](result)
}

func (h handlers) ChangeModerationHuddle(
	ctx context.Context,
	r contract.ChangeModerationHuddleRequestObject,
) (contract.ChangeModerationHuddleResponseObject, error) {
	if h.Moderation == nil {
		return nil, access.ErrForbidden
	}
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[moderation.Command](r.Body)
	if err != nil {
		return nil, err
	}
	result, err := h.Moderation.Change(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.TeamId,
		r.HuddleId,
		revision(r.Params.IfMatch),
		input,
	)
	if err != nil {
		return nil, fmt.Errorf("record explicit moderation decision: %w", err)
	}
	return convertDTO[contract.ChangeModerationHuddle200JSONResponse](result)
}

func (h handlers) GetModerationHuddleHistory(
	ctx context.Context,
	r contract.GetModerationHuddleHistoryRequestObject,
) (contract.GetModerationHuddleHistoryResponseObject, error) {
	if h.Moderation == nil {
		return nil, access.ErrForbidden
	}
	limit := int32(50)
	if r.Params.Limit != nil {
		if *r.Params.Limit < 1 || *r.Params.Limit > 100 {
			return nil, access.ErrInvalid
		}
		limit = int32(*r.Params.Limit)
	}
	result, err := h.Moderation.History(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.TeamId,
		r.HuddleId,
		r.Params.Cursor,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("read private moderation history: %w", err)
	}
	return convertDTO[contract.GetModerationHuddleHistory200JSONResponse](result)
}
