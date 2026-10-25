package judging

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestJudgeHistoryPreventsPopulatedRollback(t *testing.T) {
	f := setup(t)
	if _, err := f.card(f.first.ID, 0, "save", f.scores(6)); err != nil {
		t.Fatal(err)
	}
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
	if _, err = provider.DownTo(t.Context(), 50); err == nil {
		t.Fatal("rollback erased authored judge history")
	}
	version, err := provider.GetDBVersion(t.Context())
	if err != nil || version != 51 {
		t.Fatal("rollback changed judging schema", version, err)
	}
	var cards int
	if err = f.pool.QueryRow(t.Context(), "SELECT count(*) FROM judge_cards WHERE event_id=$1", f.event).
		Scan(&cards); err != nil ||
		cards != 1 {
		t.Fatal("rollback changed card", cards, err)
	}
	if _, err = f.pool.Exec(t.Context(), "TRUNCATE judge_history"); err == nil {
		t.Fatal("history mutation accepted")
	}
}
