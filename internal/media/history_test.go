package media_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
	"github.com/google/uuid"
)

func TestMediaHistoryCurrentOwnersMetadataPagingArchiveConsentAndNoEffects(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	f.consent("yes")
	item := f.release(f.upload([]uuid.UUID{f.subject}))
	stranger := testdb.Person(t, f.pool).ID
	f.exec(`INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'photographer')`, f.event, stranger)
	for _, actor := range []uuid.UUID{f.subject, stranger} {
		if _, err := f.s.History(ctx, actor, f.event, item.ID, 0, 2); !errors.Is(err, access.ErrForbidden) {
			t.Fatalf("nonowner history permitted: %v", err)
		}
	}
	if _, err := f.s.History(ctx, f.owner, f.second, item.ID, 0, 2); !errors.Is(err, access.ErrNotFound) {
		t.Fatalf("wrong event: %v", err)
	}
	snapshot := func() string {
		t.Helper()
		var out string
		if err := f.pool.QueryRow(ctx, `SELECT jsonb_build_object('revision',e.revision,'versions',(SELECT count(*) FROM media_versions),'commands',(SELECT count(*) FROM media_commands),'audit',(SELECT count(*) FROM audit_events),'files',(SELECT count(*) FROM files))::text FROM events e WHERE e.id=$1`, f.event).Scan(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	before := snapshot()
	for _, actor := range []uuid.UUID{f.owner, f.photographer} {
		first, err := f.s.History(ctx, actor, f.event, item.ID, 0, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(first.Items) != 2 || first.Items[0].Version != 3 || first.Items[1].Version != 2 || first.NextBeforeVersion == nil || *first.NextBeforeVersion != 2 {
			t.Fatalf("first page: %+v", first)
		}
		second, err := f.s.History(ctx, actor, f.event, item.ID, *first.NextBeforeVersion, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(second.Items) != 1 || second.Items[0].Version != 1 || second.Items[0].Action != "upload" || second.NextBeforeVersion != nil {
			t.Fatalf("second page: %+v", second)
		}
		raw, _ := json.Marshal(first.Items[0])
		var keys map[string]json.RawMessage
		if err = json.Unmarshal(raw, &keys); err != nil {
			t.Fatal(err)
		}
		if len(keys) != 3 || keys["version"] == nil || keys["action"] == nil || keys["changed_at"] == nil {
			t.Fatalf("history leaked fields: %s", raw)
		}
	}
	if after := snapshot(); after != before {
		t.Fatalf("history mutated state: %s -> %s", before, after)
	}
	f.exec(`UPDATE events SET stage='archived',writes_allowed=false,completed_at=$2 WHERE id=$1`, f.event, f.now)
	f.consent("no")
	current, err := f.s.Read(ctx, f.photographer, f.event, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !current.CanReadHistory || current.CanEdit || current.CanReadOriginal || current.RecordingURL != "" {
		t.Fatalf("archive history granted bytes/write: %+v", current)
	}
	if _, err = f.s.History(ctx, f.photographer, f.event, item.ID, 0, 30); err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.s.Original(ctx, f.photographer, f.event, item.ID); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("history revived bytes: %v", err)
	}
	f.exec(`UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='photographer'`, f.event, f.photographer)
	if _, err = f.s.History(ctx, f.photographer, f.event, item.ID, 0, 30); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("former owner retained history: %v", err)
	}
	for _, limit := range []int32{0, 101} {
		if _, err = f.s.History(ctx, f.owner, f.event, item.ID, 0, limit); !errors.Is(err, access.ErrInvalid) {
			t.Fatalf("invalid limit: %v", err)
		}
	}
}

func TestMediaHistoryRechecksRoleAfterContendingEventLock(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	item := f.upload(nil)
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SELECT id FROM events WHERE id=$1 FOR UPDATE`, f.event); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := f.s.History(ctx, f.photographer, f.event, item.ID, 0, 30); result <- err }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting bool
		if err = f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%LockEvent%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("history did not contend on native event authority lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err = tx.Exec(ctx, `UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='photographer'`, f.event, f.photographer); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-result:
		if !errors.Is(err, access.ErrForbidden) {
			t.Fatalf("stale photographer role granted history: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("history remained blocked")
	}
}
