package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/credentials"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

func (h handlers) ListTeamCredentials(
	ctx context.Context,
	r contract.ListTeamCredentialsRequestObject,
) (contract.ListTeamCredentialsResponseObject, error) {
	result, err := h.Credentials.List(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("credential request: %w", err)
	}
	return convertDTO[contract.ListTeamCredentials200JSONResponse](result)
}

func (h handlers) ChangeTeamCredentials(
	ctx context.Context,
	r contract.ChangeTeamCredentialsRequestObject,
) (contract.ChangeTeamCredentialsResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[credentials.Command](r.Body)
	if err != nil {
		return nil, fmt.Errorf("credential request: %w", err)
	}
	result, err := h.Credentials.Change(
		ctx,
		actor(ctx).ID,
		r.EventId,
		revision(r.Params.IfMatch),
		input,
	)
	if err != nil {
		return nil, fmt.Errorf("credential request: %w", err)
	}
	return convertDTO[contract.ChangeTeamCredentials200JSONResponse](result)
}

func (h handlers) RevealTeamCredential(
	ctx context.Context,
	r contract.RevealTeamCredentialRequestObject,
) (contract.RevealTeamCredentialResponseObject, error) {
	result, err := h.Credentials.Reveal(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.CredentialId,
		revision(r.Params.IfMatch),
	)
	if err != nil {
		return nil, fmt.Errorf("credential request: %w", err)
	}
	body, err := convertDTO[contract.CredentialRevealed](result)
	if err != nil {
		return nil, fmt.Errorf("credential request: %w", err)
	}
	return contract.RevealTeamCredential200JSONResponse{
		Body:    body,
		Headers: contract.RevealTeamCredential200ResponseHeaders{CacheControl: new("no-store")},
	}, nil
}
