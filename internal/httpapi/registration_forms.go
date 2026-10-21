package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

func (h handlers) ListRegistrationForms(
	ctx context.Context,
	r contract.ListRegistrationFormsRequestObject,
) (contract.ListRegistrationFormsResponseObject, error) {
	rows, err := h.Registration.Forms(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("list registration forms: %w", err)
	}
	items, err := convertDTO[[]contract.RegistrationForm](rows)
	if err != nil {
		return nil, err
	}
	return contract.ListRegistrationForms200JSONResponse{Items: items}, nil
}

func (h handlers) SaveRegistrationForm(
	ctx context.Context,
	r contract.SaveRegistrationFormRequestObject,
) (contract.SaveRegistrationFormResponseObject, error) {
	if r.Body == nil || r.Body.Id != r.FormId {
		return nil, access.ErrInvalid
	}
	d, err := convertDTO[forms.Definition](r.Body)
	if err != nil {
		return nil, err
	}
	result, err := h.Registration.SaveForm(
		ctx,
		actor(ctx).ID,
		r.EventId,
		revision(r.Params.IfMatch),
		d,
	)
	if err != nil {
		return nil, fmt.Errorf("save registration form: %w", err)
	}
	return convertDTO[contract.SaveRegistrationForm200JSONResponse](result)
}

func (h handlers) PublishRegistrationForm(
	ctx context.Context,
	r contract.PublishRegistrationFormRequestObject,
) (contract.PublishRegistrationFormResponseObject, error) {
	if r.Body == nil || r.Body.Version < 1 || r.Body.Version > 1000 {
		return nil, access.ErrInvalid
	}
	result, err := h.Registration.PublishForm(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.FormId,
		revision(r.Params.IfMatch),
		int32(r.Body.Version),
		r.Body.AcknowledgeResponseVersions,
	)
	if err != nil {
		return nil, fmt.Errorf("publish registration form: %w", err)
	}
	return convertDTO[contract.PublishRegistrationForm200JSONResponse](result)
}

func (h handlers) PreviewRegistrationForm(
	ctx context.Context,
	r contract.PreviewRegistrationFormRequestObject,
) (contract.PreviewRegistrationFormResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	d, err := convertDTO[forms.Definition](r.Body.Definition)
	if err != nil {
		return nil, err
	}
	a, err := convertDTO[forms.Answers](r.Body.Answers)
	if err != nil {
		return nil, err
	}
	result, err := h.Registration.PreviewForm(ctx, actor(ctx).ID, r.EventId, d, a)
	if err != nil {
		return nil, fmt.Errorf("preview registration form: %w", err)
	}
	return convertDTO[contract.PreviewRegistrationForm200JSONResponse](result)
}
