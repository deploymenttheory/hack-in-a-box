package httpapi

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
)

func (h handlers) CreateTemplate(
	ctx context.Context,
	r contract.CreateTemplateRequestObject,
) (contract.CreateTemplateResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[templates.Input](r.Body)
	if err != nil {
		return nil, err
	}
	result, err := h.Templates.Create(ctx, actor(ctx).ID, input)
	if err != nil {
		return nil, fmt.Errorf("CreateTemplate: %w", err)
	}
	response, err := convertDTO[contract.CreateTemplate200JSONResponse](result)
	return response, err
}

func (h handlers) EditTemplate(
	ctx context.Context,
	r contract.EditTemplateRequestObject,
) (contract.EditTemplateResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[templates.Input](r.Body)
	if err != nil {
		return nil, err
	}
	result, err := h.Templates.Edit(
		ctx,
		actor(ctx).ID,
		r.TemplateId,
		revision(r.Params.IfMatch),
		input,
	)
	if err != nil {
		return nil, fmt.Errorf("EditTemplate: %w", err)
	}
	response, err := convertDTO[contract.EditTemplate200JSONResponse](result)
	return response, err
}

func (h handlers) CloneTemplate(
	ctx context.Context,
	r contract.CloneTemplateRequestObject,
) (contract.CloneTemplateResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[templates.CloneInput](r.Body)
	if err != nil {
		return nil, err
	}
	result, err := h.Templates.CloneTemplate(ctx, actor(ctx).ID, r.TemplateId, input)
	if err != nil {
		return nil, fmt.Errorf("CloneTemplate: %w", err)
	}
	response, err := convertDTO[contract.CloneTemplate200JSONResponse](result)
	return response, err
}

func (h handlers) CloneEvent(
	ctx context.Context,
	r contract.CloneEventRequestObject,
) (contract.CloneEventResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[templates.CloneInput](r.Body)
	if err != nil {
		return nil, err
	}
	result, err := h.Templates.CloneEvent(ctx, actor(ctx).ID, r.EventId, input)
	if err != nil {
		return nil, fmt.Errorf("CloneEvent: %w", err)
	}
	response, err := convertDTO[contract.CloneEvent200JSONResponse](result)
	return response, err
}

func (h handlers) SaveEventDefinitions(
	ctx context.Context,
	r contract.SaveEventDefinitionsRequestObject,
) (contract.SaveEventDefinitionsResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[templates.Definitions](r.Body)
	if err != nil {
		return nil, err
	}
	result, err := h.Templates.SaveDefinitions(
		ctx,
		actor(ctx).ID,
		r.EventId,
		revision(r.Params.IfMatch),
		input,
	)
	if err != nil {
		return nil, fmt.Errorf("SaveEventDefinitions: %w", err)
	}
	response, err := convertDTO[contract.SaveEventDefinitions200JSONResponse](result)
	return response, err
}

func (h handlers) GetTemplate(
	ctx context.Context,
	r contract.GetTemplateRequestObject,
) (contract.GetTemplateResponseObject, error) {
	version := int32(0)
	if r.Params.Version != nil {
		if *r.Params.Version < 1 || *r.Params.Version > 1000 {
			return nil, access.ErrInvalid
		}
		version = int32(*r.Params.Version)
	}
	result, err := h.Templates.Read(ctx, actor(ctx).ID, r.TemplateId, version)
	if err != nil {
		return nil, fmt.Errorf("read template: %w", err)
	}
	response, err := convertDTO[contract.GetTemplate200JSONResponse](result)
	return response, err
}

func (h handlers) PublishTemplate(
	ctx context.Context,
	r contract.PublishTemplateRequestObject,
) (contract.PublishTemplateResponseObject, error) {
	if r.Body == nil || r.Body.Version < 1 || r.Body.Version > 1000 {
		return nil, access.ErrInvalid
	}
	result, err := h.Templates.Publish(
		ctx,
		actor(ctx).ID,
		r.TemplateId,
		revision(r.Params.IfMatch),
		int32(r.Body.Version),
	)
	if err != nil {
		return nil, fmt.Errorf("publish template: %w", err)
	}
	response, err := convertDTO[contract.PublishTemplate200JSONResponse](result)
	return response, err
}

func (h handlers) GetConfigurationSnapshot(
	ctx context.Context,
	r contract.GetConfigurationSnapshotRequestObject,
) (contract.GetConfigurationSnapshotResponseObject, error) {
	result, err := h.Templates.Snapshot(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("read configuration snapshot: %w", err)
	}
	response, err := convertDTO[contract.GetConfigurationSnapshot200JSONResponse](result)
	return response, err
}

func (h handlers) ListTemplates(
	ctx context.Context,
	r contract.ListTemplatesRequestObject,
) (contract.ListTemplatesResponseObject, error) {
	cursor := uuid.Nil
	if r.Params.After != nil {
		cursor = *r.Params.After
	}
	limit := int32(25)
	if r.Params.Limit != nil {
		if *r.Params.Limit < 1 || *r.Params.Limit > 50 {
			return nil, access.ErrInvalid
		}
		limit = int32(*r.Params.Limit)
	}
	rows, err := h.Templates.List(ctx, actor(ctx).ID, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	items, err := convertDTO[[]contract.Template](rows)
	if err != nil {
		return nil, err
	}
	result := contract.ListTemplates200JSONResponse{Items: items}
	if len(rows) == int(limit) {
		id := rows[len(rows)-1].ID
		result.NextCursor = &id
	}
	return result, nil
}

func (h handlers) ListTemplateStarters(
	ctx context.Context,
	_ contract.ListTemplateStartersRequestObject,
) (contract.ListTemplateStartersResponseObject, error) {
	rows, err := h.Templates.Starters(ctx, actor(ctx).ID)
	if err != nil {
		return nil, fmt.Errorf("list starter templates: %w", err)
	}
	items, err := convertDTO[[]contract.TemplateStarter](rows)
	if err != nil {
		return nil, err
	}
	return contract.ListTemplateStarters200JSONResponse{Items: items}, nil
}
