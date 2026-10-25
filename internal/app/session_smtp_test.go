package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/deliveryhistory"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
)

type sessionMailTransport struct {
	mu       sync.Mutex
	messages []maildelivery.Message
	outcome  maildelivery.State
}

func (*sessionMailTransport) Configured() bool { return true }
func (*sessionMailTransport) Revision() string { return "synthetic-session-relay-v1" }
func (*sessionMailTransport) MessageID(id uuid.UUID) string {
	return "<session." + id.String() + "@example.test>"
}

func (p *sessionMailTransport) Send(
	_ context.Context,
	message maildelivery.Message,
) maildelivery.Receipt {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.messages = append(p.messages, message)
	state := p.outcome
	if state == "" {
		state = maildelivery.Accepted
	}
	code := "relay_accepted"
	if state == maildelivery.Ambiguous {
		code = "acceptance_unknown"
	}
	return maildelivery.Receipt{State: state, Code: code}
}

func (p *sessionMailTransport) sent() []maildelivery.Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]maildelivery.Message{}, p.messages...)
}

func (f *sessionNotificationFixture) smtp(
	t *testing.T,
	state maildelivery.State,
) *sessionMailTransport {
	t.Helper()
	transport := &sessionMailTransport{outcome: state}
	f.runtime.SMTP.Transport = transport
	if err := dbgen.New(f.pool).
		ConfirmPersonEmail(t.Context(), dbgen.ConfirmPersonEmailParams{PersonID: f.person.ID, Mailbox: "synthetic-attendee@example.test"}); err != nil {
		t.Fatal(err)
	}
	return transport
}

func (f *sessionNotificationFixture) operation(t *testing.T, state string) (uuid.UUID, string) {
	t.Helper()
	var id uuid.UUID
	var snapshot string
	if err := f.pool.QueryRow(t.Context(), `SELECT m.operation_id,m.snapshot::text FROM smtp_campaign_messages m JOIN session_notification_occurrences n ON n.id=m.occurrence_id WHERE n.event_id=$1 AND n.session_id=$2 AND n.booking_state=$3 ORDER BY n.created_at DESC,n.id DESC LIMIT 1`, f.eventID, f.sessionID, state).
		Scan(&id, &snapshot); err != nil {
		t.Fatal(err)
	}
	return id, snapshot
}

func TestSessionSMTPWithdrawalSuppressesQueuedNormalAndSendsMinimalHigherSequenceCancel(
	t *testing.T,
) {
	f := newSessionNotificationFixture(t)
	ctx := t.Context()
	transport := f.smtp(t, maildelivery.Accepted)
	f.approve(t, []string{"smtp"})
	f.reserve(t)
	original, originalSnapshot := f.operation(t, "booked")
	if len(transport.sent()) != 0 {
		t.Fatal("queue sent provider effect")
	}
	var prior struct {
		Message maildelivery.Message `json:"message"`
	}
	if err := json.Unmarshal([]byte(originalSnapshot), &prior); err != nil {
		t.Fatal(err)
	}
	if prior.Message.Calendar == nil || prior.Message.Calendar.Cancelled ||
		prior.Message.Calendar.SessionID == nil {
		t.Fatal("native session calendar not captured")
	}
	if _, err := f.registration.Respond(
		ctx,
		f.person.ID,
		f.eventID,
		f.registrationID,
		1,
		"withdraw",
	); err != nil {
		t.Fatal(err)
	}
	if err := f.runtime.Jobs.Execute(ctx, original); err != nil {
		t.Fatal(err)
	}
	if len(transport.sent()) != 0 {
		t.Fatal("queued ordinary update bypassed current audience loss")
	}
	cancellation, _ := f.operation(t, "ineligible")
	if err := f.runtime.Jobs.Execute(ctx, cancellation); err != nil {
		t.Fatal(err)
	}
	messages := transport.sent()
	if len(messages) != 1 {
		t.Fatal("safe cancellation was silently lost", len(messages))
	}
	message := messages[0]
	if message.Calendar == nil || !message.Calendar.Cancelled ||
		message.Calendar.Sequence <= prior.Message.Calendar.Sequence ||
		*message.Calendar.SessionID != *prior.Message.Calendar.SessionID ||
		*message.Calendar.PersonID != *prior.Message.Calendar.PersonID {
		t.Fatal("cancellation lost original UID/sequence", message.Calendar)
	}
	raw, _ := json.Marshal(message)
	if strings.Contains(string(raw), "PRIVATE") {
		t.Fatal("withdrawn recipient received private event/session/link payload", string(raw))
	}
	settings := maildelivery.Settings{
		Host:          "localhost",
		Port:          587,
		Mode:          "starttls",
		Username:      "synthetic",
		Password:      "synthetic",
		From:          "sender@example.test",
		MessageDomain: "example.test",
		Timeout:       time.Second,
	}
	mime, _, err := maildelivery.Build(settings, message)
	if err != nil || !strings.Contains(string(mime), "method=CANCEL") ||
		strings.Contains(string(mime), "PRIVATE") {
		t.Fatal("actual cancellation MIME contract", err)
	}
	if err = f.runtime.Jobs.Execute(ctx, cancellation); err != nil || len(transport.sent()) != 1 {
		t.Fatal("accepted cancellation replayed", err)
	}
	var after string
	if err = f.pool.QueryRow(ctx, `SELECT snapshot::text FROM smtp_campaign_messages WHERE operation_id=$1`, original).
		Scan(&after); err != nil ||
		after != originalSnapshot {
		t.Fatal("authority loss rewrote original attempt snapshot", err)
	}
}

func TestSessionSMTPUnknownEffectCannotBeReplacedByNewConfigurationOrGet(t *testing.T) {
	f := newSessionNotificationFixture(t)
	ctx := t.Context()
	transport := f.smtp(t, maildelivery.Ambiguous)
	f.approve(t, []string{"smtp"})
	f.reserve(t)
	id, originalSnapshot := f.operation(t, "booked")
	if err := f.runtime.Jobs.Execute(ctx, id); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := f.runtime.Jobs.Execute(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.runtime.Jobs.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if len(transport.sent()) != 1 {
		t.Fatal("unknown SMTP effect was replayed", len(transport.sent()))
	}
	page, err := f.runtime.SessionBookings.Read(ctx, f.owner.ID, f.eventID, f.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.runtime.SessionBookings.SaveNotificationConfiguration(
		ctx,
		f.owner.ID,
		f.eventID,
		f.sessionID,
		page.Revision,
		[]string{"in_app"},
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("new config invalidated unresolved original source", err)
	}
	var version int64
	var state, snapshot string
	if err = f.pool.QueryRow(ctx, `SELECT s.notification_version,o.state,m.snapshot::text FROM bookable_sessions s JOIN operations o ON o.id=$3 JOIN smtp_campaign_messages m ON m.operation_id=o.id WHERE s.event_id=$1 AND s.id=$2`, f.eventID, f.sessionID, id).
		Scan(&version, &state, &snapshot); err != nil ||
		version != 1 ||
		state != "ambiguous" ||
		snapshot != originalSnapshot {
		t.Fatal("read/new approval changed original unknown state", version, state, err)
	}
	detail, err := f.runtime.Deliveries.Detail(ctx, f.owner.ID, f.eventID, id, 0, nil)
	if err != nil || !slices.Contains(detail.Actions, "resolve_no_resend") {
		t.Fatal("existing provider review cannot retain unknown without resend", detail, err)
	}
	if _, err = f.runtime.Deliveries.Change(
		ctx,
		f.owner.ID,
		f.eventID,
		id,
		detail.Item.Revision,
		deliveryhistory.Command{
			RequestID: uuid.New(),
			Action:    "resolve_no_resend",
			Reason:    "Synthetic explicit retain-unknown disposition; do not resend",
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err = f.runtime.SessionBookings.SaveNotificationConfiguration(
		ctx,
		f.owner.ID,
		f.eventID,
		f.sessionID,
		page.Revision,
		[]string{"in_app"},
	); err != nil {
		t.Fatal("reviewed no-resend disposition did not release replacement guard", err)
	}
	if len(transport.sent()) != 1 {
		t.Fatal("review disposition or replacement sent original again")
	}
}

func TestSessionSMTPDateChangeAndRollbackRetainApprovedOriginals(t *testing.T) {
	f := newSessionNotificationFixture(t)
	ctx := t.Context()
	transport := f.smtp(t, maildelivery.Accepted)
	f.approve(t, []string{"smtp"})
	f.reserve(t)
	initial, initialSnapshot := f.operation(t, "booked")
	if err := f.runtime.Jobs.Execute(ctx, initial); err != nil {
		t.Fatal(err)
	}
	current, err := f.events.Read(ctx, f.owner.ID, f.eventID)
	if err != nil {
		t.Fatal(err)
	}
	c := current.Configuration
	c.LiveStart = c.LiveStart.AddDate(0, 0, 8)
	c.LiveEnd = c.LiveEnd.AddDate(0, 0, 8)
	c.SubmissionCutoff = c.SubmissionCutoff.AddDate(0, 0, 8)
	if _, err = f.events.Update(ctx, f.owner.ID, f.eventID, current.Revision, event.Input{ID: f.eventID, Name: current.Name, Slug: current.Slug, Configuration: c, Reason: "Synthetic postponed session"}); err != nil {
		t.Fatal(err)
	}
	changed, changedSnapshot := f.operation(t, "booked")
	if changed == initial {
		t.Fatal("date change did not retain a new approved occurrence")
	}
	db := stdlib.OpenDBFromPool(f.pool)
	defer func() { _ = db.Close() }()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS("../database/migrations"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(ctx, 47); err == nil {
		t.Fatal("rollback erased approved session deliveries")
	}
	var version int64
	if err = f.pool.QueryRow(ctx, `SELECT version_id FROM goose_db_version WHERE is_applied ORDER BY id DESC LIMIT 1`).Scan(&version); err != nil || version != 49 {
		t.Fatal("rejected rollback changed owned schema", version, err)
	}
	var retained string
	if err = f.pool.QueryRow(ctx, `SELECT snapshot::text FROM smtp_campaign_messages WHERE operation_id=$1`, changed).Scan(&retained); err != nil || retained != changedSnapshot {
		t.Fatal("rollback rewrote approved delivery", err)
	}
	if err = f.runtime.Jobs.Execute(ctx, changed); err != nil {
		t.Fatal(err)
	}
	messages := transport.sent()
	if len(messages) != 2 || messages[0].Calendar == nil || messages[1].Calendar == nil {
		t.Fatal("date change not delivered", len(messages))
	}
	before, after := messages[0].Calendar, messages[1].Calendar
	if after.Sequence <= before.Sequence || after.Start.Equal(before.Start) || *after.SessionID != *before.SessionID || *after.PersonID != *before.PersonID || after.EventID != before.EventID {
		t.Fatal("date update lost stable calendar identity or sequence", before, after)
	}
	if _, err = f.registration.Respond(ctx, f.person.ID, f.eventID, f.registrationID, 1, "withdraw"); err != nil {
		t.Fatal(err)
	}
	cancel, _ := f.operation(t, "ineligible")
	if err = f.runtime.Jobs.Execute(ctx, cancel); err != nil {
		t.Fatal(err)
	}
	messages = transport.sent()
	if len(messages) != 3 || !messages[2].Calendar.Cancelled || messages[2].Calendar.Sequence <= after.Sequence {
		t.Fatal("cancellation did not supersede changed date", messages)
	}
	if err = f.pool.QueryRow(ctx, `SELECT snapshot::text FROM smtp_campaign_messages WHERE operation_id=$1`, initial).Scan(&retained); err != nil || retained != initialSnapshot {
		t.Fatal("date/cancel rewrote original attempt", err)
	}
}
