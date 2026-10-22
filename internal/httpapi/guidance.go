package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

func (h handlers) GetEventGuidance(
	ctx context.Context,
	r contract.GetEventGuidanceRequestObject,
) (contract.GetEventGuidanceResponseObject, error) {
	page, err := h.Content.Guidance(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("event guidance: %w", err)
	}
	return convertDTO[contract.GetEventGuidance200JSONResponse](page)
}
