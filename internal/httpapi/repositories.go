package httpapi

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

func (h handlers) ListTeamRepositories(
	ctx context.Context,
	r contract.ListTeamRepositoriesRequestObject,
) (contract.ListTeamRepositoriesResponseObject, error) {
	result, err := h.Repositories.List(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("team repository API: %w", err)
	}
	return convertDTO[contract.ListTeamRepositories200JSONResponse](result)
}

func (h handlers) ProvisionTeamRepositories(
	ctx context.Context,
	r contract.ProvisionTeamRepositoriesRequestObject,
) (contract.ProvisionTeamRepositoriesResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	var err error
	switch r.Body.Action {
	case "queue":
		if r.Body.TeamId != nil {
			return nil, access.ErrInvalid
		}
		err = h.Repositories.Queue(ctx, actor(ctx).ID, r.EventId, revision(r.Params.IfMatch))
	case "reconcile":
		if r.Body.TeamId == nil || *r.Body.TeamId == uuid.Nil {
			return nil, access.ErrInvalid
		}
		err = h.Repositories.Recover(
			ctx,
			actor(ctx).ID,
			r.EventId,
			*r.Body.TeamId,
			revision(r.Params.IfMatch),
		)
	default:
		return nil, access.ErrInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("team repository API: %w", err)
	}
	result, err := h.Repositories.List(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("team repository API: %w", err)
	}
	return convertDTO[contract.ProvisionTeamRepositories200JSONResponse](result)
}
