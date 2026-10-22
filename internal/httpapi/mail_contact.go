package httpapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/mailcontact"
)

func (h handlers) GetAccountEmail(
	ctx context.Context,
	_ contract.GetAccountEmailRequestObject,
) (contract.GetAccountEmailResponseObject, error) {
	status, err := h.MailContacts.Status(ctx, actor(ctx).ID)
	if err != nil {
		return nil, fmt.Errorf("email status: %w", err)
	}
	return convertDTO[contract.GetAccountEmail200JSONResponse](status)
}

func (h handlers) RequestEmailVerification(
	ctx context.Context,
	r contract.RequestEmailVerificationRequestObject,
) (contract.RequestEmailVerificationResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	status, err := h.MailContacts.Request(ctx, actor(ctx).ID, r.Body.Email)
	if err != nil {
		return nil, fmt.Errorf("email confirmation request: %w", err)
	}
	return convertDTO[contract.RequestEmailVerification202JSONResponse](status)
}

func (h handlers) ConfirmAccountEmail(
	ctx context.Context,
	r contract.ConfirmAccountEmailRequestObject,
) (contract.ConfirmAccountEmailResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	status, err := h.MailContacts.Confirm(ctx, actor(ctx).ID, r.Body.Code)
	if err != nil {
		return nil, fmt.Errorf("email confirmation: %w", err)
	}
	return convertDTO[contract.ConfirmAccountEmail200JSONResponse](status)
}

func emailError(err error) (int, string, string) {
	switch {
	case errors.Is(err, mailcontact.ErrRateLimited):
		return 429, "email_rate_limited", "Wait before requesting another confirmation code."
	case errors.Is(err, mailcontact.ErrUnavailable):
		return 503, "email_unavailable", "Personal email is not available."
	case errors.Is(err, mailcontact.ErrCode):
		return 400, "email_code_invalid", "The confirmation code is invalid or expired. Request a new code when available."
	default:
		return 0, "", ""
	}
}
