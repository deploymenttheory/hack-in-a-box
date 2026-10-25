package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
)

const configRevision = "inbox-v1"

func (s Service) Readiness(
	context.Context,
	uuid.UUID,
	uuid.UUID,
) (communications.ChannelReadiness, error) {
	return communications.ChannelReadiness{
		Available:         true,
		ConfigRevision:    configRevision,
		SupportsSelfTest:  true,
		SelfTestAvailable: true,
		SetupURL:          "#in-app-provider-setup",
	}, nil
}

func insert(ctx context.Context, q *dbgen.Queries, d Delivery) (dbgen.InboxNotification, error) {
	if d.EventID == uuid.Nil || d.RecipientID == uuid.Nil || d.Source.ID == uuid.Nil ||
		d.Source.Version < 1 ||
		len(d.Subject) > 4000 ||
		len(d.Body) > 200000 ||
		!slices.Contains([]string{"text", "markdown", "html"}, d.Format) {
		return dbgen.InboxNotification{}, access.ErrInvalid
	}
	id, err := uuid.NewV7()
	if err != nil {
		return dbgen.InboxNotification{}, wrap("create ID", err)
	}
	provenance, err := json.Marshal(d.Provenance)
	if err != nil {
		return dbgen.InboxNotification{}, wrap("encode source provenance", err)
	}
	row, err := q.InsertInboxNotification(
		ctx,
		dbgen.InsertInboxNotificationParams{
			ID:            id,
			EventID:       d.EventID,
			RecipientID:   d.RecipientID,
			SourceKind:    d.Source.Kind,
			SourceID:      d.Source.ID,
			SourceVersion: d.Source.Version,
			Provenance:    provenance,
			Subject:       d.Subject,
			Body:          d.Body,
			Format:        d.Format,
			OccurrenceID:  d.OccurrenceID,
			ScopeBinding:  d.ScopeBinding,
		},
	)
	var retained communications.InboxProvenance
	if err == nil &&
		(json.Unmarshal(row.Provenance, &retained) != nil || !sameProvenance(retained, d.Provenance)) {
		return row, access.ErrConflict
	}
	if err == nil &&
		(row.Subject != d.Subject || row.Body != d.Body || row.Format != d.Format ||
			(row.ScopeBinding != d.ScopeBinding && !reviewedReuse(d))) {
		return row, access.ErrConflict
	}
	return row, wrap("persist recipient", err)
}

// QueueApprovedInTransaction is the in-app adapter for #82's approved execution
// transaction. It does not schedule a campaign or accept arbitrary recipients.
func (s Service) QueueApprovedInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	expectedVersion int64,
) (int, error) {
	return s.QueueApprovedOccurrenceInTransaction(
		ctx,
		tx,
		eventID,
		campaignID,
		expectedVersion,
		communications.DeliveryScope{},
	)
}

func (s Service) QueueApprovedOccurrenceInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	expectedVersion int64,
	scope communications.DeliveryScope,
) (int, error) {
	scoped := communications.ScopeAudience{}
	if scope.OccurrenceID != uuid.Nil {
		if s.ScopePolicy == nil {
			return 0, access.ErrForbidden
		}
		var err error
		scoped, err = s.ScopePolicy.ResolveInTransaction(
			ctx,
			tx,
			eventID,
			campaignID,
			expectedVersion,
			scope,
		)
		if err != nil {
			return 0, wrap("resolve campaign delivery", err)
		}
		if scoped.Binding == "" || len(scoped.Binding) > 256 {
			return 0, access.ErrForbidden
		}
	}
	var approved communications.ApprovedCampaign
	var err error
	if scope.OccurrenceID == uuid.Nil {
		approved, err = s.Communications.ResolveApprovedCampaignInTransaction(
			ctx,
			tx,
			eventID,
			campaignID,
		)
	} else {
		approved, err = s.Communications.ResolveApprovedScopedCampaignInTransaction(
			ctx,
			tx,
			eventID,
			campaignID,
			expectedVersion,
			scope,
			s.ScopePolicy,
		)
	}

	if err != nil {
		return 0, wrap("resolve campaign delivery", err)
	}
	if approved.Version != expectedVersion {
		return 0, access.ErrConflict
	}
	if !slices.Contains(approved.Definition.Channels, "in_app") ||
		approved.Approval.ChannelRevisions["in_app"] != configRevision {
		return 0, access.ErrForbidden
	}
	var provenance communications.InboxProvenance
	if approved.ScopeApproval != nil && approved.ScopeApproval.Kind == "native_session_booking" {
		provenance = communications.InboxProvenance{ScopeApproval: approved.ScopeApproval}
	} else {
		provenance, err = s.Communications.RetainInboxProvenanceInTransaction(
			ctx,
			tx,
			eventID,
			campaignID,
			false,
		)
	}
	if err != nil {
		return 0, wrap("resolve campaign delivery", err)
	}
	provenance.ScopeApproval = approved.ScopeApproval
	if scoped.Resource != nil {
		ref := *scoped.Resource
		provenance.Resource = &ref
	}
	count := 0
	for _, recipient := range approved.Preview.Recipients {
		if scope.OccurrenceID != uuid.Nil &&
			!slices.Contains(scoped.RecipientIDs, recipient.PersonID) {
			continue
		}
		if provenance.Resource != nil {
			if _, err = s.nativeResourceDestination(
				ctx,
				tx,
				recipient.PersonID,
				eventID,
				*provenance.Resource,
			); err != nil {
				return 0, wrap("authorize retained campaign destination", err)
			}
		}
		_, err = insert(
			ctx,
			dbgen.New(tx),
			Delivery{
				EventID:      eventID,
				RecipientID:  recipient.PersonID,
				Source:       Source{Kind: "campaign", ID: campaignID, Version: approved.Version},
				Subject:      recipient.Subject,
				Body:         recipient.Body,
				Format:       approved.Format,
				Provenance:   provenance,
				OccurrenceID: scope.OccurrenceID,
				ScopeBinding: scoped.Binding,
				ScopeReview:  scoped.Review,
			},
		)
		if err != nil {
			return 0, wrap("resolve campaign delivery", err)
		}
		count++
	}
	return count, nil
}

// RecordResourceInTransaction is called by an owning domain after its own
// source mutation. The current resource adapter must exist and authorize this
// fixed recipient. Previews are fixed labels; source titles/prose are not stored.
func (s Service) RecordResourceInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, recipient uuid.UUID,
	source Source,
) error {
	if !slices.Contains(
		[]string{
			"content",
			"help",
			"clarification",
			"outcome_review",
			"feedback",
			"feedback_followup",
		},
		source.Kind,
	) {
		return access.ErrInvalid
	}
	if _, err := s.authorize(ctx, tx, recipient, eventID, source, nil); err != nil {
		return wrap("resolve test delivery", err)
	}
	_, err := insert(
		ctx,
		dbgen.New(tx),
		Delivery{
			EventID:     eventID,
			RecipientID: recipient,
			Source:      source,
			Subject:     label(source.Kind),
			Format:      "text",
		},
	)
	return wrap("resolve test delivery", err)
}

type TestHandler struct{ Service Service }

func (h TestHandler) Eligible(ctx context.Context, d jobs.Delivery) (bool, error) {
	allowed := false
	err := database.Within(ctx, h.Service.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		message, err := h.Service.Communications.ResolveTestSelfInTransaction(
			ctx,
			tx,
			d.EventID,
			d.TargetID,
		)
		if hidden(err) {
			return nil
		}
		if err != nil {
			return wrap("resolve test delivery", err)
		}
		allowed = message.Channel == "in_app" && message.ProviderRevision == configRevision
		return nil
	})
	return allowed, wrap("test eligibility", err)
}

func (h TestHandler) Deliver(ctx context.Context, d jobs.Delivery) jobs.Outcome {
	err := database.Within(ctx, h.Service.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		message, err := h.Service.Communications.ResolveTestSelfInTransaction(
			ctx,
			tx,
			d.EventID,
			d.TargetID,
		)
		if err != nil {
			return wrap("resolve test delivery", err)
		}
		if message.Channel != "in_app" || message.ProviderRevision != configRevision {
			return access.ErrForbidden
		}
		request, err := q.CommunicationTestRequest(
			ctx,
			dbgen.CommunicationTestRequestParams{EventID: d.EventID, ID: d.TargetID},
		)
		if err != nil {
			return wrap("load test intent", err)
		}
		provenance, err := h.Service.Communications.RetainInboxProvenanceInTransaction(
			ctx,
			tx,
			d.EventID,
			request.CampaignID,
			true,
		)
		if err != nil {
			return wrap("resolve test delivery", err)
		}
		_, err = insert(
			ctx,
			q,
			Delivery{
				EventID:     d.EventID,
				RecipientID: message.ActorID,
				Source: Source{
					Kind:    "test_self",
					ID:      d.TargetID,
					Version: request.CampaignVersion,
				},
				Subject:    message.Subject,
				Body:       message.PlainText,
				Format:     "text",
				Provenance: provenance,
			},
		)
		return wrap("resolve test delivery", err)
	})
	if err == nil {
		return jobs.Delivered
	}
	if hidden(err) || errors.Is(err, access.ErrInvalid) {
		return jobs.Permanent
	}
	return jobs.Retryable
}

// A local transaction and unique domain key make reconciliation safe: repeating
// delivery can only return the same immutable recipient item.
func (h TestHandler) Reconcile(ctx context.Context, d jobs.Delivery) jobs.Outcome {
	return h.Deliver(ctx, d)
}

func sameProvenance(left, right communications.InboxProvenance) bool {
	approvalEqual := communications.SameScopeApproval(left.ScopeApproval, right.ScopeApproval)
	resourceEqual := communications.SameResourceReference(left.Resource, right.Resource)
	left.ScopeApproval = nil
	right.ScopeApproval = nil
	left.Resource = nil
	right.Resource = nil
	return approvalEqual && resourceEqual && left == right
}

// Only a canonical current review may reuse identical already-persisted content
// across scope bindings. The retained row and its provenance are never changed.
func reviewedReuse(d Delivery) bool {
	return d.Source.Kind == "campaign" && d.OccurrenceID != uuid.Nil &&
		d.ScopeReview != nil && d.ScopeReview.OccurrenceID == d.OccurrenceID &&
		d.ScopeReview.Revision > 0 && d.ScopeReview.Binding == d.ScopeBinding
}
