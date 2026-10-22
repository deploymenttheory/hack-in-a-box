package httpapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
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

func (h handlers) SuggestTeamAllocation(
	ctx context.Context,
	r contract.SuggestTeamAllocationRequestObject,
) (contract.SuggestTeamAllocationResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[teams.AllocationRequest](r.Body)
	if err != nil {
		return nil, err
	}
	result, err := h.Teams.Suggest(ctx, actor(ctx).ID, r.EventId, revision(r.Params.IfMatch), input)
	if err != nil {
		return nil, fmt.Errorf("suggest team allocation: %w", err)
	}
	return convertDTO[contract.SuggestTeamAllocation200JSONResponse](result)
}

func assignmentConflict(err error) (string, string) {
	switch {
	case errors.Is(err, mentors.ErrCoverage):
		return "mentor_coverage_invalid", mentors.ErrCoverage.Error()
	case errors.Is(err, teams.ErrFrozen):
		return "teams_frozen", teams.ErrFrozen.Error()
	case errors.Is(err, teams.ErrLocked):
		return "team_locked", teams.ErrLocked.Error()
	case errors.Is(err, teams.ErrCapacity):
		return "team_capacity", teams.ErrCapacity.Error()
	case errors.Is(err, teams.ErrUnready):
		return "teams_unready", teams.ErrUnready.Error()
	default:
		return "", ""
	}
}
