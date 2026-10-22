package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/preparation"
)

func (h handlers) ListPreparationTasks(
	ctx context.Context,
	r contract.ListPreparationTasksRequestObject,
) (contract.ListPreparationTasksResponseObject, error) {
	f := preparation.Filter{OwnerID: r.Params.OwnerId}
	if r.Params.Status != nil {
		f.Status = string(*r.Params.Status)
	}
	if r.Params.OwnerRole != nil {
		f.OwnerRole = *r.Params.OwnerRole
	}
	if r.Params.Overdue != nil {
		f.Overdue = *r.Params.Overdue
	}
	if r.Params.Blocked != nil {
		f.Blocked = *r.Params.Blocked
	}
	if r.Params.Unowned != nil {
		f.Unowned = *r.Params.Unowned
	}
	page, err := h.Preparation.List(ctx, actor(ctx).ID, r.EventId, f)
	if err != nil {
		return nil, fmt.Errorf("list preparation: %w", err)
	}
	return convertDTO[contract.ListPreparationTasks200JSONResponse](page)
}

func (h handlers) SavePreparationTask(
	ctx context.Context,
	r contract.SavePreparationTaskRequestObject,
) (contract.SavePreparationTaskResponseObject, error) {
	if r.Body == nil || r.Body.Definition.Id != r.TaskId {
		return nil, access.ErrInvalid
	}
	u, err := convertDTO[preparation.Update](r.Body)
	if err != nil {
		return nil, err
	}
	page, err := h.Preparation.Save(ctx, actor(ctx).ID, r.EventId, revision(r.Params.IfMatch), u)
	if err != nil {
		return nil, fmt.Errorf("save preparation: %w", err)
	}
	return convertDTO[contract.SavePreparationTask200JSONResponse](page)
}

func (h handlers) PreparationTaskHistory(
	ctx context.Context,
	r contract.PreparationTaskHistoryRequestObject,
) (contract.PreparationTaskHistoryResponseObject, error) {
	rows, err := h.Preparation.History(ctx, actor(ctx).ID, r.EventId, r.TaskId)
	if err != nil {
		return nil, fmt.Errorf("task history: %w", err)
	}
	items, err := convertDTO[[]contract.PreparationChange](rows)
	if err != nil {
		return nil, err
	}
	return contract.PreparationTaskHistory200JSONResponse{Items: items}, nil
}
