package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
)

func (h handlers) GetMentorCoverage(
	ctx context.Context,
	r contract.GetMentorCoverageRequestObject,
) (contract.GetMentorCoverageResponseObject, error) {
	result, err := h.Mentors.Read(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("read mentor coverage: %w", err)
	}
	return convertDTO[contract.GetMentorCoverage200JSONResponse](result)
}

func (h handlers) ChangeMentorCoverage(
	ctx context.Context,
	r contract.ChangeMentorCoverageRequestObject,
) (contract.ChangeMentorCoverageResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[mentors.Command](r.Body)
	if err != nil {
		return nil, err
	}
	result, err := h.Mentors.Change(
		ctx,
		actor(ctx).ID,
		r.EventId,
		revision(r.Params.IfMatch),
		input,
	)
	if err != nil {
		return nil, fmt.Errorf("change mentor coverage: %w", err)
	}
	return convertDTO[contract.ChangeMentorCoverage200JSONResponse](result)
}

func (h handlers) SuggestMentorCoverage(
	ctx context.Context,
	r contract.SuggestMentorCoverageRequestObject,
) (contract.SuggestMentorCoverageResponseObject, error) {
	result, err := h.Mentors.Propose(ctx, actor(ctx).ID, r.EventId, revision(r.Params.IfMatch))
	if err != nil {
		return nil, fmt.Errorf("suggest mentor coverage: %w", err)
	}
	return convertDTO[contract.SuggestMentorCoverage200JSONResponse](result)
}
