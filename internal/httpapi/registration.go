package httpapi

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
)

func (h handlers) ApplyForEvent(
	ctx context.Context,
	r contract.ApplyForEventRequestObject,
) (contract.ApplyForEventResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[registration.Application](r.Body)
	if err != nil {
		return nil, err
	}
	result, err := h.Registration.Apply(ctx, actor(ctx).ID, r.EventId, input)
	if err != nil {
		return nil, fmt.Errorf("register for event: %w", err)
	}
	return convertDTO[contract.ApplyForEvent200JSONResponse](result)
}

func (h handlers) ListOwnRegistrations(
	ctx context.Context,
	r contract.ListOwnRegistrationsRequestObject,
) (contract.ListOwnRegistrationsResponseObject, error) {
	rows, err := h.Registration.Own(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("list own registrations: %w", err)
	}
	items, err := convertDTO[[]contract.Registration](rows)
	if err != nil {
		return nil, err
	}
	return contract.ListOwnRegistrations200JSONResponse{Items: items}, nil
}

func (h handlers) PromoteRegistration(
	ctx context.Context,
	r contract.PromoteRegistrationRequestObject,
) (contract.PromoteRegistrationResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	result, err := h.Registration.Promote(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.RegistrationId,
		revision(r.Params.IfMatch),
		r.Body.Reason,
	)
	if err != nil {
		return nil, fmt.Errorf("promote registration: %w", err)
	}
	return convertDTO[contract.PromoteRegistration200JSONResponse](result)
}

func (h handlers) GetRegistrationPage(
	ctx context.Context,
	r contract.GetRegistrationPageRequestObject,
) (contract.GetRegistrationPageResponseObject, error) {
	result, err := h.Registration.Page(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("read registration page: %w", err)
	}
	return convertDTO[contract.GetRegistrationPage200JSONResponse](result)
}

func (h handlers) DiscoverRegistrationEvents(
	ctx context.Context,
	r contract.DiscoverRegistrationEventsRequestObject,
) (contract.DiscoverRegistrationEventsResponseObject, error) {
	after := uuid.Nil
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
	rows, err := h.Registration.Discover(ctx, actor(ctx).ID, after, limit)
	if err != nil {
		return nil, fmt.Errorf("discover events: %w", err)
	}
	items, err := convertDTO[[]contract.RegistrationEvent](rows)
	if err != nil {
		return nil, err
	}
	result := contract.DiscoverRegistrationEvents200JSONResponse{Items: items}
	if len(rows) == int(limit) {
		result.NextCursor = new(rows[len(rows)-1].ID)
	}
	return result, nil
}
