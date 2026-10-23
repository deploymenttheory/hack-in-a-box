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
		Subject       string                        `json:"subject"`
		PlainText     string                        `json:"plain_text"`
		Message       maildelivery.Message          `json:"message"`
		ScopeApproval *communications.ScopeApproval `json:"scope_approval,omitempty"`
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
	report, err := s.QueueApprovedOccurrenceReportInTransaction(
		ctx,
		tx,
		eventID,
		campaignID,
		expectedVersion,
		scope,
	)
	return report.TotalIntents(), err
}

// QueueReport distinguishes queued intents from current unavailable recipients.
// Queued counts newly retained intents, never network attempts or delivery.
// Preexisting reports retained statuses; accepted means relay acceptance only.
type QueueReport struct {
	Queued      int
	Unavailable int
	Preexisting map[string]int
}

func (r QueueReport) TotalIntents() int {
	count := r.Queued
	for _, n := range r.Preexisting {
		count += n
	}
	return count
}

func (s Service) QueueApprovedOccurrenceReportInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	expectedVersion int64,
	scope communications.DeliveryScope,
) (QueueReport, error) {
	audience, err := s.scopeAudience(ctx, tx, eventID, campaignID, expectedVersion, scope)
	if err != nil {
		return QueueReport{}, err
	}
	if !s.configured() || s.Campaigns == nil || s.Campaigns.Jobs == nil {
		return QueueReport{}, mailcontact.ErrUnavailable
	}
	approved, err := s.resolveCampaign(
		ctx,
		tx,
		eventID,
		campaignID,
		expectedVersion,
		scope,
		uuid.Nil,
	)
	if err != nil {
		return QueueReport{}, fmt.Errorf("queue SMTP campaign: %w", err)
	}
	if approved.Version != expectedVersion ||
		!slices.Contains(approved.Definition.Channels, "smtp") {
		return QueueReport{}, access.ErrConflict
	}
	q := dbgen.New(tx)
	revision, err := s.revision(ctx, q, eventID)
	if err != nil {
		return QueueReport{}, fmt.Errorf("queue SMTP campaign: %w", err)
	}
	if approved.Approval.ChannelRevisions["smtp"] != revision {
		return QueueReport{}, access.ErrConflict
	}
	report := QueueReport{Preexisting: map[string]int{}}
	byPerson, err := existingOccurrenceIntents(
		ctx,
		q,
		eventID,
		campaignID,
		expectedVersion,
		approved.Approval.Binding,
		scope,
		audience.Binding,
	)
	if err != nil {
		return QueueReport{}, err
	}
	for _, recipient := range approved.Preview.Recipients {
		if scope.OccurrenceID != uuid.Nil &&
			!slices.Contains(audience.RecipientIDs, recipient.PersonID) {
			continue
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
		if prior, ok := byPerson[recipient.PersonID]; ok {
			report.Preexisting[existingIntentStatus(prior)]++
			continue
		}
		mailbox, eligible, err := queueMailbox(
			ctx,
			q,
			eventID,
			target,
			approved,
			scope,
			audience,
			recipient.PersonID,
		)
		if err != nil {
			return QueueReport{}, err
		}
		if !eligible {
			report.Unavailable++
			continue
		}

		input := dbgen.InsertOperationParams{
			EventID:      eventID,
			TargetID:     target,
			Kind:         "communication_campaign_smtp",
			OperationKey: key,
			DueAt:        approved.DueAt,
			Approved:     true,
		}
		var op dbgen.Operation
		if audience.AllowArchived {
			op, err = s.Campaigns.Jobs.EnqueueRetained(ctx, tx, input, scope.OccurrenceID)
		} else {
			op, err = s.Campaigns.Jobs.Enqueue(ctx, tx, input)
		}

		if err != nil {
			return QueueReport{}, fmt.Errorf("queue SMTP campaign: %w", err)
		}
		if prior, readErr := q.SMTPCampaignMessage(ctx, op.ID); readErr == nil {
			report.Preexisting[prior.State]++
			continue
		} else if !errors.Is(readErr, pgx.ErrNoRows) {
			return QueueReport{}, fmt.Errorf("queue SMTP campaign: %w", readErr)
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
			return QueueReport{}, fmt.Errorf("queue SMTP campaign: %w", err)
		}
		encoded, err := json.Marshal(
			campaignSnapshot{
				Subject:       recipient.Subject,
				PlainText:     recipient.Body,
				Message:       message,
				ScopeApproval: approved.ScopeApproval,
			},
		)
		if err != nil {
			return QueueReport{}, fmt.Errorf("queue SMTP campaign: %w", err)
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
			return QueueReport{}, fmt.Errorf("queue SMTP campaign: %w", err)
		}
		report.Queued++
	}
	return report, nil
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
	paused, pauseErr := dbgen.New(s.Pool).AutomationPaused(ctx, d.EventID)
	if pauseErr != nil {
		return false, fmt.Errorf("read SMTP campaign eligibility pause: %w", pauseErr)
	}
	if paused {
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
		return s.withCurrentCampaign(
			ctx,
			tx,
			d,
			func(row dbgen.SmtpCampaignMessage, snapshot campaignSnapshot) error {
				operation, err := q.Operation(ctx, d.ID)
				if err != nil {
					return fmt.Errorf("prepare SMTP campaign: %w", err)
				}
				if !mailOperationCurrent(operation, d, s.now(), "communication_campaign_smtp") {
					return access.ErrForbidden
				}
				prior = outcome(row.State)
				grantID := uuid.Nil
				if row.State != "queued" && row.State != "retryable" {
					if row.State != "accepted" && row.State != "ambiguous" &&
						row.State != "prepared" {
						return nil
					}
					grantID, err = pendingResend(ctx, tx, d.ID)
					if err != nil {
						return err
					}
					if grantID == uuid.Nil {
						return nil
					}
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
				attempt, err := q.PrepareSMTPSubmissionAttempt(
					ctx,
					dbgen.PrepareSMTPSubmissionAttemptParams{
						OperationID: d.ID,
						SourceKind:  "campaign",
					},
				)
				if err != nil {
					return fmt.Errorf("retain prepared SMTP campaign attempt: %w", err)
				}
				if grantID != uuid.Nil {
					if err = q.ConsumeDeliveryResend(
						ctx,
						dbgen.ConsumeDeliveryResendParams{ReviewID: grantID, AttemptID: attempt.ID},
					); err != nil {
						return fmt.Errorf("consume SMTP resend grant: %w", err)
					}
				}
				message = snapshot.Message
				ready = true
				return nil
			},
		)
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
		reviewed, reviewErr := s.reviewedResend(ctx, d)
		if reviewErr != nil {
			if errors.Is(reviewErr, jobs.ErrRetry) {
				return jobs.Retryable
			}
			return jobs.Permanent
		}
		if !reviewed {
			return outcome(row.State)
		}
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
	return mailOperationIdentityCurrent(operation, d, now, kind) &&
		(operation.State == "pending" || operation.State == "running")
}

func mailOperationIdentityCurrent(
	operation dbgen.Operation,
	d jobs.Delivery,
	now time.Time,
	kind string,
) bool {
	return operation.EventID == d.EventID && operation.TargetID == d.TargetID &&
		operation.Kind == kind &&
		operation.Approved &&
		!operation.Cancelled &&
		!operation.DueAt.After(now)
}

// queueMailbox preserves the reviewed disposition of an omitted person. A later
// mailbox confirmation cannot silently create a retrospective send for the same
// approved occurrence; an explicit fresh reviewed intent must have a new binding.
func queueMailbox(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, target uuid.UUID,
	approved communications.ApprovedCampaign,
	scope communications.DeliveryScope,
	audience communications.ScopeAudience,
	person uuid.UUID,
) (string, bool, error) {
	if _, err := q.SMTPCampaignOmission(
		ctx,
		dbgen.SMTPCampaignOmissionParams{EventID: eventID, IntentID: target},
	); err == nil {
		return "", false, nil
	} else if !errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return "", false, fmt.Errorf("read reviewed SMTP omission: %w", err)
	}
	existing, existingErr := q.SMTPCampaignIntent(
		ctx,
		dbgen.SMTPCampaignIntentParams{EventID: eventID, IntentID: target},
	)
	if existingErr != nil && !errors.Is(existingErr, pgx.ErrNoRows) {
		return "", false, fmt.Errorf("read reviewed SMTP intent: %w", existingErr)
	}
	mailbox, err := mailcontact.VerifiedMailboxInTransaction(ctx, q, person)
	if errors.Is(err, access.ErrUnauthenticated) {
		return "", false, nil
	}
	if errors.Is(err, mailcontact.ErrUnavailable) {
		if errors.Is(existingErr, pgx.ErrNoRows) {
			if err = q.RecordSMTPCampaignOmission(
				ctx,
				dbgen.RecordSMTPCampaignOmissionParams{
					EventID:           eventID,
					IntentID:          target,
					CampaignID:        approved.Definition.ID,
					CampaignVersion:   approved.Version,
					ApprovalBinding:   approved.Approval.Binding,
					OccurrenceID:      scope.OccurrenceID,
					OccurrenceBinding: audience.Binding,
					PersonID:          person,
				},
			); err != nil {
				return "", false, fmt.Errorf("record reviewed SMTP omission: %w", err)
			}
		}
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("resolve reviewed SMTP mailbox: %w", err)
	}
	if existingErr == nil && existing.Mailbox != mailbox {
		return "", false, nil
	}
	return mailbox, true, nil
}

func existingIntentStatus(prior dbgen.SMTPCampaignOccurrenceIntentsRow) string {
	switch prior.State {
	case "accepted", "ambiguous", "permanent":
		return prior.State
	}
	if prior.OperationState == "permanent" || prior.OperationState == "cancelled" ||
		prior.OperationState == "ambiguous" {
		return prior.OperationState
	}
	return prior.State
}

// CheckOccurrenceReviewInTransaction must run before the automation owner changes
// a review generation. Holding the event lock through its review commit prevents
// pending original workers being stranded by a newly effective scope binding.
func (s Service) CheckOccurrenceReviewInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	version int64,
	scope communications.DeliveryScope,
) error {
	if scope.OccurrenceID == uuid.Nil {
		return access.ErrInvalid
	}
	q := dbgen.New(tx)
	if _, err := q.LockEvent(ctx, eventID); err != nil {
		return fmt.Errorf("lock SMTP occurrence review: %w", err)
	}
	pending, err := q.SMTPOccurrenceReviewPending(
		ctx,
		dbgen.SMTPOccurrenceReviewPendingParams{
			EventID:         eventID,
			CampaignID:      campaignID,
			CampaignVersion: version,
			OccurrenceID:    scope.OccurrenceID,
		},
	)
	if err != nil {
		return fmt.Errorf("check SMTP occurrence review: %w", err)
	}
	if pending {
		return access.ErrConflict
	}
	return nil
}

func existingOccurrenceIntents(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, campaignID uuid.UUID,
	expectedVersion int64,
	approvalBinding string,
	scope communications.DeliveryScope,
	reviewBinding string,
) (map[uuid.UUID]dbgen.SMTPCampaignOccurrenceIntentsRow, error) {
	if scope.OccurrenceID != uuid.Nil {
		changed, err := q.SMTPOccurrenceSourceChanged(
			ctx,
			dbgen.SMTPOccurrenceSourceChangedParams{
				EventID:         eventID,
				CampaignID:      campaignID,
				OccurrenceID:    scope.OccurrenceID,
				CampaignVersion: expectedVersion,
				ApprovalBinding: approvalBinding,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("check immutable SMTP occurrence source: %w", err)
		}
		if !changed.Valid || changed.Bool {
			return nil, access.ErrConflict
		}
	}
	existing, err := q.SMTPCampaignOccurrenceIntents(
		ctx,
		dbgen.SMTPCampaignOccurrenceIntentsParams{
			EventID:         eventID,
			CampaignID:      campaignID,
			CampaignVersion: expectedVersion,
			ApprovalBinding: approvalBinding,
			OccurrenceID:    scope.OccurrenceID,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("read existing SMTP source intents: %w", err)
	}
	byPerson := map[uuid.UUID]dbgen.SMTPCampaignOccurrenceIntentsRow{}
	for _, intent := range existing {
		if intent.OccurrenceBinding != reviewBinding &&
			(intent.OperationState == "pending" || intent.OperationState == "running" || intent.State == "prepared") {
			return nil, access.ErrConflict
		}
		byPerson[intent.PersonID] = intent
	}

	return byPerson, nil
}
