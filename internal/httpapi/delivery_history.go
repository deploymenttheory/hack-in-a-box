package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/deliveryhistory"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

func (h handlers) deliveryHistory() *deliveryhistory.Service {
	if h.Deliveries != nil {
		return h.Deliveries
	}
	return &deliveryhistory.Service{Pool: h.Pool}
}

func (h handlers) GetCommunicationDeliveries(
	ctx context.Context,
	r contract.GetCommunicationDeliveriesRequestObject,
) (contract.GetCommunicationDeliveriesResponseObject, error) {
	f := deliveryhistory.Filter{CampaignID: r.Params.CampaignId, Limit: 50}
	if r.Params.Limit != nil {
		if *r.Params.Limit < 1 || *r.Params.Limit > 100 {
			return nil, access.ErrInvalid
		}
		f.Limit = int32(*r.Params.Limit)
	}
	if r.Params.Channel != nil {
		f.Channel = string(*r.Params.Channel)
	}
	if (r.Params.BeforeAt == nil) != (r.Params.BeforeId == nil) {
		return nil, access.ErrInvalid
	}
	if r.Params.BeforeAt != nil {
		f.Before = &deliveryhistory.Cursor{At: *r.Params.BeforeAt, ID: *r.Params.BeforeId}
	}
	page, err := h.deliveryHistory().Read(ctx, actor(ctx).ID, r.EventId, f)
	if err != nil {
		return nil, fmt.Errorf("read communication delivery history: %w", err)
	}
	return convertDTO[contract.GetCommunicationDeliveries200JSONResponse](page)
}

func (h handlers) GetCommunicationDelivery(
	ctx context.Context,
	r contract.GetCommunicationDeliveryRequestObject,
) (contract.GetCommunicationDeliveryResponseObject, error) {
	before := int32(0)
	if r.Params.BeforeAttempt != nil {
		if *r.Params.BeforeAttempt < 0 || *r.Params.BeforeAttempt > 2147483647 {
			return nil, access.ErrInvalid
		}
		before = int32(*r.Params.BeforeAttempt)
	}
	detail, err := h.deliveryHistory().
		Detail(ctx, actor(ctx).ID, r.EventId, r.DeliveryId, before, r.Params.BeforeReview)
	if err != nil {
		return nil, fmt.Errorf("read communication delivery: %w", err)
	}
	return convertDTO[contract.GetCommunicationDelivery200JSONResponse](detail)
}

func (h handlers) ReviewCommunicationDelivery(
	ctx context.Context,
	r contract.ReviewCommunicationDeliveryRequestObject,
) (contract.ReviewCommunicationDeliveryResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[deliveryhistory.Command](r.Body)
	if err != nil {
		return nil, err
	}
	detail, err := h.deliveryHistory().
		Change(ctx, actor(ctx).ID, r.EventId, r.DeliveryId, revision(r.Params.IfMatch), input)
	if err != nil {
		return nil, fmt.Errorf("review communication delivery: %w", err)
	}
	return convertDTO[contract.ReviewCommunicationDelivery200JSONResponse](detail)
}
