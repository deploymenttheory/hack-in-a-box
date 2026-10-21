package httpapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubidentity"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

func (h handlers) GetGitHubLink(
	ctx context.Context,
	_ contract.GetGitHubLinkRequestObject,
) (contract.GetGitHubLinkResponseObject, error) {
	if h.GitHub == nil {
		return contract.GetGitHubLink200JSONResponse{}, nil
	}
	status, err := h.GitHub.Status(ctx, actor(ctx).ID)
	if err != nil {
		return nil, fmt.Errorf("read GitHub identity: %w", err)
	}
	result, err := convertDTO[contract.GetGitHubLink200JSONResponse](status)
	return result, err
}

func (h handlers) AuthorizeGitHub(
	ctx context.Context,
	r contract.AuthorizeGitHubRequestObject,
) (contract.AuthorizeGitHubResponseObject, error) {
	if h.GitHub == nil {
		return nil, githubidentity.ErrDisabled
	}
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	url, err := h.GitHub.Begin(
		ctx,
		actor(ctx).ID,
		h.Auth.Sessions.Token(ctx),
		revision(r.Params.IfMatch),
		r.Body.AcknowledgeInvitationImpact,
	)
	if err != nil {
		return nil, fmt.Errorf("authorize GitHub identity: %w", err)
	}
	return contract.AuthorizeGitHub200JSONResponse{Url: url}, nil
}

func (h handlers) UnlinkGitHub(
	ctx context.Context,
	r contract.UnlinkGitHubRequestObject,
) (contract.UnlinkGitHubResponseObject, error) {
	if h.GitHub == nil {
		return nil, githubidentity.ErrDisabled
	}
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	if err := h.GitHub.Unlink(
		ctx,
		actor(ctx).ID,
		revision(r.Params.IfMatch),
		r.Body.AcknowledgeInvitationImpact,
	); err != nil {
		return nil, fmt.Errorf("unlink identity: %w", err)
	}
	status, err := h.GitHub.Status(ctx, actor(ctx).ID)
	if err != nil {
		return nil, fmt.Errorf("read removed identity: %w", err)
	}
	result, err := convertDTO[contract.UnlinkGitHub200JSONResponse](status)
	return result, err
}

func githubCallback(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Both destinations are fixed application paths; no callback-supplied redirect is used.
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		person, err := deps.Auth.Current(r.Context())
		if err != nil {
			http.Redirect(w, r, "/account?github=failed", http.StatusSeeOther)
			return
		}
		if err := deps.GitHub.Complete(
			r.Context(),
			person.ID,
			deps.Auth.Sessions.Token(r.Context()),
			r.URL.Query().Get("state"),
			r.URL.Query().Get("code"),
		); err != nil {
			http.Redirect(w, r, "/account?github=failed", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/account?github=linked", http.StatusSeeOther)
	}
}
