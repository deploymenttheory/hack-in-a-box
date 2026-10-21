package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
)

func (h handlers) GetOfferPolicy(
	ctx context.Context,
	r contract.GetOfferPolicyRequestObject,
) (contract.GetOfferPolicyResponseObject, error) {
	result, err := h.Registration.OfferPolicy(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("registration action: %w", err)
	}
	return convertDTO[contract.GetOfferPolicy200JSONResponse](result)
}

func (h handlers) SaveOfferPolicy(
	ctx context.Context,
	r contract.SaveOfferPolicyRequestObject,
) (contract.SaveOfferPolicyResponseObject, error) {
	if r.Body == nil || r.Body.WorkingDays < 1 || r.Body.WorkingDays > 30 {
		return nil, access.ErrInvalid
	}
	result, err := h.Registration.SaveOfferPolicy(
		ctx,
		actor(ctx).ID,
		r.EventId,
		registration.OfferSettings{
			Revision:    revision(r.Params.IfMatch),
			Automated:   r.Body.Automated,
			WorkingDays: int32(r.Body.WorkingDays),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("registration action: %w", err)
	}
	return convertDTO[contract.SaveOfferPolicy200JSONResponse](result)
}

func (h handlers) RespondToRegistration(
	ctx context.Context,
	r contract.RespondToRegistrationRequestObject,
) (contract.RespondToRegistrationResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	result, err := h.Registration.Respond(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.RegistrationId,
		revision(r.Params.IfMatch),
		string(r.Body.Action),
	)
	if err != nil {
		return nil, fmt.Errorf("registration action: %w", err)
	}
	return convertDTO[contract.RespondToRegistration200JSONResponse](result)
}

func (h handlers) GetOwnRegistrationResponse(
	ctx context.Context,
	r contract.GetOwnRegistrationResponseRequestObject,
) (contract.GetOwnRegistrationResponseResponseObject, error) {
	result, err := h.Registration.OwnResponse(ctx, actor(ctx).ID, r.EventId, r.RegistrationId)
	if err != nil {
		return nil, fmt.Errorf("registration action: %w", err)
	}
	return convertDTO[contract.GetOwnRegistrationResponse200JSONResponse](result)
}

func (h handlers) CorrectRegistrationResponse(
	ctx context.Context,
	r contract.CorrectRegistrationResponseRequestObject,
) (contract.CorrectRegistrationResponseResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	answers, err := convertDTO[forms.Answers](r.Body)
	if err != nil {
		return nil, fmt.Errorf("registration action: %w", err)
	}
	result, err := h.Registration.CorrectResponse(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.RegistrationId,
		revision(r.Params.IfMatch),
		answers,
	)
	if err != nil {
		return nil, fmt.Errorf("registration action: %w", err)
	}
	return convertDTO[contract.CorrectRegistrationResponse200JSONResponse](result)
}
