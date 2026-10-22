package communications

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// ResolveApprovedScopedBroadcastInTransaction projects one identical public
// literal message after current persisted occurrence and source approval checks.
func (s Service) ResolveApprovedScopedBroadcastInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	version int64,
	scope DeliveryScope,
	policy ScopePolicy,
) (BroadcastMessage, error) {
	if scope.OccurrenceID == uuid.Nil || policy == nil {
		return BroadcastMessage{}, access.ErrForbidden
	}
	scoped, err := policy.ResolveInTransaction(ctx, tx, eventID, campaignID, version, scope)
	if err != nil {
		return BroadcastMessage{}, contextError("resolve broadcast scope", err)
	}
	if !scoped.Broadcast || scoped.AllowArchived {
		return BroadcastMessage{}, access.ErrForbidden
	}
	approved, err := s.ResolveApprovedScopedPersonalCampaignInTransaction(
		ctx,
		tx,
		eventID,
		campaignID,
		version,
		scope,
		policy,
		uuid.Nil,
	)
	if err != nil {
		return BroadcastMessage{}, err
	}
	if !slices.Contains(approved.Definition.Channels, "teams") {
		return BroadcastMessage{}, access.ErrForbidden
	}
	q := dbgen.New(tx)
	template, audience, err := loadPair(
		ctx,
		q,
		eventID,
		approved.Definition.TemplateID,
		approved.Definition.AudienceID,
	)
	if err != nil {
		return BroadcastMessage{}, err
	}
	facts, err := q.CommunicationFacts(ctx, eventID)
	if err != nil {
		return BroadcastMessage{}, contextError("read scoped broadcast facts", err)
	}
	if len(broadcastBlockers(template, audience, facts)) > 0 ||
		len(approved.Preview.Recipients) == 0 {
		return BroadcastMessage{}, ErrUnresolved
	}
	first := approved.Preview.Recipients[0]
	for _, recipient := range approved.Preview.Recipients {
		if recipient.Subject != first.Subject || recipient.Body != first.Body ||
			len(recipient.Unresolved) > 0 {
			return BroadcastMessage{}, ErrUnresolved
		}
	}
	return BroadcastMessage{
		EventID:         eventID,
		CampaignID:      campaignID,
		CampaignVersion: approved.Version,
		Approval:        approved.Approval,
		ScopeApproval:   approved.ScopeApproval,
		DueAt:           approved.DueAt,
		Purpose:         approved.Purpose,
		Subject:         first.Subject,
		PlainText:       first.Body,
	}, nil
}
