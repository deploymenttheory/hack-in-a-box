package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/smtpintegration"
)

func (h handlers) GetCommunicationTestDelivery(
	ctx context.Context,
	r contract.GetCommunicationTestDeliveryRequestObject,
) (contract.GetCommunicationTestDeliveryResponseObject, error) {
	service := h.SMTP
	if service == nil {
		service = &smtpintegration.Service{Pool: h.Pool}
	}
	status, err := service.Status(ctx, actor(ctx).ID, r.EventId, r.TestId)
	if err != nil {
		return nil, fmt.Errorf("read SMTP test receipt: %w", err)
	}
	return convertDTO[contract.GetCommunicationTestDelivery200JSONResponse](status)
}
