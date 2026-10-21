package database_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestMigrationUpgradeConcurrencyAndTransactions(t *testing.T) {
	pool := testdb.New(t, false)
	ctx := t.Context()
	if err := database.CheckSchema(ctx, pool); !errors.Is(err, database.ErrSchema) {
		t.Fatalf("fresh schema accepted: %v", err)
	}
	if err := database.Migrate(ctx, pool, 1); err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO events(id,name) VALUES ($1,'Synthetic event')",
		id,
	); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			if err := database.Migrate(ctx, pool, 0); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if err := database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
	event, err := dbgen.New(pool).Event(ctx, id)
	if err != nil || event.Name != "Synthetic event" {
		t.Fatalf("upgrade lost fixture: %+v %v", event, err)
	}
	simulated := errors.New("simulated failure")
	err = database.Within(ctx, pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.UpdateEventName(
			ctx,
			dbgen.UpdateEventNameParams{ID: id, Name: "Rolled back", Revision: 1},
		); err != nil {
			return err
		}
		if _, err := tx.Exec(
			ctx,
			"INSERT INTO operations(event_id, operation_key, kind, target_id, due_at) VALUES ($1,'rollback','test',$1,now())",
			id,
		); err != nil {
			return err
		}
		return simulated
	})
	if !errors.Is(err, simulated) {
		t.Fatal(err)
	}
	event, err = dbgen.New(pool).Event(ctx, id)
	if err != nil || event.Revision != 1 || event.Name != "Synthetic event" {
		t.Fatalf("rollback failed: %+v %v", event, err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM operations").
		Scan(&count); err != nil ||
		count != 0 {
		t.Fatalf("outbox escaped rollback: %d %v", count, err)
	}
	_, err = dbgen.New(pool).
		UpdateEventName(ctx, dbgen.UpdateEventNameParams{ID: id, Name: "Stale", Revision: 9})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("stale revision accepted: %v", err)
	}
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO goose_db_version(version_id,is_applied) VALUES (999,true)",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.CheckSchema(ctx, pool); !errors.Is(err, database.ErrSchema) {
		t.Fatalf("future schema accepted: %v", err)
	}
}
