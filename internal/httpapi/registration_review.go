package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
)

func (h handlers) ListApplicants(
	ctx context.Context,
	r contract.ListApplicantsRequestObject,
) (contract.ListApplicantsResponseObject, error) {
	after := int64(0)
	limit := int32(25)
	if r.Params.After != nil {
		after = *r.Params.After
	}
	if r.Params.Limit != nil {
		if *r.Params.Limit < 1 || *r.Params.Limit > 50 {
			return nil, access.ErrInvalid
		}
		limit = int32(*r.Params.Limit)
	}
	rows, err := h.Registration.Applicants(ctx, actor(ctx).ID, r.EventId, after, limit)
	if err != nil {
		return nil, fmt.Errorf("read applicants: %w", err)
	}
	items, err := convertDTO[[]contract.Applicant](rows)
	if err != nil {
		return nil, err
	}
	result := contract.ListApplicants200JSONResponse{Items: items}
	if len(rows) == int(limit) {
		result.NextCursor = new(rows[len(rows)-1].Registration.Sequence)
	}
	return result, nil
}

func (h handlers) GetApplicant(
	ctx context.Context,
	r contract.GetApplicantRequestObject,
) (contract.GetApplicantResponseObject, error) {
	result, err := h.Registration.Applicant(ctx, actor(ctx).ID, r.EventId, r.RegistrationId)
	if err != nil {
		return nil, fmt.Errorf("review applicant: %w", err)
	}
	return convertDTO[contract.GetApplicant200JSONResponse](result)
}

func (h handlers) ReviewApplicant(
	ctx context.Context,
	r contract.ReviewApplicantRequestObject,
) (contract.ReviewApplicantResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[registration.ReviewInput](r.Body)
	if err != nil {
		return nil, err
	}
	result, err := h.Registration.Review(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.RegistrationId,
		revision(r.Params.IfMatch),
		input,
	)
	if err != nil {
		return nil, fmt.Errorf("apply review decision: %w", err)
	}
	return convertDTO[contract.ReviewApplicant200JSONResponse](result)
}
