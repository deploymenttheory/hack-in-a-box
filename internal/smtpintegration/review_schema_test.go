package smtpintegration

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/deliveryhistory"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func deliverySchemaProvider(t *testing.T, pool *pgxpool.Pool) *goose.Provider {
	t.Helper()
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })
	p, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS("../database/migrations"))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func retainedMailEvidence(t *testing.T, pool *pgxpool.Pool) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, table := range []string{"people", "events", "event_memberships", "person_email_contacts", "communication_campaign_versions", "operations", "delivery_attempts", "smtp_campaign_messages", "smtp_submission_attempts", "smtp_submission_receipts", "email_calendar_sequences", "audit_events"} {
		var value string
		if err := pool.QueryRow(t.Context(), `SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text),'[]'::jsonb)::text FROM `+pgx.Identifier{table}.Sanitize()+` t`).
			Scan(&value); err != nil {
			t.Fatal(table, err)
		}
		out[table] = value
	}
	return out
}

func TestDelivery38NormalUpgradePreservesReal37SMTPThenGuardsUsedResendEvidence(t *testing.T) {
	ctx := t.Context()
	pool := testdb.New(t, false)
	if err := database.Migrate(ctx, pool, 37); err != nil {
		t.Fatal(err)
	}
	// Native campaign approval, recipient selection and actual recording provider
	// execution occur on the real predecessor before review tables exist.
	f := setupPool(t, pool)
	f.approvePastProduction()
	d := f.queueProduction()
	if err := f.worker.Execute(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	if f.transport.count() != 1 {
		t.Fatal("predecessor did not record actual provider acceptance")
	}
	before := retainedMailEvidence(t, pool)
	if err := database.Migrate(ctx, pool, 38); err != nil {
		t.Fatal(err)
	}
	after := retainedMailEvidence(t, pool)
	for table, value := range before {
		if after[table] != value {
			t.Fatal("37→38 changed retained SMTP evidence", table)
		}
	}
	q := dbgen.New(pool)
	op, err := q.Operation(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	history := deliveryhistory.Service{
		Pool:      pool,
		Jobs:      f.worker,
		Providers: map[string]deliveryhistory.RecoveryProvider{"smtp": f.service},
	}
	command := deliveryhistory.Command{
		RequestID: uuid.New(),
		Action:    "resend_acknowledged",
		Reason:    "Explicit synthetic possible duplicate after normal upgrade",
	}
	if _, err = history.Change(ctx, f.owner.ID, f.event, d.ID, op.Revision, command); err != nil {
		t.Fatal(err)
	}
	if err = f.worker.Execute(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	if f.transport.count() != 2 {
		t.Fatal("normal upgrade did not enable exactly one reviewed provider attempt")
	}
	migrator := deliverySchemaProvider(t, pool)
	if _, err = migrator.DownTo(ctx, 37); err == nil {
		t.Fatal("rollback discarded private review and used resend authority")
	}
	version, err := migrator.GetDBVersion(ctx)
	if err != nil || version != 38 {
		t.Fatal("rollback crossed protected38 boundary", version, err)
	}
	var reviews, uses, receipts int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM communication_delivery_reviews WHERE operation_id=$1),(SELECT count(*) FROM communication_delivery_resend_uses WHERE review_id=$2),(SELECT count(*) FROM smtp_submission_receipts r JOIN smtp_submission_attempts a ON a.id=r.attempt_id WHERE a.operation_id=$1)`, d.ID, command.RequestID).
		Scan(&reviews, &uses, &receipts); err != nil || reviews != 1 ||
		uses != 1 ||
		receipts != 2 {
		t.Fatal("protected rollback lost immutable evidence", reviews, uses, receipts, err)
	}
	for _, table := range []string{"communication_delivery_reviews", "communication_delivery_resend_uses"} {
		for _, mutation := range []string{"DELETE FROM ", "TRUNCATE "} {
			if _, err = pool.Exec(ctx, mutation+pgx.Identifier{table}.Sanitize()); err == nil {
				t.Fatal("append-only review evidence was mutable", table)
			}
		}
	}
	if _, err = history.Detail(ctx, f.owner.ID, f.event, d.ID, 0, nil); err != nil {
		t.Fatal("history unreadable after refused rollback", err)
	}
	// Later empty migrations may have rolled back before protected38 refused.
	if err = database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	if err = database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
}

func TestDelivery38EmptyRollbackRestoresNormalLatest(t *testing.T) {
	pool := testdb.New(t, true)
	if _, err := deliverySchemaProvider(t, pool).DownTo(t.Context(), 37); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(t.Context(), pool, 0); err != nil {
		t.Fatal(err)
	}
	if err := database.CheckSchema(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
}
