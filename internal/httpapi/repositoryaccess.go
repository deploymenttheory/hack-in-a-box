package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/repositoryaccess"
)

func (h handlers) repositoryAccess() repositoryaccess.Service {
	service := h.RepositoryAccess
	if service.Pool == nil {
		service.Pool = h.Pool
	}
	return service
}

func (h handlers) GetRepositoryAccess(
	ctx context.Context,
	r contract.GetRepositoryAccessRequestObject,
) (contract.GetRepositoryAccessResponseObject, error) {
	result, err := h.repositoryAccess().Read(ctx, actor(ctx).ID, r.EventId, r.TeamId)
	if err != nil {
		return nil, fmt.Errorf("repository access API: %w", err)
	}
	return convertDTO[contract.GetRepositoryAccess200JSONResponse](result)
}

func (h handlers) ChangeRepositoryAccess(
	ctx context.Context,
	r contract.ChangeRepositoryAccessRequestObject,
) (contract.ChangeRepositoryAccessResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	command := repositoryaccess.Command{Action: string(r.Body.Action), Reason: r.Body.Reason}
	if r.Body.PlanId != nil {
		command.PlanID = *r.Body.PlanId
	}
	if r.Body.PlanVersion != nil {
		command.PlanVersion = *r.Body.PlanVersion
	}
	result, err := h.repositoryAccess().
		Change(ctx, actor(ctx).ID, r.EventId, r.TeamId, revision(r.Params.IfMatch), command)
	if err != nil {
		return nil, fmt.Errorf("repository access API: %w", err)
	}
	return convertDTO[contract.ChangeRepositoryAccess200JSONResponse](result)
}

func (h handlers) GetOwnRepositoryInvitations(
	ctx context.Context,
	r contract.GetOwnRepositoryInvitationsRequestObject,
) (contract.GetOwnRepositoryInvitationsResponseObject, error) {
	result, err := h.repositoryAccess().Own(ctx, actor(ctx).ID, r.EventId, r.TeamId)
	if err != nil {
		return nil, fmt.Errorf("own repository invitations API: %w", err)
	}
	return convertDTO[contract.GetOwnRepositoryInvitations200JSONResponse](result)
}

func (h handlers) GetRepositoryAccessReceipt(
	ctx context.Context,
	r contract.GetRepositoryAccessReceiptRequestObject,
) (contract.GetRepositoryAccessReceiptResponseObject, error) {
	result, err := h.repositoryAccess().ReadReceipt(ctx, actor(ctx).ID, r.EventId, r.TeamId, r.PlanId, string(r.Params.Action))
	if err != nil {
		return nil, fmt.Errorf("original repository access receipt API: %w", err)
	}
	return convertDTO[contract.GetRepositoryAccessReceipt200JSONResponse](result)
}
