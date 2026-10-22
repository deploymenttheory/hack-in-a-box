package smtpintegration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/mailcontact"
	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
)

// CampaignHandler keeps production intents distinct from fixed-organiser tests.
// Scheduling, missed-action decisions and event-wide pause are owned by callers.
type (
	CampaignHandler  struct{ Service *Service }
	campaignSnapshot struct {
		Subject   string               `json:"subject"`
		PlainText string               `json:"plain_text"`
		Message   maildelivery.Message `json:"message"`
	}
)

// QueueApprovedCampaignInTransaction performs one current audience resolution,
// then persists deduplicated individual intents and jobs without network effects.
func (s Service) QueueApprovedCampaignInTransaction(
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

func (s Service) scopeAudience(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	version int64,
	scope communications.DeliveryScope,
) (communications.ScopeAudience, error) {
	if scope.OccurrenceID == uuid.Nil {
		return communications.ScopeAudience{}, nil
	}
	if s.ScopePolicy == nil {
		return communications.ScopeAudience{}, access.ErrForbidden
	}
	audience, err := s.ScopePolicy.ResolveInTransaction(
		ctx,
		tx,
		eventID,
		campaignID,
		version,
		scope,
	)
	if err != nil {
		return audience, fmt.Errorf("resolve SMTP occurrence: %w", err)
	}
	if audience.Binding == "" {
		return audience, access.ErrForbidden
	}
	return audience, nil
}

// QueueApprovedOccurrenceInTransaction intersects the automation-owned current
// scope with the approved campaign. It never accepts caller-selected recipients.
func (s Service) QueueApprovedOccurrenceInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	expectedVersion int64,
	scope communications.DeliveryScope,
) (int, error) {
	audience, err := s.scopeAudience(ctx, tx, eventID, campaignID, expectedVersion, scope)
	if err != nil {
		return 0, err
	}
	if !s.configured() || s.Campaigns == nil || s.Campaigns.Jobs == nil {
		return 0, mailcontact.ErrUnavailable
	}
	approved, err := s.Campaigns.ResolveApprovedPersonalCampaignInTransaction(
		ctx,
		tx,
		eventID,
		campaignID,
		uuid.Nil,
	)
	if err != nil {
		return 0, fmt.Errorf("queue SMTP campaign: %w", err)
	}
	if approved.Version != expectedVersion ||
		!slices.Contains(approved.Definition.Channels, "smtp") {
		return 0, access.ErrConflict
	}
	q := dbgen.New(tx)
	revision, err := s.revision(ctx, q, eventID)
	if err != nil {
		return 0, fmt.Errorf("queue SMTP campaign: %w", err)
	}
	if approved.Approval.ChannelRevisions["smtp"] != revision {
		return 0, access.ErrConflict
	}
	count := 0
	for _, recipient := range approved.Preview.Recipients {
		if scope.OccurrenceID != uuid.Nil &&
			!slices.Contains(audience.RecipientIDs, recipient.PersonID) {
			continue
		}
		mailbox, err := mailcontact.VerifiedMailboxInTransaction(ctx, q, recipient.PersonID)
		if errors.Is(err, mailcontact.ErrUnavailable) || errors.Is(err, access.ErrUnauthenticated) {
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("queue SMTP campaign: %w", err)
		}
		identity := fmt.Sprintf(
			"smtp:%s:%s:%d:%s:%s:%s:%s",
			eventID,
			campaignID,
			expectedVersion,
			approved.Approval.Binding,
			scope.OccurrenceID,
			audience.Binding,
			recipient.PersonID,
		)
		digest := sha256.Sum256([]byte(identity))
		key := "communication_campaign_smtp:" + hex.EncodeToString(digest[:])
		target := uuid.NewSHA1(uuid.NameSpaceOID, []byte(key))
		op, err := s.Campaigns.Jobs.Enqueue(
			ctx,
			tx,
			dbgen.InsertOperationParams{
				EventID:      eventID,
				TargetID:     target,
				Kind:         "communication_campaign_smtp",
				OperationKey: key,
				DueAt:        approved.DueAt,
				Approved:     true,
			},
		)
		if err != nil {
			return 0, fmt.Errorf("queue SMTP campaign: %w", err)
		}
		if _, err = q.SMTPCampaignMessage(ctx, op.ID); err == nil {
			count++
			continue
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("queue SMTP campaign: %w", err)
		}
		request := communications.TestMessage{
			EventID:         eventID,
			ActorID:         recipient.PersonID,
			Subject:         recipient.Subject,
			PlainText:       recipient.Body,
			IncludeCalendar: approved.Definition.IncludeCalendar,
		}
		message, err := s.newMessageFor(
			ctx,
			q,
			request,
			jobs.Delivery{ID: op.ID, EventID: eventID, TargetID: target},
			mailbox,
			false,
		)
		if err != nil {
			return 0, fmt.Errorf("queue SMTP campaign: %w", err)
		}
		encoded, err := json.Marshal(
			campaignSnapshot{
				Subject:   recipient.Subject,
				PlainText: recipient.Body,
				Message:   message,
			},
		)
		if err != nil {
			return 0, fmt.Errorf("queue SMTP campaign: %w", err)
		}
		if err = q.SaveSMTPCampaignMessage(
			ctx,
			dbgen.SaveSMTPCampaignMessageParams{
				OperationID:       op.ID,
				EventID:           eventID,
				IntentID:          target,
				CampaignID:        campaignID,
				CampaignVersion:   expectedVersion,
				ApprovalBinding:   approved.Approval.Binding,
				OccurrenceID:      scope.OccurrenceID,
				OccurrenceBinding: audience.Binding,
				ProviderRevision:  revision,
				PersonID:          recipient.PersonID,
				Mailbox:           mailbox,
				Snapshot:          encoded,
			},
		); err != nil {
			return 0, fmt.Errorf("queue SMTP campaign: %w", err)
		}
		count++
	}
	return count, nil
}

func (h CampaignHandler) Eligible(ctx context.Context, d jobs.Delivery) (bool, error) {
	s := h.Service
	if s == nil || !s.configured() || s.Campaigns == nil {
		return false, nil
	}
	q := dbgen.New(s.Pool)
	row, err := q.SMTPCampaignMessage(ctx, d.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check SMTP campaign eligibility: %w", err)
	}
	if row.EventID != d.EventID || row.IntentID != d.TargetID {
		return false, nil
	}
	_, err = mailcontact.VerifiedMailboxInTransaction(ctx, q, row.PersonID)
	if errors.Is(err, mailcontact.ErrUnavailable) || errors.Is(err, access.ErrUnauthenticated) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check SMTP recipient: %w", err)
	}
	return true, nil
}

func (h CampaignHandler) prepare(
	ctx context.Context,
	d jobs.Delivery,
) (maildelivery.Message, bool, jobs.Outcome, error) {
	s := h.Service
	var message maildelivery.Message
	ready := false
	prior := jobs.Ambiguous
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(ctx, d.EventID); err != nil {
			return fmt.Errorf("prepare SMTP campaign: %w", err)
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
		approved, err := s.Campaigns.ResolveApprovedPersonalCampaignInTransaction(
			ctx,
			tx,
			d.EventID,
			row.CampaignID,
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
		operation, err := q.Operation(ctx, d.ID)
		if err != nil {
			return fmt.Errorf("prepare SMTP campaign: %w", err)
		}
		if !mailOperationCurrent(operation, d, s.now(), "communication_campaign_smtp") {
			return access.ErrForbidden
		}
		var snapshot campaignSnapshot
		if err = json.Unmarshal(row.Snapshot, &snapshot); err != nil {
			return fmt.Errorf("prepare SMTP campaign: %w", err)
		}
		recipient := approved.Preview.Recipients[0]
		if recipient.PersonID != row.PersonID || recipient.Subject != snapshot.Subject ||
			recipient.Body != snapshot.PlainText {
			return access.ErrConflict
		}
		prior = outcome(row.State)
		if row.State != "queued" && row.State != "retryable" {
			return nil
		}
		if err = q.RecordSMTPCampaignReceipt(
			ctx,
			dbgen.RecordSMTPCampaignReceiptParams{
				OperationID: d.ID,
				State:       "prepared",
				ReceiptCode: "",
			},
		); err != nil {
			return fmt.Errorf("prepare SMTP campaign: %w", err)
		}
		if _, err = q.PrepareSMTPSubmissionAttempt(
			ctx,
			dbgen.PrepareSMTPSubmissionAttemptParams{OperationID: d.ID, SourceKind: "campaign"},
		); err != nil {
			return fmt.Errorf("retain prepared SMTP campaign attempt: %w", err)
		}
		message = snapshot.Message
		ready = true
		return nil
	})
	if err != nil {
		return message, ready, prior, fmt.Errorf("prepare SMTP campaign: %w", err)
	}
	return message, ready, prior, nil
}

func (h CampaignHandler) Deliver(ctx context.Context, d jobs.Delivery) jobs.Outcome {
	s := h.Service
	if s == nil || !s.configured() || s.Campaigns == nil {
		return jobs.Permanent
	}
	row, err := dbgen.New(s.Pool).SMTPCampaignMessage(ctx, d.ID)
	if err == nil && row.EventID == d.EventID && row.IntentID == d.TargetID &&
		row.State != "queued" &&
		row.State != "retryable" {
		return outcome(row.State)
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return jobs.Retryable
	}
	message, ready, prior, err := h.prepare(ctx, d)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, access.ErrForbidden) ||
			errors.Is(err, access.ErrConflict) ||
			errors.Is(err, access.ErrNotFound) ||
			errors.Is(err, access.ErrUnauthenticated) ||
			errors.Is(err, communications.ErrUnresolved) ||
			errors.Is(err, mailcontact.ErrUnavailable) {
			return jobs.Permanent
		}
		return jobs.Retryable
	}
	if !ready {
		return prior
	}
	return s.sendPrepared(ctx, d, message, "campaign")
}

func (h CampaignHandler) Reconcile(ctx context.Context, d jobs.Delivery) jobs.Outcome {
	if h.Service == nil {
		return jobs.Ambiguous
	}
	row, err := dbgen.New(h.Service.Pool).SMTPCampaignMessage(ctx, d.ID)
	if err != nil || row.EventID != d.EventID || row.IntentID != d.TargetID {
		return jobs.Ambiguous
	}
	if row.State == "queued" {
		return jobs.Retryable
	}
	if row.State != "retryable" {
		return outcome(row.State)
	}
	// A definite previous rejection proves no effect. Deliver repeats the full
	// current source/recipient/approval check before creating the next attempt.
	return jobs.Retryable
}

func mailOperationCurrent(
	operation dbgen.Operation,
	d jobs.Delivery,
	now time.Time,
	kind string,
) bool {
	return operation.EventID == d.EventID && operation.TargetID == d.TargetID &&
		operation.Kind == kind &&
		operation.Approved &&
		!operation.Cancelled &&
		!operation.DueAt.After(now) &&
		(operation.State == "pending" || operation.State == "running")
}
