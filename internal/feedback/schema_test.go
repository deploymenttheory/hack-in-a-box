package feedback

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestFeedback36PreservesPopulated35AndGuardsRetainedSupportRollback(t *testing.T) {
	const prior, protected int64 = 35, 36
	pool := testdb.New(t, false)
	ctx := t.Context()
	if err := database.Migrate(ctx, pool, prior); err != nil {
		t.Fatal(err)
	}
	f := setupPool(t, pool)
	f.exec(
		"INSERT INTO communication_automation_settings(event_id,paused,actor_id,reason) VALUES($1,true,$2,'Synthetic deliberate pause retained')",
		f.event,
		f.owner.ID,
	)
	native := help.Service{Pool: pool, Clock: func() time.Time { return f.now }}
	requestID := uuid.New()
	request, err := native.Create(
		ctx,
		f.member.ID,
		f.event,
		requestID,
		help.Input{
			TeamID:      f.team,
			Category:    "setup",
			Description: "PRIVATE PRIOR35 REQUEST",
			Location:    "Synthetic meeting point",
			CommandID:   uuid.New(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(
		"INSERT INTO inbox_notifications(id,event_id,recipient_id,source_kind,source_id,source_version,subject,body,format) VALUES($1,$2,$3,'help',$4,$5,'Native retained help update','','text')",
		uuid.New(),
		f.event,
		f.member.ID,
		requestID,
		request.Request.Revision,
	)
	const snapshot = `SELECT jsonb_build_object('event',(SELECT to_jsonb(e) FROM events e WHERE id=$1),'teams',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM hack_teams t WHERE event_id=$1),'membership',(SELECT jsonb_agg(to_jsonb(m) ORDER BY person_id,role) FROM event_memberships m WHERE event_id=$1),'pause',(SELECT to_jsonb(a) FROM communication_automation_settings a WHERE event_id=$1),'help',(SELECT jsonb_agg(to_jsonb(h) ORDER BY id) FROM help_requests h WHERE event_id=$1),'inbox',(SELECT jsonb_agg(to_jsonb(n) ORDER BY id) FROM inbox_notifications n WHERE event_id=$1),'audit',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM audit_events a WHERE event_id=$1))::text`
	var before, after string
	if err = pool.QueryRow(ctx, snapshot, f.event).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err = database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	if err = database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, snapshot, f.event).Scan(&after); err != nil || before != after {
		t.Fatal("35→36 changed retained event/help/inbox/automation/audit state", err)
	}
	if _, err = native.Read(ctx, f.member.ID, f.event, requestID); err != nil {
		t.Fatal("prior native help policy changed", err)
	}
	id, p := f.create("setup")
	f.publish(id, p)
	_, action := f.task(id, f.member.ID)
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()
	migrator, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		os.DirFS("../database/migrations"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = migrator.DownTo(ctx, prior); err == nil {
		t.Fatal("rollback removed reviewed support history")
	}
	version, err := migrator.GetDBVersion(ctx)
	if err != nil || version != protected {
		t.Fatal("failed rollback lost explicit36 preservation boundary", version, err)
	}
	if _, err = f.s.Actions(ctx, f.member.ID, f.event, id, uuid.Nil, 50); err != nil {
		t.Fatal("failed rollback lost team support data", err)
	}
	if _, err = pool.Exec(
		ctx,
		"UPDATE feedback_action_versions SET document='{}'::jsonb WHERE event_id=$1 AND id=$2",
		f.event,
		action.Action.ID,
	); err == nil {
		t.Fatal("immutable action history was mutable")
	}
	if err = database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	if err = database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
}

func TestFeedback36EmptyRollbackAndSettingsOnlyHistoryGuard(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()
	migrator, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		os.DirFS("../database/migrations"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = migrator.DownTo(ctx, 35); err != nil {
		t.Fatal("empty36 rollback unsupported", err)
	}
	if err = database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	owner := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	s := Service{Pool: pool}
	if _, err = s.Configure(
		ctx,
		owner.ID,
		eventID,
		1,
		Settings{SetupSeconds: 60, LogisticsSeconds: 120, OutputSeconds: 180},
	); err != nil {
		t.Fatal(err)
	}
	if _, err = migrator.DownTo(ctx, 35); err == nil {
		t.Fatal("settings-only history was discarded")
	}
	version, err := migrator.GetDBVersion(ctx)
	if err != nil || version != 36 {
		t.Fatal("settings-only rollback boundary changed", version, err)
	}
	if err = database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	if err = database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
}
