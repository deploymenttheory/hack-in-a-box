package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
)

func (h handlers) GetEventBranding(
	ctx context.Context,
	r contract.GetEventBrandingRequestObject,
) (contract.GetEventBrandingResponseObject, error) {
	result, err := h.Templates.ReadBrand(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("get branding: %w", err)
	}
	return convertDTO[contract.GetEventBranding200JSONResponse](result)
}

func (h handlers) UpdateEventBranding(
	ctx context.Context,
	r contract.UpdateEventBrandingRequestObject,
) (contract.UpdateEventBrandingResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[templates.Branding](r.Body)
	if err != nil {
		return nil, err
	}
	result, err := h.Templates.SaveBrand(
		ctx,
		actor(ctx).ID,
		r.EventId,
		revision(r.Params.IfMatch),
		input,
	)
	if err != nil {
		return nil, fmt.Errorf("save branding: %w", err)
	}
	return convertDTO[contract.UpdateEventBranding200JSONResponse](result)
}
