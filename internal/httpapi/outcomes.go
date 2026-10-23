package httpapi

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
)

func (h handlers) ListOutcomes(
	ctx context.Context,
	r contract.ListOutcomesRequestObject,
) (contract.ListOutcomesResponseObject, error) {
	cursor, eventID := uuid.Nil, uuid.Nil
	mode := "own"
	if r.Params.Cursor != nil {
		cursor = *r.Params.Cursor
	}
	if r.Params.EventId != nil {
		eventID = *r.Params.EventId
	}
	if r.Params.Mode != nil {
		mode = string(*r.Params.Mode)
	}
	_, limit := page(nil, r.Params.Limit)
	v, err := h.Outcomes.List(ctx, actor(ctx).ID, eventID, mode, &cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("list native outcomes: %w", err)
	}
	return convertDTO[contract.ListOutcomes200JSONResponse](v)
}

func (h handlers) GetOutcome(
	ctx context.Context,
	r contract.GetOutcomeRequestObject,
) (contract.GetOutcomeResponseObject, error) {
	var version int64
	if r.Params.Version != nil {
		version = *r.Params.Version
	}
	var v outcomes.View
	var err error
	if r.Params.CommandId != nil {
		if r.Params.Version != nil {
			return nil, access.ErrInvalid
		}
		v, err = h.Outcomes.Receipt(ctx, actor(ctx).ID, r.OutcomeId, *r.Params.CommandId)
	} else {
		v, err = h.Outcomes.Read(ctx, actor(ctx).ID, r.OutcomeId, version)
	}
	if err != nil {
		return nil, fmt.Errorf("read native outcome: %w", err)
	}
	return convertDTO[contract.GetOutcome200JSONResponse](v)
}

func (h handlers) ChangeOutcome(
	ctx context.Context,
	r contract.ChangeOutcomeRequestObject,
) (contract.ChangeOutcomeResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	c, err := convertDTO[outcomes.Command](r.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.Outcomes.Change(ctx, actor(ctx).ID, r.OutcomeId, revision(r.Params.IfMatch), c)
	if err != nil {
		return nil, fmt.Errorf("change native outcome: %w", err)
	}
	return convertDTO[contract.ChangeOutcome200JSONResponse](v)
}

func (h handlers) GetOutcomeHistory(
	ctx context.Context,
	r contract.GetOutcomeHistoryRequestObject,
) (contract.GetOutcomeHistoryResponseObject, error) {
	var before int64
	if r.Params.BeforeVersion != nil {
		before = *r.Params.BeforeVersion
	}
	_, limit := page(nil, r.Params.Limit)
	v, err := h.Outcomes.History(ctx, actor(ctx).ID, r.OutcomeId, before, limit)
	if err != nil {
		return nil, fmt.Errorf("read outcome history: %w", err)
	}
	return convertDTO[contract.GetOutcomeHistory200JSONResponse](v)
}

func (h handlers) GetOutcomeEvidence(
	ctx context.Context,
	r contract.GetOutcomeEvidenceRequestObject,
) (contract.GetOutcomeEvidenceResponseObject, error) {
	cursor := uuid.Nil
	if r.Params.Cursor != nil {
		cursor = *r.Params.Cursor
	}
	_, limit := page(nil, r.Params.Limit)
	v, err := h.Outcomes.Evidence(ctx, actor(ctx).ID, r.OutcomeId, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("read restricted outcome evidence: %w", err)
	}
	return convertDTO[contract.GetOutcomeEvidence200JSONResponse](v)
}

func (h handlers) GetOutcomeOwners(
	ctx context.Context,
	r contract.GetOutcomeOwnersRequestObject,
) (contract.GetOutcomeOwnersResponseObject, error) {
	cursor := uuid.Nil
	if r.Params.Cursor != nil {
		cursor = *r.Params.Cursor
	}
	_, limit := page(nil, r.Params.Limit)
	v, err := h.Outcomes.Owners(ctx, actor(ctx).ID, r.OutcomeId, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("read eligible native outcome owners: %w", err)
	}
	return convertDTO[contract.GetOutcomeOwners200JSONResponse](v)
}

func (h handlers) GetOutcomePublication(
	ctx context.Context,
	r contract.GetOutcomePublicationRequestObject,
) (contract.GetOutcomePublicationResponseObject, error) {
	var v outcomes.PublicationView
	var err error
	if r.Params.CommandId != nil {
		v, err = h.Outcomes.PublicationReceipt(ctx, actor(ctx).ID, r.OutcomeId, *r.Params.CommandId)
	} else {
		v, err = h.Outcomes.Publication(ctx, actor(ctx).ID, r.OutcomeId)
	}
	if err != nil {
		return nil, fmt.Errorf("read outcome publication review: %w", err)
	}
	return convertDTO[contract.GetOutcomePublication200JSONResponse](v)
}

func (h handlers) ChangeOutcomePublication(
	ctx context.Context,
	r contract.ChangeOutcomePublicationRequestObject,
) (contract.ChangeOutcomePublicationResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	c, err := convertDTO[outcomes.PublicationCommand](r.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.Outcomes.ChangePublication(
		ctx,
		actor(ctx).ID,
		r.OutcomeId,
		revision(r.Params.IfMatch),
		c,
	)
	if err != nil {
		return nil, fmt.Errorf("change reviewed outcome publication: %w", err)
	}
	return convertDTO[contract.ChangeOutcomePublication200JSONResponse](v)
}

func (h handlers) ListOutcomeReviews(
	ctx context.Context,
	r contract.ListOutcomeReviewsRequestObject,
) (contract.ListOutcomeReviewsResponseObject, error) {
	v, err := h.Outcomes.Plans(ctx, actor(ctx).ID, r.OutcomeId)
	if err != nil {
		return nil, fmt.Errorf("list optional native outcome reviews: %w", err)
	}
	return convertDTO[contract.ListOutcomeReviews200JSONResponse](v)
}

func (h handlers) GetOutcomeReview(
	ctx context.Context,
	r contract.GetOutcomeReviewRequestObject,
) (contract.GetOutcomeReviewResponseObject, error) {
	var version int64
	if r.Params.Version != nil {
		version = *r.Params.Version
	}
	var v outcomes.PlanView
	var err error
	if r.Params.CommandId != nil {
		if r.Params.Version != nil {
			return nil, access.ErrInvalid
		}
		v, err = h.Outcomes.ReviewReceipt(ctx, actor(ctx).ID, r.ReviewId, *r.Params.CommandId)
	} else {
		v, err = h.Outcomes.Review(ctx, actor(ctx).ID, r.ReviewId, version)
	}
	if err != nil {
		return nil, fmt.Errorf("read native outcome review: %w", err)
	}
	return convertDTO[contract.GetOutcomeReview200JSONResponse](v)
}

func (h handlers) ChangeOutcomeReview(
	ctx context.Context,
	r contract.ChangeOutcomeReviewRequestObject,
) (contract.ChangeOutcomeReviewResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	c, err := convertDTO[outcomes.PlanCommand](r.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.Outcomes.ChangePlan(
		ctx,
		actor(ctx).ID,
		r.OutcomeId,
		r.ReviewId,
		revision(r.Params.IfMatch),
		c,
	)
	if err != nil {
		return nil, fmt.Errorf("change native outcome review: %w", err)
	}
	return convertDTO[contract.ChangeOutcomeReview200JSONResponse](v)
}

func (h handlers) ListPublishedProjects(
	ctx context.Context,
	r contract.ListPublishedProjectsRequestObject,
) (contract.ListPublishedProjectsResponseObject, error) {
	cursor := uuid.Nil
	if r.Params.Cursor != nil {
		cursor = *r.Params.Cursor
	}
	search := ""
	if r.Params.Search != nil {
		search = *r.Params.Search
	}
	_, limit := page(nil, r.Params.Limit)
	v, err := h.Outcomes.Projects(ctx, actor(ctx).ID, cursor, search, limit)
	if err != nil {
		return nil, fmt.Errorf("search reviewed published projects: %w", err)
	}
	return convertDTO[contract.ListPublishedProjects200JSONResponse](v)
}

func (h handlers) GetPublishedProject(
	ctx context.Context,
	r contract.GetPublishedProjectRequestObject,
) (contract.GetPublishedProjectResponseObject, error) {
	v, err := h.Outcomes.Public(ctx, actor(ctx).ID, r.OutcomeId)
	if err != nil {
		return nil, fmt.Errorf("read permitted published project: %w", err)
	}
	return convertDTO[contract.GetPublishedProject200JSONResponse](v)
}

func (h handlers) GetOutcomeObservations(
	ctx context.Context,
	r contract.GetOutcomeObservationsRequestObject,
) (contract.GetOutcomeObservationsResponseObject, error) {
	cursor := ""
	if r.Params.Cursor != nil {
		cursor = *r.Params.Cursor
	}
	_, limit := page(nil, r.Params.Limit)
	v, err := h.Outcomes.Observations(ctx, actor(ctx).ID, r.EventId, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("read restricted outcome observations: %w", err)
	}
	return convertDTO[contract.GetOutcomeObservations200JSONResponse](v)
}
