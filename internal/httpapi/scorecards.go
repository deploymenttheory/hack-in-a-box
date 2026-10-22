package httpapi

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
)

func (h handlers) GetScorecardCheckpoints(
	ctx context.Context,
	r contract.GetScorecardCheckpointsRequestObject,
) (contract.GetScorecardCheckpointsResponseObject, error) {
	after := uuid.Nil
	if r.Params.Cursor != nil {
		after = *r.Params.Cursor
	}
	count := 0
	if r.Params.Limit != nil {
		count = *r.Params.Limit
	}
	result, err := h.Scorecards.Checkpoints(ctx, actor(ctx).ID, r.EventId, after, count)
	if err != nil {
		return nil, fmt.Errorf("read private checkpoint configuration: %w", err)
	}
	return convertDTO[contract.GetScorecardCheckpoints200JSONResponse](result)
}

func (h handlers) ConfigureScorecardCheckpoint(
	ctx context.Context,
	r contract.ConfigureScorecardCheckpointRequestObject,
) (contract.ConfigureScorecardCheckpointResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[scorecards.CheckpointCommand](r.Body)
	if err != nil {
		return nil, err
	}
	result, err := h.Scorecards.Configure(
		ctx,
		actor(ctx).ID,
		r.EventId,
		revision(r.Params.IfMatch),
		input,
	)
	if err != nil {
		return nil, fmt.Errorf("configure private scorecard checkpoint: %w", err)
	}
	return convertDTO[contract.ConfigureScorecardCheckpoint200JSONResponse](result)
}

func (h handlers) GetOwnScorecards(
	ctx context.Context,
	r contract.GetOwnScorecardsRequestObject,
) (contract.GetOwnScorecardsResponseObject, error) {
	after := uuid.Nil
	if r.Params.Cursor != nil {
		after = *r.Params.Cursor
	}
	count := 0
	if r.Params.Limit != nil {
		count = *r.Params.Limit
	}
	result, err := h.Scorecards.Read(ctx, actor(ctx).ID, r.EventId, r.TeamId, after, count)
	if err != nil {
		return nil, fmt.Errorf("read independent private scorecards: %w", err)
	}
	return convertDTO[contract.GetOwnScorecards200JSONResponse](result)
}

func (h handlers) ChangeOwnScorecard(
	ctx context.Context,
	r contract.ChangeOwnScorecardRequestObject,
) (contract.ChangeOwnScorecardResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[scorecards.CardCommand](r.Body)
	if err != nil {
		return nil, err
	}
	result, err := h.Scorecards.Change(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.TeamId,
		revision(r.Params.IfMatch),
		input,
	)
	if err != nil {
		return nil, fmt.Errorf("change independent scorecard: %w", err)
	}
	return convertDTO[contract.ChangeOwnScorecard200JSONResponse](result)
}

func (h handlers) GetOwnScorecardHistory(
	ctx context.Context,
	r contract.GetOwnScorecardHistoryRequestObject,
) (contract.GetOwnScorecardHistoryResponseObject, error) {
	count := 0
	if r.Params.Limit != nil {
		count = *r.Params.Limit
	}
	result, err := h.Scorecards.History(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.TeamId,
		r.CardId,
		r.Params.Cursor,
		count,
	)
	if err != nil {
		return nil, fmt.Errorf("read private scorecard author history: %w", err)
	}
	return convertDTO[contract.GetOwnScorecardHistory200JSONResponse](result)
}

func (h handlers) GetModerationScorecards(
	ctx context.Context,
	r contract.GetModerationScorecardsRequestObject,
) (contract.GetModerationScorecardsResponseObject, error) {
	result, err := h.Scorecards.Panel(ctx, actor(ctx).ID, r.EventId, r.TeamId, r.Params.ContextId)
	if err != nil {
		return nil, fmt.Errorf("read actual scorecard moderation context: %w", err)
	}
	return convertDTO[contract.GetModerationScorecards200JSONResponse](result)
}

func (h handlers) GetAssignedScorecardCheckpoints(
	ctx context.Context,
	r contract.GetAssignedScorecardCheckpointsRequestObject,
) (contract.GetAssignedScorecardCheckpointsResponseObject, error) {
	after := uuid.Nil
	if r.Params.Cursor != nil {
		after = *r.Params.Cursor
	}
	count := 0
	if r.Params.Limit != nil {
		count = *r.Params.Limit
	}
	result, err := h.Scorecards.TeamCheckpoints(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.TeamId,
		after,
		count,
	)
	if err != nil {
		return nil, fmt.Errorf("read current assigned checkpoint metadata: %w", err)
	}
	return convertDTO[contract.GetAssignedScorecardCheckpoints200JSONResponse](result)
}
