package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

func (h handlers) GetCommunications(
	ctx context.Context,
	r contract.GetCommunicationsRequestObject,
) (contract.GetCommunicationsResponseObject, error) {
	result, err := h.Communications.Read(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("read communications: %w", err)
	}
	return convertDTO[contract.GetCommunications200JSONResponse](result)
}

func (h handlers) ChangeCommunications(
	ctx context.Context,
	r contract.ChangeCommunicationsRequestObject,
) (contract.ChangeCommunicationsResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[communications.Command](r.Body)
	if err != nil {
		return nil, err
	}
	result, err := h.Communications.Change(
		ctx,
		actor(ctx).ID,
		r.EventId,
		revision(r.Params.IfMatch),
		input,
	)
	if err != nil {
		return nil, fmt.Errorf("change communications: %w", err)
	}
	return convertDTO[contract.ChangeCommunications200JSONResponse](result)
}
