package loadprofile

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

// This test seeds query-component inputs directly. It establishes actual SQL
// compatibility, scoped reads and missing-fact accounting only. These seeded
// rows are never native owner, worker-start or release-load evidence.
func TestNativeDatabaseQueriesRetainPendingFactsAndScope(t *testing.T) {
	pool := testdb.New(t, true)
	owner := testdb.Person(t, pool)
	event := testdb.Event(t, pool, owner.ID)
	outside := testdb.Event(t, pool, owner.ID)
	ctx := t.Context()
	if _, err := pool.Exec(
		ctx,
		`UPDATE events SET configuration='{"capacity":180}'::jsonb WHERE id=$1`,
		event,
	); err != nil {
		t.Fatal(err)
	}
	native, err := NewNativeDatabase(pool, []uuid.UUID{event})
	if err != nil {
		t.Fatal(err)
	}
	inactive := testdb.Person(t, pool)
	if _, err := pool.Exec(
		ctx,
		`UPDATE people SET active=false WHERE id=$1`,
		inactive.ID,
	); err != nil {
		t.Fatal(err)
	}
	// Deliberately seeded STORE QUERY inputs, not native authenticated jars.
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(token,data,expiry) VALUES
 ('synthetic-query-live','synthetic-query-data',now()+interval '1 hour'),
 ('synthetic-query-expired','synthetic-query-data',now()-interval '1 hour')`); err != nil {
		t.Fatal(err)
	}
	installation, err := native.Installation(ctx)
	if err != nil || installation.Events != 2 || installation.People != 2 ||
		installation.ActivePeople != 1 ||
		installation.UnexpiredSessions != 1 {
		t.Fatal(
			"global installation census hid outside events, inactive people or session expiry",
			installation,
			err,
		)
	}
	admission, err := native.Admission(ctx, []uuid.UUID{event})
	if err != nil {
		t.Fatal(err)
	}
	if len(admission) != 1 || admission[0].ConfiguredCapacity != 180 ||
		admission[0].Staff != 1 || admission[0].People != 1 || admission[0].Confirmed != 0 {
		t.Fatal("native census did not distinguish staff from competitors", admission)
	}
	if _, err := native.Admission(ctx, []uuid.UUID{outside}); err == nil {
		t.Fatal("oracle read an event outside its fixture scope")
	}
	intent, source := uuid.New(), uuid.New()
	for id, kind := range map[uuid.UUID]string{intent: "communication.dispatch", source: "teams.published"} {
		if _, err := pool.Exec(
			ctx,
			`INSERT INTO operations(id,event_id,operation_key,kind,target_id,due_at,approved) VALUES($1,$2,$3,$4,$1,now()-interval '1 minute',true)`,
			id,
			event,
			id.String(),
			kind,
		); err != nil {
			t.Fatal(err)
		}
	}
	queue, err := native.Queue(ctx, []uuid.UUID{event})
	if err != nil {
		t.Fatal(err)
	}
	if queue.Pending != 2 || queue.OverdueApproved != 2 || queue.MaximumDueLagSeconds < 60 ||
		queue.PendingByKind["teams.published"] != 1 || queue.PendingByKind["communication.dispatch"] != 1 {
		t.Fatal("raw pending source facts were hidden or equated to started work", queue)
	}
	starts, err := native.JobStarts(ctx, []uuid.UUID{intent})
	if err != nil {
		t.Fatal(err)
	}
	if len(starts) != 1 || starts[0].FirstStarted != nil || starts[0].Attempts != 0 {
		t.Fatal("unstarted approved intent disappeared", starts)
	}
	if _, err := native.JobStarts(ctx, []uuid.UUID{uuid.New()}); err == nil {
		t.Fatal("missing expected intent silently dropped")
	}
	// Reading an actual attempt timestamp is tested with deliberately seeded
	// input; only the separate worker owner can prove a genuine guarded start.
	started := time.Now().UTC().Truncate(time.Microsecond)
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO delivery_attempts(operation_id,lease_token,outcome,started_at) VALUES($1,$2,'running',$3)`,
		intent,
		uuid.New(),
		started,
	); err != nil {
		t.Fatal(err)
	}
	starts, err = native.JobStarts(ctx, []uuid.UUID{intent})
	if err != nil || len(starts) != 1 || starts[0].FirstStarted == nil ||
		!starts[0].FirstStarted.Equal(started) ||
		starts[0].Attempts != 1 {
		t.Fatal("original timestamp/count changed in query projection", starts, err)
	}
	for _, action := range []string{"create", "claim", "resolve"} {
		spec := WriteSpec{
			Kind:        HelpEffect,
			Action:      action,
			Event:       event,
			Actor:       owner.ID,
			Command:     uuid.New(),
			Resource:    uuid.New(),
			Version:     1,
			Fingerprint: "unused-missing-candidate",
		}
		facts, commits, err := native.Effects(ctx, map[uuid.UUID]WriteSpec{uuid.New(): spec})
		if err != nil || len(facts) != 1 || len(commits) != 0 {
			t.Fatal("absent original help command not retained", action, facts, commits, err)
		}
		for _, fact := range facts {
			if fact.Present || fact.Effects != 0 {
				t.Fatal("absent command became accepted native effect", fact)
			}
		}
	}
}
