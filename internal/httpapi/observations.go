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
	result, err := h.Observations.Read(ctx, actor(ctx).ID, r.EventId, r.TeamId)
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
	result, err := h.Observations.History(ctx, actor(ctx).ID, r.EventId, r.TeamId, id)
	if err != nil {
		return nil, fmt.Errorf("read private mentor history: %w", err)
	}
	return convertDTO[contract.GetMentorObservationHistory200JSONResponse](result)
}
