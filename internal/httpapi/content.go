package httpapi

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

func (h handlers) ListContent(
	ctx context.Context,
	r contract.ListContentRequestObject,
) (contract.ListContentResponseObject, error) {
	_, limit := page(nil, r.Params.Limit)
	cursor := uuid.Nil
	if r.Params.Cursor != nil {
		cursor = *r.Params.Cursor
	}
	preview := r.Params.Preview != nil && *r.Params.Preview
	search := ""
	if r.Params.Q != nil {
		search = *r.Params.Q
	}
	v, err := h.Content.Search(ctx, actor(ctx).ID, r.EventId, cursor, limit, preview, search)
	if err != nil {
		return nil, fmt.Errorf("content library: %w", err)
	}
	return convertDTO[contract.ListContent200JSONResponse](v)
}

func (h handlers) GetContent(
	ctx context.Context,
	r contract.GetContentRequestObject,
) (contract.GetContentResponseObject, error) {
	preview := r.Params.Preview != nil && *r.Params.Preview
	var version int64
	if r.Params.Version != nil {
		version = *r.Params.Version
	}
	v, err := h.Content.Read(ctx, actor(ctx).ID, r.EventId, r.ContentId, preview, version)
	if err != nil {
		return nil, fmt.Errorf("content page: %w", err)
	}
	return convertDTO[contract.GetContent200JSONResponse](v)
}

func (h handlers) SaveContent(
	ctx context.Context,
	r contract.SaveContentRequestObject,
) (contract.SaveContentResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[content.Input](r.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.Content.Save(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.ContentId,
		revision(r.Params.IfMatch),
		input,
	)
	if err != nil {
		return nil, fmt.Errorf("save page: %w", err)
	}
	return convertDTO[contract.SaveContent200JSONResponse](v)
}

func (h handlers) ContentAction(
	ctx context.Context,
	r contract.ContentActionRequestObject,
) (contract.ContentActionResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	v, err := h.Content.Action(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.ContentId,
		revision(r.Params.IfMatch),
		string(r.Body.Action),
		r.Body.Reason,
	)
	if err != nil {
		return nil, fmt.Errorf("page workflow: %w", err)
	}
	return convertDTO[contract.ContentAction200JSONResponse](v)
}

func (h handlers) ContentHistory(
	ctx context.Context,
	r contract.ContentHistoryRequestObject,
) (contract.ContentHistoryResponseObject, error) {
	var before int64
	if r.Params.Before != nil {
		before = *r.Params.Before
	}
	_, limit := page(nil, r.Params.Limit)
	v, err := h.Content.History(ctx, actor(ctx).ID, r.EventId, r.ContentId, before, limit)
	if err != nil {
		return nil, fmt.Errorf("page history: %w", err)
	}
	return convertDTO[contract.ContentHistory200JSONResponse](v)
}

func (h handlers) EventNotices(
	ctx context.Context,
	r contract.EventNoticesRequestObject,
) (contract.EventNoticesResponseObject, error) {
	var after int64
	if r.Params.After != nil {
		after = *r.Params.After
	}
	items, err := h.Content.Notices(ctx, actor(ctx).ID, r.EventId, after)
	if err != nil {
		return nil, fmt.Errorf("event notices: %w", err)
	}
	return convertDTO[contract.EventNotices200JSONResponse](struct {
		Items []content.Notice `json:"items"`
	}{items})
}

func (h handlers) ContentReadiness(
	ctx context.Context,
	r contract.ContentReadinessRequestObject,
) (contract.ContentReadinessResponseObject, error) {
	v, err := h.Content.PublicationStatus(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("publication readiness: %w", err)
	}
	return convertDTO[contract.ContentReadiness200JSONResponse](v)
}

func (h handlers) AcknowledgeContent(
	ctx context.Context,
	r contract.AcknowledgeContentRequestObject,
) (contract.AcknowledgeContentResponseObject, error) {
	v, err := h.Content.Acknowledge(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.ContentId,
		revision(r.Params.IfMatch),
	)
	if err != nil {
		return nil, fmt.Errorf("acknowledge content: %w", err)
	}
	return convertDTO[contract.AcknowledgeContent200JSONResponse](v)
}
