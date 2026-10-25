package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/qualification"
)

func (h handlers) GetQualification(
	ctx context.Context,
	r contract.GetQualificationRequestObject,
) (contract.GetQualificationResponseObject, error) {
	if h.Qualification.Pool == nil {
		return nil, access.ErrForbidden
	}
	page, err := h.Qualification.Read(ctx, actor(ctx).ID, r.EventId, r.RunId)
	if err != nil {
		return nil, fmt.Errorf("read mentor qualification: %w", err)
	}
	return convertDTO[contract.GetQualification200JSONResponse](page)
}

func (h handlers) ChangeQualification(
	ctx context.Context,
	r contract.ChangeQualificationRequestObject,
) (contract.ChangeQualificationResponseObject, error) {
	if h.Qualification.Pool == nil {
		return nil, access.ErrForbidden
	}
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	expected, err := judgeRevision(r.Params.IfMatch)
	if err != nil {
		return nil, fmt.Errorf("qualification reviewed revision: %w", err)
	}
	input, err := convertDTO[qualification.Command](r.Body)
	if err != nil {
		return nil, fmt.Errorf("qualification reviewed intent: %w", err)
	}
	page, err := h.Qualification.Change(ctx, actor(ctx).ID, r.EventId, r.RunId, expected, input)
	if err != nil {
		return nil, fmt.Errorf("record mentor qualification: %w", err)
	}
	return convertDTO[contract.ChangeQualification200JSONResponse](page)
}

func (h handlers) GetQualificationReceipt(
	ctx context.Context,
	r contract.GetQualificationReceiptRequestObject,
) (contract.GetQualificationReceiptResponseObject, error) {
	if h.Qualification.Pool == nil {
		return nil, access.ErrForbidden
	}
	receipt, err := h.Qualification.OriginalReceipt(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.RunId,
		r.RequestId,
	)
	if err != nil {
		return nil, fmt.Errorf("recover original mentor qualification request: %w", err)
	}
	return convertDTO[contract.GetQualificationReceipt200JSONResponse](receipt)
}

func (h handlers) GetQualificationHistory(
	ctx context.Context,
	r contract.GetQualificationHistoryRequestObject,
) (contract.GetQualificationHistoryResponseObject, error) {
	if h.Qualification.Pool == nil {
		return nil, access.ErrForbidden
	}
	limit := int32(20)
	if r.Params.Limit != nil {
		if *r.Params.Limit < 1 || *r.Params.Limit > 100 {
			return nil, access.ErrInvalid
		}
		limit = int32(*r.Params.Limit)
	}
	history, err := h.Qualification.History(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.RunId,
		r.Params.Before,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("read private qualification history: %w", err)
	}
	return convertDTO[contract.GetQualificationHistory200JSONResponse](history)
}

func (h handlers) GetQualificationRounds(
	ctx context.Context,
	r contract.GetQualificationRoundsRequestObject,
) (contract.GetQualificationRoundsResponseObject, error) {
	if h.Qualification.Pool == nil {
		return nil, access.ErrForbidden
	}
	rounds, err := h.Qualification.Rounds(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("discover current qualification panel: %w", err)
	}
	return convertDTO[contract.GetQualificationRounds200JSONResponse](rounds)
}
