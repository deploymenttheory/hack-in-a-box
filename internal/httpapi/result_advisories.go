package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/results"
)

func (h handlers) GetMentorAdvisories(
	ctx context.Context,
	r contract.GetMentorAdvisoriesRequestObject,
) (contract.GetMentorAdvisoriesResponseObject, error) {
	if h.Results.Pool == nil {
		return nil, access.ErrForbidden
	}
	value, err := h.Results.MentorAdvisories(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("read own mentor advisory: %w", err)
	}
	return convertDTO[contract.GetMentorAdvisories200JSONResponse](value)
}

func (h handlers) RecordMentorAdvisory(
	ctx context.Context,
	r contract.RecordMentorAdvisoryRequestObject,
) (contract.RecordMentorAdvisoryResponseObject, error) {
	if h.Results.Pool == nil {
		return nil, access.ErrForbidden
	}
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	expected, err := judgeRevision(r.Params.IfMatch)
	if err != nil {
		return nil, fmt.Errorf("advisory revision: %w", err)
	}
	command, err := convertDTO[results.AdvisoryCommand](r.Body)
	if err != nil {
		return nil, fmt.Errorf("advisory command: %w", err)
	}
	value, err := h.Results.Advise(ctx, actor(ctx).ID, r.EventId, expected, command)
	if err != nil {
		return nil, fmt.Errorf("record own mentor advisory: %w", err)
	}
	return convertDTO[contract.RecordMentorAdvisory200JSONResponse](value)
}

func (h handlers) GetMentorAdvisoryReceipt(
	ctx context.Context,
	r contract.GetMentorAdvisoryReceiptRequestObject,
) (contract.GetMentorAdvisoryReceiptResponseObject, error) {
	if h.Results.Pool == nil {
		return nil, access.ErrForbidden
	}
	value, err := h.Results.MentorAdvisoryReceipt(ctx, actor(ctx).ID, r.EventId, r.RequestId)
	if err != nil {
		return nil, fmt.Errorf("recover advisory acknowledgement: %w", err)
	}
	return convertDTO[contract.GetMentorAdvisoryReceipt200JSONResponse](value)
}
