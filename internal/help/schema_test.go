package help

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

func TestHelpUpgradeFromObservationSchema31PreservesPrivateResources(t *testing.T) {
	ctx := t.Context()
	pool := testdb.New(t, false)
	if err := database.Migrate(ctx, pool, 31); err != nil {
		t.Fatal(err)
	}
	owner, mentor := testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	teamID, noteID := uuid.New(), uuid.New()
	now := time.Date(2027, 6, 20, 10, 0, 0, 0, time.UTC)
	statements := []struct {
		sql  string
		args []any
	}{
		{
			"INSERT INTO hack_teams(event_id,id,name,state) VALUES($1,$2,'Synthetic preserved team','published')",
			[]any{eventID, teamID},
		},
		{
			"INSERT INTO event_memberships(event_id,person_id,role,active) VALUES($1,$2,'mentor',true)",
			[]any{eventID, mentor.ID},
		},
		{
			"INSERT INTO mentor_observation_versions(event_id,team_id,id,version,author_id,observed_at,changed_at,category,evidence,outcome) VALUES($1,$2,$3,1,$4,$5,$5,'technical','PRIVATE-OBSERVATION-CANARY','Synthetic retained outcome')",
			[]any{eventID, teamID, noteID, mentor.ID, now},
		},
		{
			"INSERT INTO mentor_team_status_versions(event_id,team_id,version,actor_id,state,reason,next_action,changed_at) VALUES($1,$2,1,$3,'blocked','PRIVATE-STATUS-CANARY','Synthetic action',$4)",
			[]any{eventID, teamID, mentor.ID, now},
		},
		{
			"INSERT INTO inbox_notifications(id,event_id,recipient_id,source_kind,source_id,source_version,subject,body,format) VALUES($1,$2,$3,'test_self',$4,1,'Synthetic retained message','PRIVATE-INBOX-CANARY','text')",
			[]any{uuid.New(), eventID, owner.ID, uuid.New()},
		},
		{
			"INSERT INTO person_email_contacts(person_id,mailbox,provenance) VALUES($1,'synthetic-private@example.invalid','oidc_verified')",
			[]any{owner.ID},
		},
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	const snapshot = `SELECT jsonb_build_object(
 'event',(SELECT to_jsonb(e) FROM events e WHERE id=$1),
 'team',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM hack_teams t WHERE event_id=$1),
 'memberships',(SELECT jsonb_agg(to_jsonb(m) ORDER BY person_id,role) FROM event_memberships m WHERE event_id=$1),
 'observations',(SELECT jsonb_agg(to_jsonb(o) ORDER BY id,version) FROM mentor_observation_versions o WHERE event_id=$1),
 'status',(SELECT jsonb_agg(to_jsonb(s) ORDER BY team_id,version) FROM mentor_team_status_versions s WHERE event_id=$1),
 'inbox',(SELECT jsonb_agg(to_jsonb(n)-'scope_approval' ORDER BY id) FROM inbox_notifications n WHERE event_id=$1),
 'mailbox',(SELECT to_jsonb(c) FROM person_email_contacts c WHERE person_id=$2),
 'audit',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM audit_events a WHERE event_id=$1)
 )::text`
	var before, after string
	if err := pool.QueryRow(ctx, snapshot, eventID, owner.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	if err := database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, snapshot, eventID, owner.ID).
		Scan(&after); err != nil ||
		before != after {
		t.Fatal("help upgrade changed private resources, authority or event state", err)
	}
	queue, err := (Service{Pool: pool}).List(ctx, owner.ID, eventID)
	if err != nil || len(queue.Items) != 0 || queue.Waiting != 0 || queue.Claimed != 0 {
		t.Fatalf("upgrade invented help state: %+v %v", queue, err)
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
	if _, err = provider.DownTo(ctx, 31); err != nil {
		t.Fatal("empty help rollback", err)
	}
	if err = database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, snapshot, eventID, owner.ID).
		Scan(&after); err != nil ||
		before != after {
		t.Fatal("empty help rollback changed earlier private history", err)
	}
	// Even settings-only state needs deliberate preservation before rollback.
	if _, err = pool.Exec(
		ctx,
		"INSERT INTO help_settings(event_id,revision,setup_threshold_seconds,logistics_threshold_seconds,output_threshold_seconds) VALUES($1,2,60,120,180)",
		eventID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(ctx, 31); err == nil {
		t.Fatal("rollback discarded configured help thresholds")
	}
	if err = database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	if err = database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
}

func TestHelpPopulatedRollbackPreservesPrivateRequestAndHistory(t *testing.T) {
	f := newFixture(t)
	created := f.create("setup")
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
	if _, err = provider.DownTo(t.Context(), 31); err == nil {
		t.Fatal("rollback discarded request and transition history")
	}
	current, err := f.s.Read(t.Context(), f.member.ID, f.eventID, created.Request.ID)
	if err != nil || current.Request.Revision != created.Request.Revision ||
		len(current.History) != 1 {
		t.Fatalf("rollback changed current request: %+v %v", current, err)
	}
	if err = database.Migrate(t.Context(), f.pool, 0); err != nil {
		t.Fatal(err)
	}
	if err = database.CheckSchema(t.Context(), f.pool); err != nil {
		t.Fatal(err)
	}
}
