package httpapi

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/objectives"
)

func (h handlers) ListObjectives(
	ctx context.Context,
	r contract.ListObjectivesRequestObject,
) (contract.ListObjectivesResponseObject, error) {
	cursor := uuid.Nil
	if r.Params.Cursor != nil {
		cursor = *r.Params.Cursor
	}
	_, limit := page(nil, r.Params.Limit)
	v, err := h.Objectives.List(ctx, actor(ctx).ID, r.EventId, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("list objective configurations: %w", err)
	}
	return convertDTO[contract.ListObjectives200JSONResponse](v)
}

func (h handlers) GetObjective(
	ctx context.Context,
	r contract.GetObjectiveRequestObject,
) (contract.GetObjectiveResponseObject, error) {
	var version int64
	if r.Params.Version != nil {
		version = *r.Params.Version
	}
	v, err := h.Objectives.Read(ctx, actor(ctx).ID, r.EventId, r.ObjectiveId, version)
	if err != nil {
		return nil, fmt.Errorf("read objective configuration: %w", err)
	}
	return convertDTO[contract.GetObjective200JSONResponse](v)
}

func (h handlers) ChangeObjective(
	ctx context.Context,
	r contract.ChangeObjectiveRequestObject,
) (contract.ChangeObjectiveResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	c, err := convertDTO[objectives.Command](r.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.Objectives.Change(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.ObjectiveId,
		revision(r.Params.IfMatch),
		c,
	)
	if err != nil {
		return nil, fmt.Errorf("change objective configuration: %w", err)
	}
	return convertDTO[contract.ChangeObjective200JSONResponse](v)
}

func (h handlers) GetObjectiveHistory(
	ctx context.Context,
	r contract.GetObjectiveHistoryRequestObject,
) (contract.GetObjectiveHistoryResponseObject, error) {
	var before int64
	if r.Params.BeforeVersion != nil {
		before = *r.Params.BeforeVersion
	}
	_, limit := page(nil, r.Params.Limit)
	v, err := h.Objectives.History(ctx, actor(ctx).ID, r.EventId, r.ObjectiveId, before, limit)
	if err != nil {
		return nil, fmt.Errorf("read objective version history: %w", err)
	}
	return convertDTO[contract.GetObjectiveHistory200JSONResponse](v)
}

func (h handlers) GetObjectiveOwners(
	ctx context.Context,
	r contract.GetObjectiveOwnersRequestObject,
) (contract.GetObjectiveOwnersResponseObject, error) {
	v, err := h.Objectives.Owners(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("read objective owner candidates: %w", err)
	}
	return convertDTO[contract.GetObjectiveOwners200JSONResponse](v)
}

func (h handlers) GetDisclosedObjective(
	ctx context.Context,
	r contract.GetDisclosedObjectiveRequestObject,
) (contract.GetDisclosedObjectiveResponseObject, error) {
	v, err := h.Objectives.Public(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.ContentId,
		objectives.Reference{ID: r.ObjectiveId, Version: r.Params.Version},
	)
	if err != nil {
		return nil, fmt.Errorf("read native disclosed objective: %w", err)
	}
	return convertDTO[contract.GetDisclosedObjective200JSONResponse](v)
}
