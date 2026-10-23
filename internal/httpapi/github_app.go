package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

func (h handlers) GetGitHubAppIntegration(
	ctx context.Context,
	r contract.GetGitHubAppIntegrationRequestObject,
) (contract.GetGitHubAppIntegrationResponseObject, error) {
	status, err := h.GitHubApp.Status(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("read App integration: %w", err)
	}
	return convertDTO[contract.GetGitHubAppIntegration200JSONResponse](status)
}

func (h handlers) ConfigureGitHubAppIntegration(
	ctx context.Context,
	r contract.ConfigureGitHubAppIntegrationRequestObject,
) (contract.ConfigureGitHubAppIntegrationResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[githubapp.Command](r.Body)
	if err != nil {
		return nil, err
	}
	if err = h.GitHubApp.Configure(
		ctx,
		actor(ctx).ID,
		r.EventId,
		revision(r.Params.IfMatch),
		input,
	); err != nil {
		return nil, fmt.Errorf("configure App integration: %w", err)
	}
	status, err := h.GitHubApp.Status(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("read updated App integration: %w", err)
	}
	return convertDTO[contract.ConfigureGitHubAppIntegration200JSONResponse](status)
}

func (h handlers) AuthorizeGitHubAppInstallation(
	ctx context.Context,
	r contract.AuthorizeGitHubAppInstallationRequestObject,
) (contract.AuthorizeGitHubAppInstallationResponseObject, error) {
	destination, err := h.GitHubApp.Begin(
		ctx,
		actor(ctx).ID,
		r.EventId,
		h.Auth.Sessions.Token(ctx),
		revision(r.Params.IfMatch),
	)
	if err != nil {
		return nil, fmt.Errorf("start App installation: %w", err)
	}
	return contract.AuthorizeGitHubAppInstallation200JSONResponse{Url: destination}, nil
}

func (h handlers) CompleteGitHubAppInstallation(
	ctx context.Context,
	r contract.CompleteGitHubAppInstallationRequestObject,
) (contract.CompleteGitHubAppInstallationResponseObject, error) {
	if h.Auth == nil {
		return nil, access.ErrUnauthenticated
	}
	person, err := h.Auth.Current(ctx)
	if err != nil {
		return nil, fmt.Errorf("authenticate App callback: %w", err)
	}
	eventID, err := h.GitHubApp.Complete(
		ctx,
		person.ID,
		h.Auth.Sessions.Token(ctx),
		r.Params.State,
		r.Params.InstallationId,
	)
	if err != nil {
		return nil, fmt.Errorf("validate App callback: %w", err)
	}
	return contract.CompleteGitHubAppInstallation303Response{
		Headers: contract.CompleteGitHubAppInstallation303ResponseHeaders{
			Location: "/events/" + eventID.String() + "?github_app=connected",
		},
	}, nil
}
