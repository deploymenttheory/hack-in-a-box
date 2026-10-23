package teamsworkflow

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/automation"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/deliveryhistory"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
)

func TestAutomationTeamsTLSAcceptanceReceiptFaultCannotReplayAfterConsumerRestart(t *testing.T) {
	f := newFixture(t)
	auto := &automation.Service{
		Pool:     f.pool,
		Composer: &f.s.Composer,
		Now:      func() time.Time { return f.now },
	}
	f.s.ScopePolicy = auto
	outbox, err := jobs.New(
		f.pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{automation.HandlerKind: auto, HandlerKind(): f.s},
	)
	if err != nil {
		t.Fatal(err)
	}
	auto.Jobs, f.s.Jobs, f.s.Composer.Jobs = outbox, outbox, outbox
	auto.Providers = map[string]automation.ProviderQueue{
		"teams": func(ctx context.Context, tx pgx.Tx, eventID, campaignID uuid.UUID, version int64, scope communications.DeliveryScope) (automation.ChannelReport, error) {
			report, e := f.s.QueueApprovedOccurrenceReportInTransaction(
				ctx,
				tx,
				eventID,
				campaignID,
				version,
				scope,
			)
			return automation.ChannelReport{
				Queued:      report.Queued,
				Preexisting: report.Preexisting,
			}, e
		},
	}
	auto.ReviewGuards = map[string]automation.ReviewGuard{
		"teams": f.s.CheckOccurrenceReviewInTransaction,
	}
	campaignID := f.campaign("reminder", "Approved common event reminder; no private records.")
	campaign, err := f.s.Composer.Campaign(t.Context(), f.owner.ID, f.id, campaignID)
	if err != nil {
		t.Fatal(err)
	}
	definition := automation.Definition{
		ID:                     uuid.New(),
		CampaignID:             campaignID,
		CampaignVersion:        campaign.Campaign.Campaign.Version,
		Trigger:                "scheduled",
		MaximumLatenessSeconds: 3600,
	}
	change := func(command automation.Command) automation.Page {
		t.Helper()
		page, _, e := auto.Change(t.Context(), f.owner.ID, f.id, f.revision(), command)
		if e != nil {
			t.Fatal(e)
		}
		return page
	}
	page := change(
		automation.Command{
			Action:     "save",
			Definition: &definition,
			Reason:     "Review actual Teams occurrence",
		},
	)
	change(
		automation.Command{
			Action:    "approve",
			BindingID: definition.ID,
			Version:   page.Bindings[0].Version,
			Reason:    "Approve original shared-channel source and timing",
		},
	)
	for range 2 {
		if err = auto.Maintain(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	page, err = auto.Read(t.Context(), f.owner.ID, f.id)
	if err != nil || len(page.Occurrences) != 1 {
		t.Fatal("duplicate discovery did not retain one current occurrence", err)
	}
	occurrence, err := dbgen.New(f.pool).
		AutomationOccurrence(t.Context(), dbgen.AutomationOccurrenceParams{EventID: f.id, ID: page.Occurrences[0].ID})
	if err != nil || occurrence.OperationID == nil {
		t.Fatal(err)
	}
	f.execute(*occurrence.OperationID)
	var operation uuid.UUID
	var delivery uuid.UUID
	if err = f.pool.QueryRow(t.Context(), "SELECT operation_id,id FROM teams_workflow_deliveries WHERE occurrence_id=$1", occurrence.ID).
		Scan(&operation, &delivery); err != nil {
		t.Fatal(err)
	}
	f.exec(
		"CREATE FUNCTION synthetic_teams_receipt_outage() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic durable receipt outage'; END $$",
	)
	f.exec(
		"CREATE TRIGGER synthetic_teams_receipt_outage BEFORE INSERT ON teams_workflow_receipts FOR EACH ROW EXECUTE FUNCTION synthetic_teams_receipt_outage()",
	)
	f.execute(operation)
	if f.calls.Load() != 1 {
		t.Fatal("automation did not reach local TLS recording provider exactly once")
	}
	payload := <-f.payloads
	for _, private := range []string{"@", "PRIVATE-SCORE", "CREDENTIAL-VALUE", "question_id", "SYNTHETIC_NOT_A_REAL_CHANNEL"} {
		if strings.Contains(payload, private) {
			t.Fatal("private data entered common broadcast", private)
		}
	}
	f.exec("DROP TRIGGER synthetic_teams_receipt_outage ON teams_workflow_receipts")
	restarted, err := jobs.New(
		f.pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{automation.HandlerKind: auto, HandlerKind(): f.s},
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
		if err = auto.Maintain(t.Context()); err != nil {
			t.Fatal(err)
		}
		f.execute(*occurrence.OperationID)
	}
	if f.calls.Load() != 1 {
		t.Fatal("unknown accepted broadcast replayed after restart")
	}
	var attempts, receipts int
	if err = f.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM teams_workflow_attempts WHERE delivery_id=$1),(SELECT count(*) FROM teams_workflow_receipts r JOIN teams_workflow_attempts a ON a.event_id=r.event_id AND a.id=r.attempt_id WHERE a.delivery_id=$1)`, delivery).
		Scan(&attempts, &receipts); err != nil ||
		attempts != 1 ||
		receipts > 1 {
		t.Fatal("restart invented repeated provider evidence", attempts, receipts, err)
	}
	history := deliveryhistory.Service{Pool: f.pool}
	detail, err := history.Detail(t.Context(), f.owner.ID, f.id, delivery, 0, nil)
	if err != nil || detail.Item.State != "ambiguous" || !detail.Item.Attempted {
		t.Fatal("lost acceptance receipt became retryable or never attempted", err)
	}
	raw, _ := json.Marshal(detail)
	if strings.Contains(string(raw), "Approved common event reminder") ||
		strings.Contains(string(raw), "sig=") {
		t.Fatal("broadcast payload or installation endpoint leaked into safe history")
	}
}
