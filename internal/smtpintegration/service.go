// Package smtpintegration binds reviewed personal campaign and organiser-test
// intents to durable private mail snapshots and bounded transport receipts.
// Campaign timing and occurrence orchestration remain with the automation owner.
package smtpintegration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/mailcontact"
	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
)

var ErrTestExpired = errors.New("organiser mail test expired")

type (
	Transport interface {
		Configured() bool
		Revision() string
		MessageID(uuid.UUID) string
		Send(context.Context, maildelivery.Message) maildelivery.Receipt
	}
	Service struct {
		Pool        *pgxpool.Pool
		Transport   Transport
		Origin      string
		Campaigns   *communications.Service
		ScopePolicy communications.ScopePolicy
		Now         func() time.Time
	}
	privateSnapshot struct {
		ProviderRevision string               `json:"provider_revision"`
		Message          maildelivery.Message `json:"message"`
	}
)

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s Service) configured() bool {
	return s.Transport != nil && s.Transport.Configured() && s.Transport.Revision() != ""
}

func brand(ctx context.Context, q *dbgen.Queries, eventID uuid.UUID) (maildelivery.Brand, error) {
	row, err := q.Event(ctx, eventID)
	if err != nil {
		return maildelivery.Brand{}, fmt.Errorf("read mail event: %w", err)
	}
	definitions := templates.EmptyDefinitions()
	stored, err := q.EventDefinitions(ctx, eventID)
	if err == nil {
		if err = json.Unmarshal(stored.Definitions, &definitions); err != nil {
			return maildelivery.Brand{}, fmt.Errorf("read mail brand: %w", err)
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return maildelivery.Brand{}, fmt.Errorf("read mail branding: %w", err)
	}
	return maildelivery.Brand{
		Name:       row.Name,
		Primary:    definitions.Branding.Primary,
		Background: definitions.Branding.Background,
		Foreground: definitions.Branding.Foreground,
	}, nil
}

func (s Service) revision(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
) (string, error) {
	current, err := brand(ctx, q, eventID)
	if err != nil {
		return "", err
	}
	data, _ := json.Marshal(struct {
		Transport string             `json:"transport"`
		Origin    string             `json:"origin"`
		Brand     maildelivery.Brand `json:"brand"`
	}{s.Transport.Revision(), s.Origin, current})
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func (s Service) Readiness(
	ctx context.Context,
	eventID, actor uuid.UUID,
) (communications.ChannelReadiness, error) {
	result := communications.ChannelReadiness{
		Channel:          "smtp",
		SupportsSelfTest: true,
		Reason:           "not_configured",
		SelfTestReason:   "not_configured",
	}
	if !s.configured() {
		return result, nil
	}
	revision, err := s.revision(ctx, dbgen.New(s.Pool), eventID)
	if err != nil {
		return result, fmt.Errorf("SMTP readiness: %w", err)
	}
	result.Available = true
	result.ConfigRevision = revision
	result.Reason = "ready"
	result.SelfTestReason = "address_unverified"
	_, err = mailcontact.VerifiedMailboxInTransaction(ctx, dbgen.New(s.Pool), actor)
	if err == nil {
		result.SelfTestAvailable = true
		result.SelfTestReason = "ready"
	} else if !errors.Is(err, mailcontact.ErrUnavailable) && !errors.Is(err, access.ErrUnauthenticated) {
		return result, fmt.Errorf("SMTP self-test readiness: %w", err)
	}
	return result, nil
}

// Eligible performs nonlocking reads because the generic outbox owns its
// operation row here. Deliver rechecks all authority under the event lock before
// recording the immutable effect; taking an event lock here would invert order.
func (s Service) Eligible(ctx context.Context, d jobs.Delivery) (bool, error) {
	if !s.configured() || s.Campaigns == nil {
		return false, nil
	}
	q := dbgen.New(s.Pool)
	request, err := q.CommunicationTestRequest(
		ctx,
		dbgen.CommunicationTestRequestParams{EventID: d.EventID, ID: d.TargetID},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read SMTP test eligibility: %w", err)
	}
	if request.Channel != "smtp" {
		return false, nil
	}
	if err = (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		request.ActorID,
		d.EventID,
		d.EventID,
		access.EventEdit,
	); err != nil {
		if errors.Is(err, access.ErrForbidden) {
			return false, nil
		}
		return false, fmt.Errorf("check SMTP requester authority: %w", err)
	}
	ready, err := s.Readiness(ctx, d.EventID, request.ActorID)
	return ready.SelfTestAvailable, err
}

func outcome(state string) jobs.Outcome {
	switch state {
	case "accepted":
		return jobs.Delivered
	case "retryable":
		return jobs.Retryable
	case "permanent":
		return jobs.Permanent
	default:
		return jobs.Ambiguous
	}
}

func sameDelivery(stored dbgen.SmtpMessageSnapshot, d jobs.Delivery) bool {
	return stored.OperationID == d.ID && stored.EventID == d.EventID &&
		stored.TestRequestID == d.TargetID
}

func (s Service) calendar(
	ctx context.Context,
	q *dbgen.Queries,
	request communications.TestMessage,
) (*maildelivery.Calendar, error) {
	return s.calendarFor(ctx, q, request.EventID, true)
}

func (s Service) calendarFor(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	test bool,
) (*maildelivery.Calendar, error) {
	row, err := q.Event(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("read approved calendar: %w", err)
	}
	var configuration event.Configuration
	if err = json.Unmarshal(row.Configuration, &configuration); err != nil {
		return nil, fmt.Errorf("read event calendar: %w", err)
	}
	calendar := &maildelivery.Calendar{
		EventID:      row.ID,
		Test:         test,
		Start:        configuration.LiveStart,
		End:          configuration.LiveEnd,
		Timezone:     configuration.Timezone,
		Summary:      row.Name,
		WorkspaceURL: strings.TrimRight(s.Origin, "/") + "/events/" + row.ID.String(),
	}
	if test {
		calendar.Summary = "[TEST] " + calendar.Summary
	}
	data, _ := json.Marshal(calendar)
	fingerprint := sha256.Sum256(data)
	sequence, err := q.NextEmailCalendarSequence(
		ctx,
		dbgen.NextEmailCalendarSequenceParams{
			EventID:     row.ID,
			Test:        test,
			Fingerprint: fingerprint[:],
		},
	)
	if err != nil {
		return nil, fmt.Errorf("retain calendar sequence: %w", err)
	}
	calendar.Sequence = int64(sequence)
	return calendar, nil
}

func (s Service) newMessage(
	ctx context.Context,
	q *dbgen.Queries,
	request communications.TestMessage,
	d jobs.Delivery,
	mailbox string,
) (maildelivery.Message, error) {
	return s.newMessageFor(ctx, q, request, d, mailbox, true)
}

func (s Service) newMessageFor(
	ctx context.Context,
	q *dbgen.Queries,
	request communications.TestMessage,
	d jobs.Delivery,
	mailbox string,
	test bool,
) (maildelivery.Message, error) {
	currentBrand, err := brand(ctx, q, d.EventID)
	if err != nil {
		return maildelivery.Message{}, err
	}
	var calendar *maildelivery.Calendar
	body := request.PlainText
	if request.IncludeCalendar {
		calendar, err = s.calendarFor(ctx, q, request.EventID, test)
		if err != nil {
			return maildelivery.Message{}, err
		}
		details, err := maildelivery.DateDetails(*calendar)
		if err != nil {
			return maildelivery.Message{}, fmt.Errorf("render approved dates: %w", err)
		}
		body += "\n\n" + details + "\n" + calendar.WorkspaceURL
	}
	return maildelivery.Message{
		ID:        d.ID,
		Recipient: mailbox,
		Subject:   request.Subject,
		PlainText: body,
		Brand:     currentBrand,
		SentAt:    s.now(),
		Calendar:  calendar,
	}, nil
}

func (s Service) prepare(
	ctx context.Context,
	d jobs.Delivery,
) (maildelivery.Message, jobs.Outcome, bool, error) {
	var message maildelivery.Message
	ready := false
	previous := jobs.Ambiguous
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		// The resolver owns the event lock and current campaign/source recheck.
		request, err := s.Campaigns.ResolveTestSelfInTransaction(ctx, tx, d.EventID, d.TargetID)
		if err != nil {
			return fmt.Errorf("resolve reviewed mail intent: %w", err)
		}
		retained, err := q.CommunicationTestRequest(
			ctx,
			dbgen.CommunicationTestRequestParams{EventID: d.EventID, ID: d.TargetID},
		)
		if err != nil {
			return fmt.Errorf("read mail intent lifetime: %w", err)
		}
		if !s.now().Before(retained.CreatedAt.Add(15 * time.Minute)) {
			return ErrTestExpired
		}
		if request.Channel != "smtp" {
			return access.ErrForbidden
		}
		operation, err := q.Operation(ctx, d.ID)
		if err != nil {
			return fmt.Errorf("read SMTP operation: %w", err)
		}
		if operation.EventID != d.EventID || operation.TargetID != d.TargetID ||
			operation.Kind != "communication_test_smtp" ||
			!operation.Approved ||
			operation.Cancelled || operation.DueAt.After(s.now()) || (operation.State != "running" && operation.State != "pending") {
			return access.ErrForbidden
		}
		revision, err := s.revision(ctx, q, d.EventID)
		if err != nil {
			return err
		}
		if request.ProviderRevision != revision {
			return access.ErrConflict
		}
		mailbox, err := mailcontact.VerifiedMailboxInTransaction(ctx, q, request.ActorID)
		if err != nil {
			return fmt.Errorf("resolve verified test mailbox: %w", err)
		}
		stored, err := q.SMTPMessageSnapshot(ctx, d.ID)
		if err == nil {
			if !sameDelivery(stored, d) || stored.PersonID != request.ActorID ||
				stored.Mailbox != mailbox {
				return access.ErrConflict
			}
			var snapshot privateSnapshot
			if err = json.Unmarshal(stored.Snapshot, &snapshot); err != nil {
				return fmt.Errorf("decode private SMTP snapshot: %w", err)
			}
			if snapshot.ProviderRevision != revision {
				return access.ErrConflict
			}
			previous = outcome(stored.State)
			if stored.State != "retryable" {
				return nil
			}
			// Persist uncertainty before the next actual attempt. A crash cannot leave
			// the earlier retryable receipt falsely authorising an additional send.
			if err = q.RecordSMTPReceipt(
				ctx,
				dbgen.RecordSMTPReceiptParams{
					OperationID: d.ID,
					State:       "prepared",
					ReceiptCode: "",
				},
			); err != nil {
				return fmt.Errorf("prepare SMTP retry: %w", err)
			}
			message = snapshot.Message
			ready = true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read SMTP snapshot: %w", err)
		}
		message, err = s.newMessage(ctx, q, request, d, mailbox)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(privateSnapshot{ProviderRevision: revision, Message: message})
		if err != nil {
			return fmt.Errorf("encode private SMTP attempt: %w", err)
		}
		// A message ID is based solely on immutable operation identity and deployment
		// domain inside the transport; no recipient data enters the operation key.
		if err = q.SaveSMTPMessageSnapshot(
			ctx,
			dbgen.SaveSMTPMessageSnapshotParams{
				OperationID:   d.ID,
				EventID:       d.EventID,
				PersonID:      request.ActorID,
				TestRequestID: d.TargetID,
				Mailbox:       mailbox,
				MessageID:     s.Transport.MessageID(d.ID),
				Snapshot:      encoded,
			},
		); err != nil {
			return fmt.Errorf("retain private SMTP attempt: %w", err)
		}
		ready = true
		return nil
	})
	if err != nil {
		return message, previous, ready, fmt.Errorf("prepare SMTP effect: %w", err)
	}
	return message, previous, ready, nil
}

func (s Service) Deliver(ctx context.Context, d jobs.Delivery) jobs.Outcome {
	if !s.configured() || s.Campaigns == nil {
		return jobs.Permanent
	}
	// Already accepted/uncertain effects are historical facts and never resent,
	// even if later authority changes. This grants no new access or disclosure.
	stored, err := dbgen.New(s.Pool).SMTPMessageSnapshot(ctx, d.ID)
	if err == nil && sameDelivery(stored, d) && stored.State != "retryable" {
		return outcome(stored.State)
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return jobs.Retryable
	}
	message, previous, ready, err := s.prepare(ctx, d)
	if err != nil {
		var blocked *event.Blocked
		if errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrUnauthenticated) ||
			errors.Is(err, access.ErrConflict) ||
			errors.Is(err, access.ErrNotFound) ||
			errors.Is(err, mailcontact.ErrUnavailable) ||
			errors.Is(err, communications.ErrUnresolved) ||
			errors.Is(err, ErrTestExpired) ||
			errors.As(err, &blocked) {
			return jobs.Permanent
		}
		return jobs.Retryable
	}
	if !ready {
		return previous
	}
	receipt := maildelivery.SafeReceipt(s.Transport.Send(ctx, message))
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err = dbgen.New(s.Pool).
		RecordSMTPReceipt(cleanup, dbgen.RecordSMTPReceiptParams{OperationID: d.ID, State: string(receipt.State), ReceiptCode: receipt.Code}); err != nil {
		return jobs.Ambiguous
	}
	return outcome(string(receipt.State))
}

func (s Service) Reconcile(ctx context.Context, d jobs.Delivery) jobs.Outcome {
	stored, err := dbgen.New(s.Pool).SMTPMessageSnapshot(ctx, d.ID)
	if err != nil || !sameDelivery(stored, d) {
		return jobs.Ambiguous
	}
	if stored.State != "retryable" {
		return outcome(stored.State)
	}
	// A prior explicit SMTP rejection proves no acceptance for that attempt.
	// Rebuild current authority/source permission before allowing a new attempt.
	if allowed, err := s.Eligible(ctx, d); err != nil || !allowed {
		return jobs.Permanent
	}
	var snapshot privateSnapshot
	if json.Unmarshal(stored.Snapshot, &snapshot) != nil {
		return jobs.Ambiguous
	}
	revision, err := s.revision(ctx, dbgen.New(s.Pool), d.EventID)
	if err != nil {
		return jobs.Ambiguous
	}
	mailbox, err := mailcontact.VerifiedMailboxInTransaction(
		ctx,
		dbgen.New(s.Pool),
		stored.PersonID,
	)
	if err != nil || mailbox != stored.Mailbox || revision != snapshot.ProviderRevision {
		return jobs.Permanent
	}
	return jobs.Retryable
}

func (s Service) Maintain(ctx context.Context) error {
	if _, err := dbgen.New(s.Pool).
		CancelExpiredSMTPTestOperations(ctx, s.now().Add(-15*time.Minute)); err != nil {
		return fmt.Errorf("cancel expired SMTP tests: %w", err)
	}
	return nil
}
