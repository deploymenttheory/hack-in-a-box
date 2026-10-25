package teamreports

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestTeamReportSchemaPreservesPopulatedParentAndEmptyRollback(t *testing.T) {
	pool := testdb.New(t, false)
	if err := database.Migrate(t.Context(), pool, 53); err != nil {
		t.Fatal(err)
	}
	actor := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, actor.ID)
	reference := testdb.ApprovedRubric(t, pool, actor.ID, eventID)
	if err := database.Migrate(t.Context(), pool, 54); err != nil {
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
	if _, err = provider.DownTo(t.Context(), 53); err != nil {
		t.Fatal("empty report rollback", err)
	}
	version, err := provider.GetDBVersion(t.Context())
	if err != nil || version != 53 {
		t.Fatal("empty rollback changed wrong schema", version, err)
	}
	if err = database.Migrate(t.Context(), pool, 0); err != nil {
		t.Fatal("restore actual current schema", err)
	}
	if err = database.CheckSchema(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = pool.QueryRow(t.Context(), "SELECT count(*) FROM event_rubrics WHERE event_id=$1 AND id=$2", eventID, reference.ID).
		Scan(&count); err != nil ||
		count != 1 {
		t.Fatal("report upgrade/rollback changed parent rubric", count, err)
	}
}
