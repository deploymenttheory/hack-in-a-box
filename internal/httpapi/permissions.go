package httpapi

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/permissions"
)

func revision(value string) int64 {
	n, _ := strconv.ParseInt(strings.Trim(value, "\""), 10, 64)
	return n
}

func (h handlers) ListStaff(
	ctx context.Context,
	r contract.ListStaffRequestObject,
) (contract.ListStaffResponseObject, error) {
	rows, err := h.Permissions.List(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("event permissions: %w", err)
	}
	result := contract.StaffPage{Items: make([]contract.StaffMember, 0, len(rows))}
	for _, row := range rows {
		result.Items = append(
			result.Items,
			contract.StaffMember{
				PersonId:    row.PersonID,
				DisplayName: row.DisplayName,
				Role:        contract.StaffRole(row.Role),
				Active:      row.Active,
				ChangedAt:   row.ChangedAt,
			},
		)
	}
	return contract.ListStaff200JSONResponse(result), nil
}

func (h handlers) ChangeStaff(
	ctx context.Context,
	r contract.ChangeStaffRequestObject,
) (contract.ChangeStaffResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	event, err := h.Permissions.Change(
		ctx,
		permissions.Change{
			Actor:                             actor(ctx).ID,
			Event:                             r.EventId,
			Person:                            r.Body.PersonId,
			Role:                              string(r.Body.Role),
			Active:                            r.Body.Active,
			AcknowledgeCompetitionRestriction: r.Body.AcknowledgeCompetitionRestriction,
			Revision:                          revision(r.Params.IfMatch),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("event permissions: %w", err)
	}
	return contract.ChangeStaff200JSONResponse{Revision: event.Revision}, nil
}

func (h handlers) TransferOwner(
	ctx context.Context,
	r contract.TransferOwnerRequestObject,
) (contract.TransferOwnerResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	event, err := h.Permissions.Transfer(
		ctx,
		permissions.Change{
			Actor:                             actor(ctx).ID,
			Event:                             r.EventId,
			Person:                            r.Body.PersonId,
			AcknowledgeCompetitionRestriction: r.Body.AcknowledgeCompetitionRestriction,
			Revision:                          revision(r.Params.IfMatch),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("event permissions: %w", err)
	}
	return contract.TransferOwner200JSONResponse{Revision: event.Revision}, nil
}
