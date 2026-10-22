package smtpintegration

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
)

func (s Service) resolveCampaign(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	version int64,
	scope communications.DeliveryScope,
	personID uuid.UUID,
) (communications.ApprovedCampaign, error) {
	var approved communications.ApprovedCampaign
	var err error
	if scope.OccurrenceID == uuid.Nil {
		approved, err = s.Campaigns.ResolveApprovedPersonalCampaignInTransaction(
			ctx,
			tx,
			eventID,
			campaignID,
			personID,
		)
	} else {
		approved, err = s.Campaigns.ResolveApprovedScopedPersonalCampaignInTransaction(
			ctx,
			tx,
			eventID,
			campaignID,
			version,
			scope,
			s.ScopePolicy,
			personID,
		)
	}
	if err != nil {
		return approved, fmt.Errorf("resolve scoped personal SMTP campaign: %w", err)
	}
	return approved, nil
}

// ReadOnlyEligible is narrowly reserved for current retained outcome occurrences.
// Generic archived-event messages and self-tests never gain this capability.
func (h CampaignHandler) ReadOnlyEligible(ctx context.Context, d jobs.Delivery) (bool, error) {
	if h.Service == nil || h.Service.Campaigns == nil || h.Service.ScopePolicy == nil {
		return false, nil
	}
	eligible := false
	err := database.Within(ctx, h.Service.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, readErr := q.SMTPCampaignMessage(ctx, d.ID)
		if readErr != nil {
			return fmt.Errorf("read retained SMTP intent: %w", readErr)
		}
		if row.EventID != d.EventID || row.IntentID != d.TargetID || row.OccurrenceID == uuid.Nil {
			return access.ErrForbidden
		}
		scope := communications.DeliveryScope{OccurrenceID: row.OccurrenceID}
		audience, scopeErr := h.Service.scopeAudience(
			ctx,
			tx,
			d.EventID,
			row.CampaignID,
			row.CampaignVersion,
			scope,
		)
		if scopeErr != nil {
			return scopeErr
		}
		if !audience.AllowArchived || audience.Binding != row.OccurrenceBinding {
			return access.ErrForbidden
		}
		approved, scopeErr := h.Service.resolveCampaign(
			ctx,
			tx,
			d.EventID,
			row.CampaignID,
			row.CampaignVersion,
			scope,
			row.PersonID,
		)
		if scopeErr != nil {
			return scopeErr
		}
		eligible = approved.Approval.Binding == row.ApprovalBinding &&
			len(approved.Preview.Recipients) == 1
		return nil
	})
	if errors.Is(err, communications.ErrUnresolved) || errors.Is(err, access.ErrForbidden) ||
		errors.Is(err, access.ErrConflict) ||
		errors.Is(err, access.ErrNotFound) ||
		errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check retained SMTP intent: %w", err)
	}
	return eligible, nil
}

func campaignProjectionMatches(
	snapshot campaignSnapshot,
	proof *communications.ScopeApproval,
	recipient communications.Recipient,
	personID uuid.UUID,
) bool {
	return communications.SameScopeApproval(snapshot.ScopeApproval, proof) &&
		recipient.PersonID == personID &&
		recipient.Subject == snapshot.Subject &&
		recipient.Body == snapshot.PlainText
}
