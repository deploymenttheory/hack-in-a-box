package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

func (h handlers) GetPublishedRubric(
	ctx context.Context,
	r contract.GetPublishedRubricRequestObject,
) (contract.GetPublishedRubricResponseObject, error) {
	v, err := h.Rubrics.Public(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("common published rubric: %w", err)
	}
	return convertDTO[contract.GetPublishedRubric200JSONResponse](v)
}

func (h handlers) GetRubricConfiguration(
	ctx context.Context,
	r contract.GetRubricConfigurationRequestObject,
) (contract.GetRubricConfigurationResponseObject, error) {
	var version int64
	if r.Params.Version != nil {
		version = *r.Params.Version
	}
	v, err := h.Rubrics.Configuration(ctx, actor(ctx).ID, r.EventId, version)
	if err != nil {
		return nil, fmt.Errorf("organiser rubric configuration: %w", err)
	}
	return convertDTO[contract.GetRubricConfiguration200JSONResponse](v)
}

func (h handlers) ChangeRubricConfiguration(
	ctx context.Context,
	r contract.ChangeRubricConfigurationRequestObject,
) (contract.ChangeRubricConfigurationResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	command, err := convertDTO[rubric.Command](r.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.Rubrics.Change(ctx, actor(ctx).ID, r.EventId, revision(r.Params.IfMatch), command)
	if err != nil {
		return nil, fmt.Errorf("change organiser rubric: %w", err)
	}
	return convertDTO[contract.ChangeRubricConfiguration200JSONResponse](v)
}

func (h handlers) GetRubricHistory(
	ctx context.Context,
	r contract.GetRubricHistoryRequestObject,
) (contract.GetRubricHistoryResponseObject, error) {
	var cursor int64
	if r.Params.Cursor != nil {
		cursor = *r.Params.Cursor
	}
	_, limit := page(nil, r.Params.Limit)
	v, err := h.Rubrics.History(ctx, actor(ctx).ID, r.EventId, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("rubric version history: %w", err)
	}
	return convertDTO[contract.GetRubricHistory200JSONResponse](v)
}
