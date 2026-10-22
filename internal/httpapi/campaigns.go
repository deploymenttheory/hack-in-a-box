package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

func (h handlers) GetCampaigns(
	ctx context.Context,
	r contract.GetCampaignsRequestObject,
) (contract.GetCampaignsResponseObject, error) {
	result, err := h.Communications.Campaigns(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("read campaigns: %w", err)
	}
	return convertDTO[contract.GetCampaigns200JSONResponse](result)
}

func (h handlers) GetCampaign(
	ctx context.Context,
	r contract.GetCampaignRequestObject,
) (contract.GetCampaignResponseObject, error) {
	result, err := h.Communications.Campaign(ctx, actor(ctx).ID, r.EventId, r.CampaignId)
	if err != nil {
		return nil, fmt.Errorf("read campaign: %w", err)
	}
	return convertDTO[contract.GetCampaign200JSONResponse](result)
}

func (h handlers) ChangeCampaign(
	ctx context.Context,
	r contract.ChangeCampaignRequestObject,
) (contract.ChangeCampaignResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[communications.CampaignCommand](r.Body)
	if err != nil {
		return nil, err
	}
	result, err := h.Communications.ChangeCampaign(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.CampaignId,
		revision(r.Params.IfMatch),
		input,
	)
	if err != nil {
		return nil, fmt.Errorf("change campaign: %w", err)
	}
	return convertDTO[contract.ChangeCampaign200JSONResponse](result)
}
