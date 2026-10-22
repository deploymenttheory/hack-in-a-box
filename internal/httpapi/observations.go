package httpapi

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/observations"
)

func (h handlers) GetMentorObservations(
	ctx context.Context,
	r contract.GetMentorObservationsRequestObject,
) (contract.GetMentorObservationsResponseObject, error) {
	options := observations.ReadOptions{Cursor: r.Params.Cursor}
	if r.Params.Limit != nil {
		options.Limit = *r.Params.Limit
	}
	result, err := h.Observations.ReadPage(ctx, actor(ctx).ID, r.EventId, r.TeamId, options)
	if err != nil {
		return nil, fmt.Errorf("read private mentor evidence: %w", err)
	}
	return convertDTO[contract.GetMentorObservations200JSONResponse](result)
}

func (h handlers) ChangeMentorObservations(
	ctx context.Context,
	r contract.ChangeMentorObservationsRequestObject,
) (contract.ChangeMentorObservationsResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[observations.Command](r.Body)
	if err != nil {
		return nil, err
	}
	result, err := h.Observations.Change(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.TeamId,
		revision(r.Params.IfMatch),
		input,
	)
	if err != nil {
		return nil, fmt.Errorf("change private mentor evidence: %w", err)
	}
	return convertDTO[contract.ChangeMentorObservations200JSONResponse](result)
}

func (h handlers) GetMentorObservationHistory(
	ctx context.Context,
	r contract.GetMentorObservationHistoryRequestObject,
) (contract.GetMentorObservationHistoryResponseObject, error) {
	id := uuid.Nil
	if r.Params.ObservationId != nil {
		id = *r.Params.ObservationId
	}
	options := observations.HistoryOptions{
		ObservationID: id, ObservationCursor: r.Params.ObservationCursor,
		StatusCursor: r.Params.StatusCursor,
	}
	if r.Params.Limit != nil {
		options.Limit = *r.Params.Limit
	}
	result, err := h.Observations.HistoryPage(ctx, actor(ctx).ID, r.EventId, r.TeamId, options)
	if err != nil {
		return nil, fmt.Errorf("read private mentor history: %w", err)
	}
	return convertDTO[contract.GetMentorObservationHistory200JSONResponse](result)
}
