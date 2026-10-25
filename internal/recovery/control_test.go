package recovery_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/deploymenttheory/hack-in-a-box/internal/app"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/recovery"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestRecoveryHoldFencesStartupAndRequiresCurrentReview(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	event := uuid.New()
	_, err := pool.Exec(ctx, `INSERT INTO events(id,name) VALUES($1,'Synthetic recovery');`, event)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(
		ctx,
		`INSERT INTO operations(event_id,operation_key,kind,target_id,due_at,approved,state) VALUES($1,'original-pending','test',$1,now(),true,'pending'),($1,'original-unknown','test',$1,now(),true,'ambiguous')`,
		event,
	)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := recovery.Runtime(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = recovery.Hold(ctx, pool); !errors.Is(err, recovery.ErrBusy) {
		t.Fatalf("hold accepted live runtime: %v", err)
	}
	if err = lease.Close(ctx); err != nil {
		t.Fatal(err)
	}
	held, err := recovery.Hold(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if held.HoldID == nil || held.ReviewEntries != 2 {
		t.Fatalf("missing hold/review: %+v", held)
	}
	same, err := recovery.Hold(ctx, pool)
	if err != nil || same.HoldID == nil || *same.HoldID != *held.HoldID {
		t.Fatalf("hold not stable: %+v %v", same, err)
	}

	// Exercise the actual worker composition: even opening/cleaning files is forbidden.
	storage := t.TempDir() + "/not-opened"
	blocked, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	err = app.RunWorker(
		blocked,
		config.Config{DatabaseURL: pool.Config().ConnString(), StoragePath: storage},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("held worker did not wait: %v", err)
	}
	if _, err = os.Stat(storage); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("worker touched storage while held: %v", err)
	}
	var runs int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM reconciliation_runs").
		Scan(&runs); err != nil ||
		runs != 0 {
		t.Fatalf("worker reconciled during hold: %d %v", runs, err)
	}
	if err = database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}

	// A rollback must not remove the startup barrier for an unreviewed restoration.
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()
	migrations, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		os.DirFS("../database/migrations"),
	)
	if err != nil {
		t.Fatal(err)
	}
	// Isolate this owner's migration so later empty migrations cannot change
	// the schema version retained by its populated rollback guard.
	if _, err = migrations.DownTo(ctx, 48); err != nil {
		t.Fatal(err)
	}
	if _, err = migrations.DownTo(ctx, 47); err == nil {
		t.Fatal("rollback bypassed active recovery hold")
	}
	var retainedVersion int64
	if err = pool.QueryRow(ctx, "SELECT version_id FROM goose_db_version WHERE is_applied ORDER BY id DESC LIMIT 1").
		Scan(&retainedVersion); err != nil || retainedVersion != 48 {
		t.Fatalf("failed rollback changed recovery schema: %d %v", retainedVersion, err)
	}
	if err = database.Migrate(ctx, pool, database.SchemaVersion); err != nil {
		t.Fatal(err)
	}
	if err = database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}

	// Native schedules and verification attempts can precede generic job rows.
	// Their safe review entries must change the fingerprint without disclosing
	// command bodies or private destinations.
	scheduleID := uuid.New()
	if _, err = pool.Exec(
		ctx,
		`INSERT INTO schedule_items(id,event_id,definition,due_at,approved) VALUES($1,$2,'{"secret":"SYNTHETIC-PRIVATE-SCHEDULE"}',now(),true)`,
		scheduleID,
		event,
	); err != nil {
		t.Fatal(err)
	}
	scheduled, err := recovery.Read(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if scheduled.ReviewEntries != 3 || scheduled.Fingerprint == held.Fingerprint {
		t.Fatal("new pending native schedule omitted from review")
	}
	if err = recovery.Resume(
		ctx,
		pool,
		*held.HoldID,
		held.Fingerprint,
	); !errors.Is(
		err,
		recovery.ErrReview,
	) {
		t.Fatalf("new schedule did not invalidate review: %v", err)
	}
	person := testdb.Person(t, pool)
	if _, err = pool.Exec(
		ctx,
		`INSERT INTO email_verifications(id,person_id,session_revision,mailbox,mailbox_hash,code_hash,requested_at,expires_at) VALUES($1,$2,1,'SYNTHETIC-PRIVATE-MAILBOX@example.test','\x01','\x02',now(),now()+interval '1 hour')`,
		uuid.New(),
		person.ID,
	); err != nil {
		t.Fatal(err)
	}
	withVerification, err := recovery.Read(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if withVerification.ReviewEntries != 4 ||
		withVerification.Fingerprint == scheduled.Fingerprint {
		t.Fatal("native verification attempt omitted from review")
	}
	encoded, err := json.Marshal(withVerification)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"SYNTHETIC-PRIVATE", "mailbox", "code_hash", "definition", "ciphertext"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("private content entered deployment review: %s", forbidden)
		}
	}
	if err = recovery.Resume(
		ctx,
		pool,
		*held.HoldID,
		scheduled.Fingerprint,
	); !errors.Is(
		err,
		recovery.ErrReview,
	) {
		t.Fatalf("new native verification did not invalidate review: %v", err)
	}

	// A reviewer cannot resume a stale snapshot or another hold incarnation.
	if err = recovery.Resume(
		ctx,
		pool,
		uuid.New(),
		held.Fingerprint,
	); !errors.Is(
		err,
		recovery.ErrReview,
	) {
		t.Fatalf("wrong hold resumed: %v", err)
	}
	if _, err = pool.Exec(
		ctx,
		"UPDATE operations SET revision=revision+1 WHERE state='pending'",
	); err != nil {
		t.Fatal(err)
	}
	if err = recovery.Resume(
		ctx,
		pool,
		*held.HoldID,
		held.Fingerprint,
	); !errors.Is(
		err,
		recovery.ErrReview,
	) {
		t.Fatalf("stale review resumed: %v", err)
	}
	current, err := recovery.Read(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if err = recovery.Resume(ctx, pool, *current.HoldID, current.Fingerprint); err != nil {
		t.Fatal(err)
	}
	lease, err = recovery.Runtime(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if err = lease.Close(ctx); err != nil {
		t.Fatal(err)
	}
	var unchanged int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM operations WHERE state IN ('pending','ambiguous') AND approved AND attempts=0").
		Scan(&unchanged); err != nil ||
		unchanged != 2 {
		t.Fatalf("resume replayed/modified originals: %d %v", unchanged, err)
	}
	var history int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM recovery_history").
		Scan(&history); err != nil ||
		history != 2 {
		t.Fatalf("missing audit: %d %v", history, err)
	}
	if _, err = pool.Exec(ctx, "TRUNCATE recovery_history"); err == nil {
		t.Fatal("maintenance history was truncatable")
	}
	if _, err = pool.Exec(ctx, "DELETE FROM recovery_history"); err == nil {
		t.Fatal("maintenance history was mutable")
	}
	if _, err = migrations.DownTo(ctx, 47); err != nil {
		t.Fatalf("reviewed rollback blocked: %v", err)
	}
}

func TestRecoverySQLClientRollsBackIncompleteStream(t *testing.T) {
	target := testdb.NewRecoveryTarget(t)
	err := target.RestoreStream(
		t.Context(),
		strings.NewReader(
			"CREATE TABLE synthetic_partial_restore(value text); INSERT INTO synthetic_partial_restore VALUES('synthetic'); SELECT 1/0;",
		),
	)
	if err == nil {
		t.Fatal("incomplete logical restore accepted")
	}
	var present bool
	if err = target.Pool.QueryRow(t.Context(), "SELECT to_regclass('public.synthetic_partial_restore') IS NOT NULL").
		Scan(&present); err != nil {
		t.Fatal(err)
	}
	if present {
		t.Fatal("failed SQL restore exposed partial committed data")
	}
}
