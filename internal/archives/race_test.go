package archives

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestArchiveExporterWaitingBehindRoleLossRechecksAfterEventLock(t *testing.T) {
	pool := testdb.New(t, true)
	owner := testdb.Person(t, pool)
	id := testdb.Event(t, pool, owner.ID)
	execute(
		t,
		pool,
		"UPDATE events SET stage='archived',writes_allowed=false,completed_at=now()WHERE id=$1",
		id,
	)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err = dbgen.New(tx).LockEvent(ctx, id); err != nil {
		t.Fatal(err)
	}
	var holder int32
	if err = tx.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&holder); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(
		ctx,
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2",
		id,
		owner.ID,
	); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, e := (Service{Pool: pool}).ExportSummary(ctx, owner.ID, id); done <- e }()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err = pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))", holder).
			Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err = <-done:
			t.Fatal("export bypassed event lock", err)
		case <-ctx.Done():
			t.Fatal("export did not reach held event lock")
		case <-ticker.C:
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if !errors.Is(err, access.ErrForbidden) {
			t.Fatal("waiting export used stale role", err)
		}
	case <-ctx.Done():
		t.Fatal("waiting export did not finish")
	}
	var count int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE event_id=$1 AND action='archive.exported'", id).
		Scan(&count); err != nil ||
		count != 0 {
		t.Fatal("denied export appended success audit", count, err)
	}
}
