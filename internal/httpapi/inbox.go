package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

func (h handlers) GetInbox(
	ctx context.Context,
	r contract.GetInboxRequestObject,
) (contract.GetInboxResponseObject, error) {
	limit := 30
	if r.Params.Limit != nil {
		limit = *r.Params.Limit
	}
	result, err := h.Inbox.List(ctx, actor(ctx).ID, r.Params.EventId, r.Params.Cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("inbox request: %w", err)
	}
	return convertDTO[contract.GetInbox200JSONResponse](result)
}

func (h handlers) GetInboxNotification(
	ctx context.Context,
	r contract.GetInboxNotificationRequestObject,
) (contract.GetInboxNotificationResponseObject, error) {
	result, err := h.Inbox.Open(ctx, actor(ctx).ID, r.NotificationId, nil)
	if err != nil {
		return nil, fmt.Errorf("inbox request: %w", err)
	}
	return convertDTO[contract.GetInboxNotification200JSONResponse](result)
}

func (h handlers) SetInboxRead(
	ctx context.Context,
	r contract.SetInboxReadRequestObject,
) (contract.SetInboxReadResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	result, err := h.Inbox.Open(ctx, actor(ctx).ID, r.NotificationId, &r.Body.Read)
	if err != nil {
		return nil, fmt.Errorf("inbox request: %w", err)
	}
	return convertDTO[contract.SetInboxRead200JSONResponse](result)
}
