package communications

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// ReviewCampaignApprovalInTransaction checks an existing exact generic approval
// without dispatching it early. It is used to review an automation binding; the
// execution resolver still requires the approved due time to have arrived.
func (s Service) ReviewCampaignApprovalInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, id uuid.UUID,
) (ApprovedCampaign, error) {
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return ApprovedCampaign{}, contextError("lock campaign approval review", err)
	}
	c, err := loadCampaign(ctx, q, eventID, id)
	if err != nil {
		return ApprovedCampaign{}, err
	}
	if c.Cancelled || c.Approval == nil {
		return ApprovedCampaign{}, ErrUnresolved
	}
	if err = authorize(ctx, q, c.Approval.ActorID, eventID, true); err != nil {
		return ApprovedCampaign{}, err
	}
	basis, err := s.campaignBasis(ctx, q, row, c.Approval.ActorID)
	if err != nil {
		return ApprovedCampaign{}, err
	}
	view := campaignView(c, basis)
	if view.Status != "approved" || view.DueAt == nil {
		return ApprovedCampaign{}, ErrUnresolved
	}
	preview, err := s.ResolveApprovedInTransaction(ctx, tx, eventID, c.Approval.PreviewID)
	if err != nil {
		return ApprovedCampaign{}, err
	}
	template, _, err := basis.pair(c.Definition)
	if err != nil {
		return ApprovedCampaign{}, err
	}
	return ApprovedCampaign{
		Approval:   *c.Approval,
		Purpose:    template.Purpose,
		Format:     template.Format,
		Definition: c.Definition,
		Version:    c.Version,
		DueAt:      *view.DueAt,
		Preview:    preview,
	}, nil
}
