package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/teams"
)

func (h handlers) GetTeams(
	ctx context.Context,
	r contract.GetTeamsRequestObject,
) (contract.GetTeamsResponseObject, error) {
	result, err := h.Teams.Read(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("read event teams: %w", err)
	}
	return convertDTO[contract.GetTeams200JSONResponse](result)
}

func (h handlers) ChangeTeams(
	ctx context.Context,
	r contract.ChangeTeamsRequestObject,
) (contract.ChangeTeamsResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[teams.Command](r.Body)
	if err != nil {
		return nil, err
	}
	result, err := h.Teams.Act(ctx, actor(ctx).ID, r.EventId, revision(r.Params.IfMatch), input)
	if err != nil {
		return nil, fmt.Errorf("apply team command: %w", err)
	}
	return convertDTO[contract.ChangeTeams200JSONResponse](result)
}
