package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/teamsworkflow"
)

func (h handlers) GetTeamsWorkflow(
	ctx context.Context,
	r contract.GetTeamsWorkflowRequestObject,
) (contract.GetTeamsWorkflowResponseObject, error) {
	if h.TeamsWorkflow == nil {
		return nil, database.ErrDatabase
	}
	result, err := h.TeamsWorkflow.Read(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("read Teams workflow: %w", err)
	}
	return convertDTO[contract.GetTeamsWorkflow200JSONResponse](result)
}

func (h handlers) ChangeTeamsWorkflow(
	ctx context.Context,
	r contract.ChangeTeamsWorkflowRequestObject,
) (contract.ChangeTeamsWorkflowResponseObject, error) {
	if h.TeamsWorkflow == nil {
		return nil, database.ErrDatabase
	}
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[teamsworkflow.Command](r.Body)
	if err != nil {
		return nil, err
	}
	result, err := h.TeamsWorkflow.Change(
		ctx,
		actor(ctx).ID,
		r.EventId,
		revision(r.Params.IfMatch),
		input,
	)
	if err != nil {
		return nil, fmt.Errorf("change Teams workflow: %w", err)
	}
	return convertDTO[contract.ChangeTeamsWorkflow200JSONResponse](result)
}

func (h handlers) TestTeamsWorkflow(
	ctx context.Context,
	r contract.TestTeamsWorkflowRequestObject,
) (contract.TestTeamsWorkflowResponseObject, error) {
	if h.TeamsWorkflow == nil {
		return nil, database.ErrDatabase
	}
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	result, err := h.TeamsWorkflow.QueueTest(
		ctx,
		actor(ctx).ID,
		r.EventId,
		revision(r.Params.IfMatch),
		teamsworkflow.TestInput{RequestID: r.Body.RequestId},
	)
	if err != nil {
		return nil, fmt.Errorf("queue Teams connection test: %w", err)
	}
	return convertDTO[contract.TestTeamsWorkflow200JSONResponse](result)
}

func (h handlers) QueueTeamsBroadcast(
	ctx context.Context,
	r contract.QueueTeamsBroadcastRequestObject,
) (contract.QueueTeamsBroadcastResponseObject, error) {
	if h.TeamsWorkflow == nil {
		return nil, database.ErrDatabase
	}
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	result, err := h.TeamsWorkflow.QueueBroadcast(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.CampaignId,
		r.Body.Version,
	)
	if err != nil {
		return nil, fmt.Errorf("queue Teams broadcast: %w", err)
	}
	return convertDTO[contract.QueueTeamsBroadcast200JSONResponse](result)
}
