package smtpintegration

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type fakeTransport struct {
	mu       sync.Mutex
	revision string
	state    maildelivery.State
	outcomes []maildelivery.State
	messages []maildelivery.Message
}

func (*fakeTransport) Configured() bool   { return true }
func (p *fakeTransport) Revision() string { return p.revision }
func (*fakeTransport) MessageID(id uuid.UUID) string {
	return "<hbox." + id.String() + "@example.test>"
}

func (p *fakeTransport) Send(_ context.Context, message maildelivery.Message) maildelivery.Receipt {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.messages = append(p.messages, message)
	if len(p.outcomes) > 0 {
		p.state = p.outcomes[0]
		p.outcomes = p.outcomes[1:]
	}
	return maildelivery.Receipt{
		State: p.state,
		Code:  map[maildelivery.State]string{maildelivery.Accepted: "relay_accepted", maildelivery.Retryable: "relay_temporary_rejection", maildelivery.Permanent: "relay_rejection", maildelivery.Ambiguous: "acceptance_unknown"}[p.state],
	}
}
func (p *fakeTransport) count() int { p.mu.Lock(); defer p.mu.Unlock(); return len(p.messages) }

type fixture struct {
	t         *testing.T
	pool      *pgxpool.Pool
	owner     dbgen.Person
	event     uuid.UUID
	service   *Service
	comm      *communications.Service
	transport *fakeTransport
	worker    *jobs.Service
	campaign  communications.Campaign
	now       time.Time
}

func setup(t *testing.T) *fixture {
	t.Helper()
	pool := testdb.New(t, true)
	owner := testdb.Person(t, pool)
	id := testdb.Event(t, pool, owner.ID)
	f := &fixture{
		t:         t,
		pool:      pool,
		owner:     owner,
		event:     id,
		transport: &fakeTransport{revision: "synthetic-relay-v1", state: maildelivery.Accepted},
		now:       time.Now().UTC().Truncate(time.Microsecond),
	}
	configuration := event.DefaultConfiguration()
	configuration.LiveStart = f.now.AddDate(0, 1, 0)
	configuration.LiveEnd = configuration.LiveStart.Add(8 * time.Hour)
	configuration.SubmissionCutoff = configuration.LiveEnd.Add(-time.Hour)
	configuration.TeamFreeze = configuration.LiveStart.AddDate(0, 0, -8)
	configuration.AdmissionCutoff = configuration.TeamFreeze.Add(-time.Hour)
	data, _ := json.Marshal(configuration)
	f.exec("UPDATE events SET configuration=$2,stage='registration' WHERE id=$1", id, data)
	f.exec("INSERT INTO event_gate_facts(event_id) VALUES($1)", id)
	if err := dbgen.New(pool).
		ConfirmPersonEmail(t.Context(), dbgen.ConfirmPersonEmailParams{PersonID: owner.ID, Mailbox: "organiser@example.test"}); err != nil {
		t.Fatal(err)
	}
	f.comm = &communications.Service{Pool: pool, Now: func() time.Time { return f.now }}
	f.service = &Service{
		Pool:      pool,
		Transport: f.transport,
		Origin:    "https://hack.example.test",
		Campaigns: f.comm,
		Now:       func() time.Time { return f.now },
	}
	f.comm.Channels = map[string]communications.ChannelProvider{"smtp": f.service}
	worker, err := jobs.New(
		pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{"communication_test_smtp": f.service},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.worker = worker
	f.comm.Jobs = worker
	template := communications.Template{
		ID:      uuid.New(),
		Name:    "Synthetic safe reminder",
		Purpose: "reminder",
		Format:  "html",
		Subject: "Hello {{person.name}}",
		Body:    "{{event.name}} for {{person.name}}",
	}
	audience := communications.Audience{
		ID:                 uuid.New(),
		Name:               "Current organisers",
		Roles:              []string{"organiser"},
		RegistrationStates: []string{},
		Teams:              []uuid.UUID{},
	}
	for _, command := range []communications.Command{{Action: "save_template", Template: &template}, {Action: "save_audience", Audience: &audience}} {
		command.Reason = "Synthetic reviewed source"
		if _, err = f.comm.Change(t.Context(), owner.ID, id, f.revision(), command); err != nil {
			t.Fatal(err)
		}
	}
	definition := communications.CampaignDefinition{
		ID:              uuid.New(),
		Name:            "Synthetic organiser test",
		TemplateID:      template.ID,
		AudienceID:      audience.ID,
		Channels:        []string{"smtp"},
		IncludeCalendar: true,
		Timing: communications.CampaignTiming{
			Kind:   "relative",
			Anchor: "live_start",
			Days:   -1,
		},
	}
	result, err := f.comm.ChangeCampaign(
		t.Context(),
		owner.ID,
		id,
		definition.ID,
		f.revision(),
		communications.CampaignCommand{
			Action:     "save",
			Definition: &definition,
			Reason:     "Synthetic campaign review",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.campaign = result.Campaign.Campaign
	return f
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.t.Context(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) revision() int64 {
	f.t.Helper()
	row, err := dbgen.New(f.pool).Event(f.t.Context(), f.event)
	if err != nil {
		f.t.Fatal(err)
	}
	return row.Revision
}

func (f *fixture) queue() jobs.Delivery {
	f.t.Helper()
	result, err := f.comm.ChangeCampaign(
		f.t.Context(),
		f.owner.ID,
		f.event,
		f.campaign.Definition.ID,
		f.revision(),
		communications.CampaignCommand{
			Action:  "test_self",
			Channel: "smtp",
			Version: f.campaign.Version,
			Reason:  "Requesting organiser only",
		},
	)
	if err != nil || result.TestRequestID == nil {
		f.t.Fatalf("queue test:%v", err)
	}
	operation, err := dbgen.New(f.pool).
		SMTPTestOperation(f.t.Context(), dbgen.SMTPTestOperationParams{EventID: f.event, TargetID: *result.TestRequestID})
	if err != nil {
		f.t.Fatal(err)
	}
	return jobs.Delivery{
		ID:       operation.ID,
		EventID:  operation.EventID,
		TargetID: operation.TargetID,
		Key:      operation.ID.String(),
	}
}

func TestSMTPOutboxAcceptanceAndPrivateImmutableSnapshot(t *testing.T) {
	f := setup(t)
	d := f.queue()
	if err := f.worker.Execute(t.Context(), d.ID); err != nil {
		t.Fatal(err)
	}
	stored, err := dbgen.New(f.pool).SMTPMessageSnapshot(t.Context(), d.ID)
	if err != nil || stored.State != "accepted" || stored.PersonID != f.owner.ID ||
		stored.Mailbox != "organiser@example.test" ||
		stored.MessageID != f.transport.MessageID(d.ID) {
		t.Fatal("transport acceptance/actual recipient not retained privately")
	}
	if f.transport.count() != 1 {
		t.Fatal("accepted test did not send once")
	}
	for _, mutation := range []string{
		"mailbox='redirected@example.test'",
		"message_id='<different@example.test>'",
		"snapshot='{}'::jsonb",
		"created_at=created_at+interval '1 second'",
	} {
		if _, err = f.pool.Exec(t.Context(), "UPDATE smtp_message_snapshots SET "+mutation+" WHERE operation_id=$1", d.ID); err == nil {
			t.Fatal("database permitted immutable SMTP attempt mutation")
		}
	}
	message := f.transport.messages[0]
	if message.Recipient != "organiser@example.test" ||
		!strings.Contains(message.Subject, "[TEST]") ||
		message.Calendar == nil ||
		!message.Calendar.Test ||
		message.Calendar.Sequence != 0 {
		t.Fatal("test redirected recipient or lost isolated calendar")
	}
	if err = f.worker.Execute(
		t.Context(),
		d.ID,
	); err != nil || f.service.Reconcile(t.Context(), d) != jobs.Delivered ||
		f.transport.count() != 1 {
		t.Fatal("accepted effect was resent")
	}
	var operationState string
	if err = f.pool.QueryRow(t.Context(), "SELECT state FROM operations WHERE id=$1", d.ID).
		Scan(&operationState); err != nil ||
		operationState != "delivered" {
		t.Fatal("generic outbox acceptance result not recorded")
	}
	// Generic operation/audit records contain references only. Recipient and body
	// remain in the specifically private snapshot table, never the outbox payload.
	var publicRecords string
	if err = f.pool.QueryRow(t.Context(), "SELECT coalesce((SELECT string_agg(row_to_json(o)::text,' ') FROM operations o),'') || coalesce((SELECT string_agg(row_to_json(a)::text,' ') FROM audit_events a),'')").
		Scan(&publicRecords); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(publicRecords, message.Recipient) ||
		strings.Contains(publicRecords, message.PlainText) {
		t.Fatal("private recipient/content leaked to generic records")
	}
}

func TestSMTPRejectedRetryRetainsCalendarAndAmbiguousNeverResends(t *testing.T) {
	f := setup(t)
	d := f.queue()
	f.transport.state = maildelivery.Retryable
	if result := f.service.Deliver(t.Context(), d); result != jobs.Retryable {
		t.Fatal("definite rejection not retryable")
	}
	first := f.transport.messages[0]
	if f.service.Reconcile(t.Context(), d) != jobs.Retryable {
		t.Fatal("definite rejection cannot be reconciled safely")
	}
	f.transport.state = maildelivery.Accepted
	if result := f.service.Deliver(t.Context(), d); result != jobs.Delivered {
		t.Fatal("safe retry not accepted")
	}
	second := f.transport.messages[1]
	if first.ID != second.ID || !first.SentAt.Equal(second.SentAt) ||
		first.Calendar.Sequence != second.Calendar.Sequence ||
		first.Calendar.EventID != second.Calendar.EventID {
		t.Fatal("retry changed immutable message/calendar identity")
	}
	next := f.queue()
	f.transport.state = maildelivery.Ambiguous
	if result := f.service.Deliver(t.Context(), next); result != jobs.Ambiguous {
		t.Fatal("uncertain SMTP acceptance mislabeled")
	}
	count := f.transport.count()
	if f.service.Reconcile(t.Context(), next) != jobs.Ambiguous ||
		f.service.Deliver(t.Context(), next) != jobs.Ambiguous ||
		f.transport.count() != count {
		t.Fatal("ambiguous send automatically retried")
	}
}

func TestSMTPCurrentAuthorityMailboxAndProviderChangesBlockPendingEffects(t *testing.T) {
	for _, change := range []string{"role", "mailbox", "transport", "origin", "brand", "cancel", "source", "cross_event"} {
		t.Run(change, func(t *testing.T) {
			f := setup(t)
			d := f.queue()
			switch change {
			case "role":
				f.exec(
					"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='organiser'",
					f.event,
					f.owner.ID,
				)
			case "mailbox":
				f.exec("DELETE FROM person_email_contacts WHERE person_id=$1", f.owner.ID)
			case "transport":
				f.transport.revision = "synthetic-relay-v2"
			case "origin":
				f.service.Origin = "https://other.example.test"
			case "brand":
				f.exec(
					"INSERT INTO event_definitions(event_id,definitions) VALUES($1,$2)",
					f.event,
					[]byte(
						`{"branding":{"primary":"#000000","background":"#ffffff","foreground":"#111111"}}`,
					),
				)
			case "cancel":
				_, err := f.comm.ChangeCampaign(
					t.Context(),
					f.owner.ID,
					f.event,
					f.campaign.Definition.ID,
					f.revision(),
					communications.CampaignCommand{
						Action:  "cancel",
						Version: f.campaign.Version,
						Reason:  "Stop test",
					},
				)
				if err != nil {
					t.Fatal(err)
				}
			case "source":
				f.exec("UPDATE events SET name='Changed approved event' WHERE id=$1", f.event)
			case "cross_event":
				d.EventID = testdb.Event(t, f.pool, f.owner.ID)
			}
			if result := f.service.Deliver(
				t.Context(),
				d,
			); result != jobs.Permanent ||
				f.transport.count() != 0 {
				t.Fatalf("%s change permitted SMTP effect: %s", change, result)
			}
		})
	}
}

func TestSMTPPreparedCrashAndChangedRetryDestinationAreFailClosed(t *testing.T) {
	f := setup(t)
	d := f.queue()
	_, _, ready, err := f.service.prepare(t.Context(), d)
	if err != nil || !ready {
		t.Fatal("could not prepare synthetic crash")
	}
	if result := f.service.Reconcile(t.Context(), d); result != jobs.Ambiguous {
		t.Fatal("prepared crash incorrectly proved safe to retry")
	}
	if result := f.service.Deliver(
		t.Context(),
		d,
	); result != jobs.Ambiguous ||
		f.transport.count() != 0 {
		t.Fatal("prepared crash automatically emitted SMTP")
	}
	next := f.queue()
	f.transport.state = maildelivery.Retryable
	if f.service.Deliver(t.Context(), next) != jobs.Retryable {
		t.Fatal("synthetic rejection failed")
	}
	f.exec(
		"UPDATE person_email_contacts SET mailbox='new-confirmed@example.test' WHERE person_id=$1",
		f.owner.ID,
	)
	count := f.transport.count()
	if result := f.service.Deliver(
		t.Context(),
		next,
	); result != jobs.Permanent ||
		f.transport.count() != count {
		t.Fatal("retry silently redirected immutable recipient")
	}
}

type fastRetry struct{}

func (fastRetry) NextRetry(
	*rivertype.JobRow,
) time.Time {
	return time.Now().Add(50 * time.Millisecond)
}

func TestSMTPTemporaryRejectionRetriesThroughActualRiverWorker(t *testing.T) {
	f := setup(t)
	f.transport.outcomes = []maildelivery.State{maildelivery.Retryable, maildelivery.Accepted}
	workers := river.NewWorkers()
	river.AddWorker(workers, f.worker)
	client, err := river.NewClient(
		riverpgxv5.New(f.pool),
		&river.Config{
			Workers:           workers,
			Queues:            map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
			RetryPolicy:       fastRetry{},
			FetchCooldown:     5 * time.Millisecond,
			FetchPollInterval: 20 * time.Millisecond,
			Logger:            slog.New(slog.NewTextHandler(io.Discard, nil)),
			JobTimeout:        5 * time.Second,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.worker.Client = client
	d := f.queue()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	if err = client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(t.Context()), 3*time.Second)
		defer stop()
		if err := client.Stop(cleanup); err != nil {
			t.Error(err)
		}
	})
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		operation, err := dbgen.New(f.pool).Operation(ctx, d.ID)
		if err != nil {
			t.Fatal(err)
		}
		if operation.State == "delivered" {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("actual River worker did not retry the temporary rejection")
		case <-ticker.C:
		}
	}
	if err = client.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if f.transport.count() != 2 {
		t.Fatal("temporary SMTP rejection did not yield exactly one bounded retry")
	}
	first, second := f.transport.messages[0], f.transport.messages[1]
	if first.ID != second.ID || !first.SentAt.Equal(second.SentAt) ||
		first.Calendar.Sequence != second.Calendar.Sequence {
		t.Fatal("River retry changed message/calendar identity")
	}
	var attempts int
	var outcomes string
	if err = f.pool.QueryRow(ctx, "SELECT count(*),string_agg(outcome,',' ORDER BY started_at) FROM delivery_attempts WHERE operation_id=$1", d.ID).
		Scan(&attempts, &outcomes); err != nil ||
		attempts != 2 ||
		outcomes != "retryable,delivered" {
		t.Fatal("River retry lost durable provider attempt evidence")
	}
}

func TestSMTPReceiptStorageFaultLeavesAmbiguousRatherThanResending(t *testing.T) {
	f := setup(t)
	d := f.queue()
	f.exec(
		"CREATE FUNCTION reject_smtp_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='accepted' THEN RAISE EXCEPTION 'synthetic receipt outage'; END IF; RETURN NEW; END $$",
	)
	f.exec(
		"CREATE TRIGGER smtp_receipt_fault BEFORE UPDATE ON smtp_message_snapshots FOR EACH ROW EXECUTE FUNCTION reject_smtp_receipt()",
	)
	if result := f.service.Deliver(
		t.Context(),
		d,
	); result != jobs.Ambiguous ||
		f.transport.count() != 1 {
		t.Fatal("receipt outage was reported as safe retry or acceptance")
	}
	f.exec("DROP TRIGGER smtp_receipt_fault ON smtp_message_snapshots")
	if f.service.Reconcile(t.Context(), d) != jobs.Ambiguous ||
		f.service.Deliver(t.Context(), d) != jobs.Ambiguous ||
		f.transport.count() != 1 {
		t.Fatal("lost acceptance receipt caused a duplicate send")
	}
}

func TestSMTPTestExpiryBoundaryAndUncertainReceiptSurviveCancellation(t *testing.T) {
	f := setup(t)
	d := f.queue()
	var due time.Time
	if err := f.pool.QueryRow(t.Context(), "SELECT due_at FROM operations WHERE id=$1", d.ID).Scan(&due); err != nil {
		t.Fatal(err)
	}
	f.now = due.Add(15*time.Minute - time.Microsecond)
	if err := f.service.Maintain(t.Context()); err != nil {
		t.Fatal(err)
	}
	status, err := f.service.Status(t.Context(), f.owner.ID, f.event, d.TargetID)
	if err != nil || status.State != "pending" {
		t.Fatal("test expired before exclusive lifetime boundary")
	}
	f.now = due.Add(15 * time.Minute)
	if err = f.service.Maintain(t.Context()); err != nil {
		t.Fatal(err)
	}
	status, err = f.service.Status(t.Context(), f.owner.ID, f.event, d.TargetID)
	if err != nil || status.State != "cancelled" || f.transport.count() != 0 {
		t.Fatal("expired pending test was not cancelled without an external effect")
	}
	f.now = time.Now().UTC()
	uncertain := f.queue()
	f.transport.state = maildelivery.Ambiguous
	if f.service.Deliver(t.Context(), uncertain) != jobs.Ambiguous {
		t.Fatal("fixture did not retain uncertain external effect")
	}
	f.exec("UPDATE operations SET cancelled=true,state='cancelled' WHERE id=$1", uncertain.ID)
	status, err = f.service.Status(t.Context(), f.owner.ID, f.event, uncertain.TargetID)
	if err != nil || status.State != "ambiguous" || f.transport.count() != 1 {
		t.Fatal("cancellation obscured an uncertain external effect")
	}
}
