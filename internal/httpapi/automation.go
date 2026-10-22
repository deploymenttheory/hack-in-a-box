package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/automation"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

func (h handlers) GetCommunicationAutomation(
	ctx context.Context,
	r contract.GetCommunicationAutomationRequestObject,
) (contract.GetCommunicationAutomationResponseObject, error) {
	if h.Automation == nil {
		return nil, database.ErrDatabase
	}
	page, err := h.Automation.Read(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("read communication automation: %w", err)
	}
	return convertDTO[contract.GetCommunicationAutomation200JSONResponse](page)
}

func (h handlers) ChangeCommunicationAutomation(
	ctx context.Context,
	r contract.ChangeCommunicationAutomationRequestObject,
) (contract.ChangeCommunicationAutomationResponseObject, error) {
	if h.Automation == nil {
		return nil, database.ErrDatabase
	}
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[automation.Command](r.Body)
	if err != nil {
		return nil, err
	}
	page, preview, err := h.Automation.Change(
		ctx,
		actor(ctx).ID,
		r.EventId,
		revision(r.Params.IfMatch),
		input,
	)
	if err != nil {
		return nil, fmt.Errorf("change communication automation: %w", err)
	}
	return convertDTO[contract.ChangeCommunicationAutomation200JSONResponse](struct {
		Page    automation.Page                        `json:"page"`
		Preview *communications.PublicationPlanPreview `json:"publication_preview"`
	}{Page: page, Preview: preview})
}
