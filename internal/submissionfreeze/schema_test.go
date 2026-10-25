package submissionfreeze

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestMigration50PreservesFreezeFactsAndOriginalCommands(t *testing.T) {
	pool := testdb.New(t, false)
	if err := database.Migrate(t.Context(), pool, 49); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(t.Context(), pool, 0); err != nil {
		t.Fatal(err)
	}
	if err := database.CheckSchema(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()
	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		os.DirFS("../database/migrations"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(t.Context(), 49); err != nil {
		t.Fatal("empty 50 rollback", err)
	}
	if err = database.Migrate(t.Context(), pool, 0); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, pool)
	c := Command{RequestID: uuid.New(), Action: "configure", Branch: "main", ExpectedRevision: 0}
	ack, err := f.service.Change(t.Context(), f.member, f.event, f.team, c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(t.Context(), 49); err == nil {
		t.Fatal("rollback erased branch or original command")
	}
	original, err := f.service.OriginalReceipt(
		t.Context(),
		f.member,
		f.event,
		f.team,
		ack.RequestID,
	)
	if err != nil || original.Receipt == nil || original.Receipt.Fingerprint != ack.Fingerprint {
		t.Fatal(original, err)
	}
	for _, statement := range []string{"UPDATE submission_freeze_commands SET fingerprint='changed'", "DELETE FROM submission_freeze_commands", "TRUNCATE submission_freeze_commands"} {
		if _, err = pool.Exec(t.Context(), statement); err == nil {
			t.Fatal("mutable original command", statement)
		}
	}
	f.submit("repository")
	f.now = f.cfg.SubmissionCutoff
	f.capture()
	p := f.read(f.owner)
	decision := f.decision(p, "reconcile")
	if _, err = f.service.Change(t.Context(), f.owner, f.event, f.team, decision); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"submission_freeze_sources", "submission_freeze_observations", "submission_freeze_decisions"} {
		if _, err = pool.Exec(t.Context(), "DELETE FROM "+table); err == nil {
			t.Fatal("mutable freeze evidence", table)
		}
		if _, err = pool.Exec(t.Context(), "TRUNCATE "+table+" CASCADE"); err == nil {
			t.Fatal("truncatable evidence", table)
		}
	}
	if err = database.CheckSchema(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
}
