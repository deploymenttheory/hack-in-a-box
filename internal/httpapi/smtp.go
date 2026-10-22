package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/mailcontact"
)

func (h handlers) GetCommunicationTestDelivery(
	ctx context.Context,
	r contract.GetCommunicationTestDeliveryRequestObject,
) (contract.GetCommunicationTestDeliveryResponseObject, error) {
	if h.SMTP == nil {
		return nil, mailcontact.ErrUnavailable
	}
	status, err := h.SMTP.Status(ctx, actor(ctx).ID, r.EventId, r.TestId)
	if err != nil {
		return nil, fmt.Errorf("read SMTP test receipt: %w", err)
	}
	return convertDTO[contract.GetCommunicationTestDelivery200JSONResponse](status)
}
