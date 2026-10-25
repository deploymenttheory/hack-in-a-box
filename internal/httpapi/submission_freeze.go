package httpapi

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissionfreeze"
)

func (h handlers) GetSubmissionFreeze(
	ctx context.Context,
	r contract.GetSubmissionFreezeRequestObject,
) (contract.GetSubmissionFreezeResponseObject, error) {
	p, err := h.SubmissionFreeze.Read(ctx, actor(ctx).ID, r.EventId, r.TeamId)
	if err != nil {
		return nil, fmt.Errorf("submission freeze read: %w", err)
	}
	return convertDTO[contract.GetSubmissionFreeze200JSONResponse](p)
}

func (h handlers) ChangeSubmissionFreeze(
	ctx context.Context,
	r contract.ChangeSubmissionFreezeRequestObject,
) (contract.ChangeSubmissionFreezeResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	c, err := convertDTO[submissionfreeze.Command](r.Body)
	if err != nil {
		return nil, err
	}
	c.ExpectedRevision, err = strconv.ParseInt(strings.Trim(r.Params.IfMatch, "\""), 10, 64)
	if err != nil || c.ExpectedRevision < 0 {
		return nil, access.ErrInvalid
	}
	p, err := h.SubmissionFreeze.Change(ctx, actor(ctx).ID, r.EventId, r.TeamId, c)
	if err != nil {
		return nil, fmt.Errorf("submission freeze change: %w", err)
	}
	return convertDTO[contract.ChangeSubmissionFreeze200JSONResponse](p)
}

func (h handlers) GetSubmissionFreezeReceipt(
	ctx context.Context,
	r contract.GetSubmissionFreezeReceiptRequestObject,
) (contract.GetSubmissionFreezeReceiptResponseObject, error) {
	p, err := h.SubmissionFreeze.OriginalReceipt(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.TeamId,
		r.RequestId,
	)
	if err != nil {
		return nil, fmt.Errorf("submission freeze receipt: %w", err)
	}
	return convertDTO[contract.GetSubmissionFreezeReceipt200JSONResponse](p)
}

func (h handlers) GetSubmissionFreezePushEvidence(
	ctx context.Context,
	r contract.GetSubmissionFreezePushEvidenceRequestObject,
) (contract.GetSubmissionFreezePushEvidenceResponseObject, error) {
	cursor := uuid.Nil
	if r.Params.After != nil {
		cursor = *r.Params.After
	}
	limit := int32(50)
	if r.Params.Limit != nil {
		if *r.Params.Limit < 1 || *r.Params.Limit > 100 {
			return nil, access.ErrInvalid
		}
		limit = int32(*r.Params.Limit)
	}
	p, err := h.SubmissionFreeze.PushEvidence(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.TeamId,
		cursor,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("submission freeze push evidence: %w", err)
	}
	return convertDTO[contract.GetSubmissionFreezePushEvidence200JSONResponse](p)
}
