package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

func (h handlers) GetHelpQueue(
	ctx context.Context,
	r contract.GetHelpQueueRequestObject,
) (contract.GetHelpQueueResponseObject, error) {
	p, err := h.Help.List(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("help queue: %w", err)
	}
	return convertDTO[contract.GetHelpQueue200JSONResponse](p)
}

func (h handlers) GetHelpRequest(
	ctx context.Context,
	r contract.GetHelpRequestRequestObject,
) (contract.GetHelpRequestResponseObject, error) {
	p, err := h.Help.Read(ctx, actor(ctx).ID, r.EventId, r.RequestId)
	if err != nil {
		return nil, fmt.Errorf("help request: %w", err)
	}
	return convertDTO[contract.GetHelpRequest200JSONResponse](p)
}

func (h handlers) CreateHelpRequest(
	ctx context.Context,
	r contract.CreateHelpRequestRequestObject,
) (contract.CreateHelpRequestResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	if revision(r.Params.IfMatch) != 0 {
		return nil, access.ErrConflict
	}
	input, err := convertDTO[help.Input](r.Body)
	if err != nil {
		return nil, err
	}
	input.CommandID = r.Params.IdempotencyKey
	p, err := h.Help.Create(ctx, actor(ctx).ID, r.EventId, r.RequestId, input)
	if err != nil {
		return nil, fmt.Errorf("create help request: %w", err)
	}
	return convertDTO[contract.CreateHelpRequest200JSONResponse](p)
}

func (h handlers) ChangeHelpRequest(
	ctx context.Context,
	r contract.ChangeHelpRequestRequestObject,
) (contract.ChangeHelpRequestResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[help.Command](r.Body)
	if err != nil {
		return nil, err
	}
	input.CommandID = r.Params.IdempotencyKey
	p, err := h.Help.Change(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.RequestId,
		revision(r.Params.IfMatch),
		input,
	)
	if err != nil {
		return nil, fmt.Errorf("change help request: %w", err)
	}
	return convertDTO[contract.ChangeHelpRequest200JSONResponse](p)
}

func (h handlers) GetHelpSettings(
	ctx context.Context,
	r contract.GetHelpSettingsRequestObject,
) (contract.GetHelpSettingsResponseObject, error) {
	p, err := h.Help.Settings(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("help settings: %w", err)
	}
	return convertDTO[contract.GetHelpSettings200JSONResponse](p)
}

func (h handlers) ConfigureHelp(
	ctx context.Context,
	r contract.ConfigureHelpRequestObject,
) (contract.ConfigureHelpResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[help.Settings](r.Body)
	if err != nil {
		return nil, err
	}
	p, err := h.Help.Configure(ctx, actor(ctx).ID, r.EventId, revision(r.Params.IfMatch), input)
	if err != nil {
		return nil, fmt.Errorf("configure help: %w", err)
	}
	return convertDTO[contract.ConfigureHelp200JSONResponse](p)
}
