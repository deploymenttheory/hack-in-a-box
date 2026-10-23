package automation

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

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/deliveryhistory"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
	"github.com/deploymenttheory/hack-in-a-box/internal/smtpintegration"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type capturingAcceptedRelay struct {
	mu       sync.Mutex
	messages []maildelivery.Message
}

func (*capturingAcceptedRelay) Configured() bool { return true }
func (*capturingAcceptedRelay) Revision() string { return "synthetic-captured-relay" }
func (*capturingAcceptedRelay) MessageID(id uuid.UUID) string {
	return "<" + id.String() + "@example.test>"
}

func (r *capturingAcceptedRelay) Send(
	_ context.Context,
	message maildelivery.Message,
) maildelivery.Receipt {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, message)
	return maildelivery.Receipt{State: maildelivery.Accepted, Code: "relay_accepted"}
}

func (r *capturingAcceptedRelay) snapshot() []maildelivery.Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]maildelivery.Message(nil), r.messages...)
}

func TestAutomationSMTPAcceptanceWithoutReceiptSurvivesRestartWithoutDuplicatingLocalOrExternalEffect(
	t *testing.T,
) {
	f := newFixture(t)
	smtp, _ := installProviders(t, f)
	relay := &capturingAcceptedRelay{}
	smtp.Transport = relay
	q := dbgen.New(f.pool)
	if err := q.ConfirmPersonEmail(
		t.Context(),
		dbgen.ConfirmPersonEmailParams{PersonID: f.person.ID, Mailbox: "intended@example.test"},
	); err != nil {
		t.Fatal(err)
	}
	outsider := testdb.Person(t, f.pool)
	if err := q.ConfirmPersonEmail(
		t.Context(),
		dbgen.ConfirmPersonEmailParams{PersonID: outsider.ID, Mailbox: "unintended@example.test"},
	); err != nil {
		t.Fatal(err)
	}
	f.campaign(
		"reminder",
		"Reviewed event reminder, without private source prose.",
		true,
		"smtp",
		"in_app",
	)
	binding := f.bind()
	id := f.maintain().Occurrences[0].ID
	if page := f.maintain(); len(page.Occurrences) != 1 {
		t.Fatal("duplicate clock sweep created another occurrence")
	}
	f.now = f.now.Add(time.Minute)
	f.execute(id)
	f.execute(id)
	var operation uuid.UUID
	if err := f.pool.QueryRow(t.Context(), "SELECT operation_id FROM smtp_campaign_messages WHERE occurrence_id=$1", id).
		Scan(&operation); err != nil {
		t.Fatal(err)
	}
	// The relay accepts, but the process cannot durably record that receipt.
	// The already committed prepared barrier is the only safe recovery evidence.
	f.exec(
		"CREATE FUNCTION synthetic_receipt_outage() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic durable receipt outage'; END $$",
	)
	f.exec(
		"CREATE TRIGGER synthetic_receipt_outage BEFORE INSERT ON smtp_submission_receipts FOR EACH ROW EXECUTE FUNCTION synthetic_receipt_outage()",
	)
	if err := f.s.Jobs.Execute(t.Context(), operation); err != nil {
		t.Fatal(err)
	}
	f.exec("DROP TRIGGER synthetic_receipt_outage ON smtp_submission_receipts")
	// A fresh Jobs service models a restarted consumer. Neither old jobs, another
	// scheduler sweep nor reconciliation may infer that acceptance was rejected.
	restarted, err := jobs.New(
		f.pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{
			HandlerKind:                   f.s,
			"communication_campaign_smtp": smtpintegration.CampaignHandler{Service: smtp},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err = restarted.Execute(t.Context(), operation); err != nil {
			t.Fatal(err)
		}
		if err = restarted.Reconcile(t.Context()); err != nil {
			t.Fatal(err)
		}
		f.maintain()
		f.execute(id)
	}
	f.change(
		Command{
			Action:    "cancel",
			BindingID: binding.Definition.ID,
			Version:   binding.Version,
			Reason:    "Stop future automation; do not claim accepted relay effect recalled",
		},
	)
	if err = restarted.Execute(t.Context(), operation); err != nil {
		t.Fatal(err)
	}
	var local, intents, attempts, receipts int
	if err = f.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM inbox_notifications WHERE source_kind='campaign'),(SELECT count(*) FROM smtp_campaign_messages),(SELECT count(*) FROM smtp_submission_attempts),(SELECT count(*) FROM smtp_submission_receipts)`).
		Scan(&local, &intents, &attempts, &receipts); err != nil || local != 1 || intents != 1 ||
		attempts != 1 ||
		receipts != 0 {
		t.Fatal(
			"crash recovery changed durable effect evidence",
			local,
			intents,
			attempts,
			receipts,
			err,
		)
	}
	messages := relay.snapshot()
	if len(messages) != 1 || messages[0].Recipient != "intended@example.test" {
		t.Fatal("recovery duplicated or redirected the individual envelope")
	}
	// The private recipient is allowed only in its own SMTP envelope, never in
	// common history, another participant's envelope or shared message prose.
	if strings.Contains(messages[0].PlainText, "@") {
		t.Fatal("participant address leaked into common prose")
	}
	history := deliveryhistory.Service{Pool: f.pool}
	detail, err := history.Detail(t.Context(), f.owner.ID, f.id, operation, 0, nil)
	if err != nil || detail.Item.State != "ambiguous" || !detail.Item.Attempted ||
		len(detail.Attempts) != 1 {
		t.Fatal("lost acceptance was reclassified as safe failure", err)
	}
	raw, _ := json.Marshal(detail)
	if strings.Contains(string(raw), "@example.test") ||
		strings.Contains(string(raw), "Reviewed event reminder") {
		t.Fatal("private envelope or rendered message entered history")
	}
}
