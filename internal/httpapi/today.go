package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/today"
)

func (h handlers) GetToday(
	ctx context.Context,
	r contract.GetTodayRequestObject,
) (contract.GetTodayResponseObject, error) {
	page, err := (today.Service{Events: h.Events, Presentation: h.Presentation}).Read(
		ctx,
		actor(ctx).ID,
		r.EventId,
	)
	if err != nil {
		return nil, fmt.Errorf("read Today context: %w", err)
	}
	return convertDTO[contract.GetToday200JSONResponse](page)
}
