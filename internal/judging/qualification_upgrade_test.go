package judging

import (
	"maps"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func native51Snapshot(t *testing.T, pool *pgxpool.Pool, tables []string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, table := range tables {
		var raw string
		query := "SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text),'[]'::jsonb)::text FROM " + pgx.Identifier{
			table,
		}.Sanitize() + " t"
		if err := pool.QueryRow(t.Context(), query).Scan(&raw); err != nil {
			t.Fatal(table, err)
		}
		out[table] = raw
	}
	return out
}

func TestQualification52PreservesCompleteActual51DataAndEmptyRollback(t *testing.T) {
	f := setup(t)
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
	if _, err = provider.DownTo(t.Context(), 51); err != nil {
		t.Fatal("empty52 rollback before actual51 fixture", err)
	}
	if _, err = f.card(f.first.ID, 0, "save", f.scores(7)); err != nil {
		t.Fatal(err)
	}
	mentor := testdb.Person(t, f.pool)
	f.mentor(mentor.ID)
	if _, err = f.s.ChangeSummary(
		t.Context(),
		mentor.ID,
		f.event,
		f.team,
		0,
		SummaryCommand{
			RequestID:          uuid.New(),
			Action:             "save",
			SubmissionRevision: 1,
			Facts: []SummaryFact{
				{
					DeliverableID: f.deliverable,
					Fact:          "Explicit native51 factual-summary preservation canary",
				},
			},
			Reason: "Actual authored native51 summary",
		},
	); err != nil {
		t.Fatal(err)
	}
	rows, err := f.pool.Query(
		t.Context(),
		"SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename<>'goose_db_version' ORDER BY tablename",
	)
	if err != nil {
		t.Fatal(err)
	}
	tables := []string{}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	before := native51Snapshot(t, f.pool, tables)
	for _, table := range []string{"judge_cards", "judge_commands", "judge_history", "judge_factual_summaries"} {
		if before[table] == "[]" || before[table] == "" {
			t.Fatal("critical actual51 owner not populated", table)
		}
	}
	if err = database.Migrate(t.Context(), f.pool, 52); err != nil {
		t.Fatal(err)
	}
	if after := native51Snapshot(t, f.pool, tables); !maps.Equal(before, after) {
		t.Fatal("51→52 changed predecessor data")
	}
	if _, err = provider.DownTo(t.Context(), 51); err != nil {
		t.Fatal("empty52 rollback rejected", err)
	}
	if after := native51Snapshot(t, f.pool, tables); !maps.Equal(before, after) {
		t.Fatal("empty52 rollback changed native51 owner/history")
	}
	if err = database.Migrate(t.Context(), f.pool, 52); err != nil {
		t.Fatal(err)
	}
	if after := native51Snapshot(t, f.pool, tables); !maps.Equal(before, after) {
		t.Fatal("reapplied52 changed native51 owner/history")
	}
}
