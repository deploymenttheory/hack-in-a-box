package smtpintegration

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/mailcontact"
)

// withCurrentCampaign is the common source/audience/provider/mailbox/snapshot
// policy for execution and an explicit operator review. It performs no send.
func (s Service) withCurrentCampaign(
	ctx context.Context,
	tx pgx.Tx,
	d jobs.Delivery,
	apply func(dbgen.SmtpCampaignMessage, campaignSnapshot) error,
) error {
	q := dbgen.New(tx)
	paused, err := q.AutomationPaused(ctx, d.EventID)
	if err != nil {
		return fmt.Errorf("read current SMTP campaign pause: %w", err)
	}
	if paused {
		return jobs.ErrRetry
	}
	row, err := q.SMTPCampaignMessage(ctx, d.ID)
	if err != nil {
		return fmt.Errorf("prepare SMTP campaign: %w", err)
	}
	if row.EventID != d.EventID || row.IntentID != d.TargetID {
		return access.ErrForbidden
	}
	audience, err := s.scopeAudience(
		ctx,
		tx,
		d.EventID,
		row.CampaignID,
		row.CampaignVersion,
		communications.DeliveryScope{OccurrenceID: row.OccurrenceID},
	)
	if err != nil {
		return err
	}
	if row.OccurrenceID != uuid.Nil &&
		(audience.Binding != row.OccurrenceBinding || !slices.Contains(audience.RecipientIDs, row.PersonID)) {
		return access.ErrForbidden
	}
	approved, err := s.resolveCampaign(
		ctx,
		tx,
		d.EventID,
		row.CampaignID,
		row.CampaignVersion,
		communications.DeliveryScope{OccurrenceID: row.OccurrenceID},
		row.PersonID,
	)
	if err != nil {
		return fmt.Errorf("prepare SMTP campaign: %w", err)
	}
	if approved.Version != row.CampaignVersion ||
		approved.Approval.Binding != row.ApprovalBinding ||
		!slices.Contains(approved.Definition.Channels, "smtp") ||
		len(approved.Preview.Recipients) != 1 {
		return access.ErrConflict
	}
	revision, err := s.revision(ctx, q, d.EventID)
	if err != nil {
		return fmt.Errorf("prepare SMTP campaign: %w", err)
	}
	if revision != row.ProviderRevision ||
		approved.Approval.ChannelRevisions["smtp"] != revision {
		return access.ErrConflict
	}
	mailbox, err := mailcontact.VerifiedMailboxInTransaction(ctx, q, row.PersonID)
	if err != nil {
		return fmt.Errorf("prepare SMTP campaign: %w", err)
	}
	if mailbox != row.Mailbox {
		return access.ErrConflict
	}
	var snapshot campaignSnapshot
	if err = json.Unmarshal(row.Snapshot, &snapshot); err != nil {
		return fmt.Errorf("prepare SMTP campaign: %w", err)
	}
	recipient := approved.Preview.Recipients[0]
	if !campaignProjectionMatches(snapshot, approved.ScopeApproval, recipient, row.PersonID) {
		return access.ErrConflict
	}
	return apply(row, snapshot)
}
