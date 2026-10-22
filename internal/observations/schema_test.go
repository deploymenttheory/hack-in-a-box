package observations

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

// This fixture exercises the concrete preceding inbox schema30. A stacked PR
// still needs restacking onto merged main and fresh CI before it can merge.
func TestObservationMigrationPreservesExistingRecordsAndProtectsRollback(t *testing.T) {
	const predecessor int64 = 30
	ctx := t.Context()
	pool := testdb.New(t, false)
	if err := database.Migrate(ctx, pool, predecessor); err != nil {
		t.Fatal(err)
	}
	owner, mentor := testdb.Person(t, pool), testdb.Person(t, pool)
	id, team := testdb.Event(t, pool, owner.ID), uuid.New()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO hack_teams(event_id,id,name,state) VALUES($1,$2,'Synthetic upgrade team','published')", []any{id, team}},
		{"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mentor')", []any{id, mentor.ID}},
		{"INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at) VALUES($1,$2,$3,'primary','published','2027-06-20T09:00Z','2027-06-20T17:00Z')", []any{id, team, mentor.ID}},
	} {
		if _, err := pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	const snapshotSQL = `SELECT jsonb_build_object(
	 'event',(SELECT to_jsonb(e) FROM events e WHERE id=$1),
	 'teams',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM hack_teams t WHERE event_id=$1),
	 'assignments',(SELECT jsonb_agg(to_jsonb(a) ORDER BY person_id) FROM mentor_assignments a WHERE event_id=$1),
	 'memberships',(SELECT jsonb_agg(to_jsonb(m) ORDER BY person_id,role) FROM event_memberships m WHERE event_id=$1),
	 'audit',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM audit_events a WHERE event_id=$1)
	)::text`
	var before, after string
	if err := pool.QueryRow(ctx, snapshotSQL, id).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	if err := database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, snapshotSQL, id).Scan(&after); err != nil || before != after {
		t.Fatalf("observation migration changed existing event/team/assignment/audit data: %v", err)
	}
	now := time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	note := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO mentor_observation_versions(event_id,team_id,id,version,author_id,observed_at,changed_at,category,evidence,outcome)
	 VALUES($1,$2,$3,1,$4,$5,$5,'technical','Synthetic retained evidence','Synthetic outcome')`, id, team, note, mentor.ID, now); err != nil {
		t.Fatal(err)
	}
	other := testdb.Event(t, pool, owner.ID)
	if _, err := pool.Exec(ctx, `INSERT INTO mentor_team_status_versions(event_id,team_id,version,actor_id,state,reason,next_action,changed_at)
	 VALUES($1,$2,1,$3,'blocked','Synthetic cross-event status','',$4)`, other, team, mentor.ID, now); err == nil {
		t.Fatal("cross-event team reference accepted")
	}
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer func() { _ = sqlDB.Close() }()
	migrator, err := goose.NewProvider(goose.DialectPostgres, sqlDB, os.DirFS("../database/migrations"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = migrator.DownTo(ctx, predecessor); err == nil {
		t.Fatal("rollback discarded populated private history")
	}
	version, err := migrator.GetDBVersion(ctx)
	if err != nil || version != 31 {
		t.Fatalf("failed rollback changed the current schema: version=%d err=%v", version, err)
	}
	var evidence string
	if err = pool.QueryRow(ctx, "SELECT evidence FROM mentor_observation_versions WHERE event_id=$1 AND team_id=$2 AND id=$3", id, team, note).Scan(&evidence); err != nil || evidence != "Synthetic retained evidence" {
		t.Fatalf("failed rollback lost retained history: %v", err)
	}
	if err = database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	if err = database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
}
