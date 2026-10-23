package httpapi

import (
	"context"
	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/google/uuid"
)

func reviewPageArgs(cursor *uuid.UUID, limit *int, maximum int) (uuid.UUID, int, error) {
	after := uuid.Nil
	if cursor != nil {
		after = *cursor
	}
	count := 25
	if limit != nil {
		count = *limit
	}
	if count < 1 || count > maximum {
		return after, 0, access.ErrInvalid
	}
	return after, count, nil
}
func (h handlers) GetReviewCoverage(ctx context.Context, r contract.GetReviewCoverageRequestObject) (contract.GetReviewCoverageResponseObject, error) {
	if h.ReviewDashboard == nil {
		return nil, access.ErrForbidden
	}
	after, count, err := reviewPageArgs(r.Params.Cursor, r.Params.Limit, 50)
	if err != nil {
		return nil, err
	}
	page, err := h.ReviewDashboard.Read(ctx, actor(ctx).ID, r.EventId, after, r.Params.TeamId, int32(count))
	if err != nil {
		return nil, err
	}
	return convertDTO[contract.GetReviewCoverage200JSONResponse](page)
}
func (h handlers) GetReviewCheckpoints(ctx context.Context, r contract.GetReviewCheckpointsRequestObject) (contract.GetReviewCheckpointsResponseObject, error) {
	if h.ReviewDashboard == nil {
		return nil, access.ErrForbidden
	}
	after, count, err := reviewPageArgs(r.Params.Cursor, r.Params.Limit, 100)
	if err != nil {
		return nil, err
	}
	page, err := h.ReviewDashboard.Checkpoints(ctx, actor(ctx).ID, r.EventId, r.TeamId, after, count)
	if err != nil {
		return nil, err
	}
	return convertDTO[contract.GetReviewCheckpoints200JSONResponse](page)
}
func (h handlers) GetReviewAgenda(ctx context.Context, r contract.GetReviewAgendaRequestObject) (contract.GetReviewAgendaResponseObject, error) {
	if h.ReviewDashboard == nil {
		return nil, access.ErrForbidden
	}
	after, count, err := reviewPageArgs(r.Params.Cursor, r.Params.Limit, 100)
	if err != nil {
		return nil, err
	}
	page, err := h.ReviewDashboard.Huddles(ctx, actor(ctx).ID, r.EventId, r.TeamId, after, int32(count))
	if err != nil {
		return nil, err
	}
	return convertDTO[contract.GetReviewAgenda200JSONResponse](page)
}
func (h handlers) GetJudgeSummaryPreview(ctx context.Context, r contract.GetJudgeSummaryPreviewRequestObject) (contract.GetJudgeSummaryPreviewResponseObject, error) {
	if h.ReviewDashboard == nil {
		return nil, access.ErrForbidden
	}
	page, err := h.ReviewDashboard.Preview(ctx, actor(ctx).ID, r.EventId, r.TeamId)
	if err != nil {
		return nil, err
	}
	return convertDTO[contract.GetJudgeSummaryPreview200JSONResponse](page)
}
