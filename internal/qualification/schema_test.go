package qualification

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestQualificationHistoryRejectsTruncateIncludingCascade(t *testing.T) {
	pool := testdb.New(t, true)
	for _, statement := range []string{"TRUNCATE mentor_qualification_commands", "TRUNCATE mentor_qualification_sources", "TRUNCATE mentor_qualification_rounds CASCADE"} {
		if _, err := pool.Exec(t.Context(), statement); err == nil {
			t.Fatalf("immutable qualification history accepted %s", statement)
		}
	}
}

func TestQualification52PopulatedRollbackAndMutationCannotEraseOriginalDecisions(t *testing.T) {
	f, s, run := receiptFixture(t)
	page, err := s.Change(t.Context(), f.owner.ID, f.event, run, f.revision(), panelCommand(f))
	if err != nil {
		t.Fatal(err)
	}
	var before string
	snapshot := func() string {
		t.Helper()
		var raw string
		if err := f.pool.QueryRow(t.Context(), "SELECT jsonb_build_object('rounds',(SELECT jsonb_agg(to_jsonb(r) ORDER BY version) FROM mentor_qualification_rounds r WHERE event_id=$1),'commands',(SELECT jsonb_agg(to_jsonb(c) ORDER BY request_id) FROM mentor_qualification_commands c WHERE event_id=$1))::text", f.event).
			Scan(&raw); err != nil {
			t.Fatal(err)
		}
		return raw
	}
	before = snapshot()
	db := stdlib.OpenDBFromPool(f.pool)
	defer func() { _ = db.Close() }()
	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		os.DirFS("../database/migrations"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(t.Context(), 51); err == nil {
		t.Fatal("populated52 rollback erased decisions")
	}
	version, err := provider.GetDBVersion(t.Context())
	if err != nil || version != 52 {
		t.Fatal("failed rollback changed52", version, err)
	}
	for _, statement := range []string{"UPDATE mentor_qualification_rounds SET reason='erase'", "DELETE FROM mentor_qualification_commands", "TRUNCATE mentor_qualification_commands", "TRUNCATE mentor_qualification_rounds CASCADE"} {
		if _, err = f.pool.Exec(t.Context(), statement); err == nil {
			t.Fatal("immutable original history accepted mutation", statement)
		}
	}
	if snapshot() != before {
		t.Fatal("guarded rollback/mutation changed original decision data")
	}
	original, err := s.OriginalReceipt(
		t.Context(),
		f.owner.ID,
		f.event,
		run,
		page.Receipt.RequestID,
	)
	if err != nil || original.Receipt == nil || *original.Receipt != *page.Receipt {
		t.Fatal("guarded rollback lost exact original ACK", original, err)
	}
}
