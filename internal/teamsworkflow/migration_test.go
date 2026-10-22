package teamsworkflow

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestTeamsSchemaUpgradeAndGuardedRollback(t *testing.T) {
	pool := testdb.New(t, false)
	ctx := t.Context()
	if err := database.Migrate(ctx, pool, 27); err != nil {
		t.Fatal(err)
	}
	owner := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	if err := database.Migrate(ctx, pool, 28); err != nil {
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
	if _, err = provider.DownTo(ctx, 27); err != nil {
		t.Fatal("empty Teams schema should roll back", err)
	}
	if err = database.Migrate(ctx, pool, 28); err != nil {
		t.Fatal(err)
	}
	// Never silently discard even a revoked/unused connection during rollback.
	_, err = pool.Exec(
		ctx,
		`INSERT INTO teams_workflow_connections(id,event_id,version,label,owner_label,co_owner_label,key_id,ciphertext,active,updated_at) VALUES($1,$2,1,'Synthetic','Owner','Co-owner','',decode('','hex'),false,$3)`,
		uuid.New(),
		eventID,
		time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(ctx, 27); err == nil {
		t.Fatal("rollback discarded connection/history")
	}
	var count int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM teams_workflow_connections WHERE event_id=$1", eventID).
		Scan(&count); err != nil ||
		count != 1 {
		t.Fatal("failed rollback did not preserve connection", err)
	}
	if err = database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
}
