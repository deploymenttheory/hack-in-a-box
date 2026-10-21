package jobs_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type provider struct {
	calls     atomic.Int32
	active    atomic.Int32
	overlap   atomic.Bool
	outcome   jobs.Outcome
	reconcile jobs.Outcome
	entered   chan string
	release   chan struct{}
}

func (*provider) Eligible(context.Context, jobs.Delivery) (bool, error) { return true, nil }
func (p *provider) Deliver(ctx context.Context, d jobs.Delivery) jobs.Outcome {
	if p.active.Add(1) > 1 {
		p.overlap.Store(true)
	}
	defer p.active.Add(-1)
	p.calls.Add(1)
	if p.entered != nil {
		p.entered <- d.Key
		select {
		case <-p.release:
		case <-ctx.Done():
			return jobs.Ambiguous
		}
	}
	return p.outcome
}
func (p *provider) Reconcile(context.Context, jobs.Delivery) jobs.Outcome { return p.reconcile }

func TestDurableEnqueueTwoWorkersAndRecovery(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	event := uuid.New()
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO events(id,name) VALUES ($1,'Synthetic')",
		event,
	); err != nil {
		t.Fatal(err)
	}
	handler := &provider{
		outcome:   jobs.Delivered,
		reconcile: jobs.Retryable,
		entered:   make(chan string, 4),
		release:   make(chan struct{}),
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	producer, err := jobs.New(pool, logger, map[string]jobs.Handler{"test": handler})
	if err != nil {
		t.Fatal(err)
	}
	first, err := jobs.New(pool, logger, map[string]jobs.Handler{"test": handler})
	if err != nil {
		t.Fatal(err)
	}
	second, err := jobs.New(pool, logger, map[string]jobs.Handler{"test": handler})
	if err != nil {
		t.Fatal(err)
	}
	due := time.Now().Add(-time.Minute)
	input := dbgen.InsertOperationParams{
		EventID:      event,
		OperationKey: "stable-action",
		Kind:         "test",
		TargetID:     event,
		DueAt:        due,
		Approved:     true,
	}
	var operation dbgen.Operation
	if err := database.Within(ctx, pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		var err error
		operation, err = producer.Enqueue(ctx, tx, input)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.Within(ctx, pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		duplicate, err := producer.Enqueue(ctx, tx, input)
		if err == nil && duplicate.ID != operation.ID {
			t.Error("duplicate operation identity")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// The enqueueing process never starts. Two independent clients consume its
	// committed job, proving durable enqueue and competing-worker ownership.
	if err := first.Client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := second.Client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stop, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		first.Client.StopAndCancel(stop)
		second.Client.StopAndCancel(stop)
	})
	select {
	case key := <-handler.entered:
		if key != operation.ID.String() {
			t.Fatal("unstable delivery key")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("queued job never executed")
	}
	if err := second.Execute(ctx, operation.ID); err == nil {
		t.Fatal("second worker should have snoozed on live guard")
	}
	close(handler.release)
	waitState(t, dbgen.New(pool), operation.ID, "delivered")
	if handler.calls.Load() != 1 || handler.overlap.Load() {
		t.Fatal("duplicate live delivery")
	}
	// Recover a process that died with a recorded in-flight attempt. The provider
	// explicitly proves no send occurred, so the same durable operation can retry.
	input.OperationKey = "crashed"
	input.Approved = false
	var crashed dbgen.Operation
	if err := database.Within(ctx, pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		var err error
		crashed, err = first.Enqueue(ctx, tx, input)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	token := uuid.New()
	q := dbgen.New(pool)
	if _, err := q.SetApproval(
		ctx,
		dbgen.SetApprovalParams{ID: crashed.ID, Approved: true, Revision: 1},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ClaimOperation(
		ctx,
		dbgen.ClaimOperationParams{
			ID:         crashed.ID,
			LeaseToken: &token,
			LeaseUntil: pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := q.BeginAttempt(
		ctx,
		dbgen.BeginAttemptParams{OperationID: crashed.ID, LeaseToken: token},
	); err != nil {
		t.Fatal(err)
	}
	if err := second.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if err := second.Execute(ctx, crashed.ID); err != nil {
		t.Fatal(err)
	}
	waitState(t, q, crashed.ID, "delivered")
}

func waitState(t *testing.T, q *dbgen.Queries, id uuid.UUID, state string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		op, err := q.Operation(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if op.State == state {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("operation did not reach %s", state)
}

func TestApprovalCancellationRollbackAndOutcomes(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	event := uuid.New()
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO events(id,name) VALUES ($1,'Synthetic')",
		event,
	); err != nil {
		t.Fatal(err)
	}
	q := dbgen.New(pool)
	for _, outcome := range []jobs.Outcome{jobs.Delivered, jobs.Retryable, jobs.Permanent, jobs.Ambiguous} {
		handler := &provider{outcome: outcome}
		service, err := jobs.New(
			pool,
			slog.New(slog.NewTextHandler(io.Discard, nil)),
			map[string]jobs.Handler{"test": handler},
		)
		if err != nil {
			t.Fatal(err)
		}
		input := dbgen.InsertOperationParams{
			EventID:      event,
			OperationKey: string(outcome),
			Kind:         "test",
			TargetID:     event,
			DueAt:        time.Now().Add(-time.Minute).Truncate(time.Microsecond),
		}
		var op dbgen.Operation
		failure := errors.New("rollback")
		if err := database.Within(ctx, pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
			var err error
			op, err = service.Enqueue(ctx, tx, input)
			if err != nil {
				return err
			}
			return failure
		}); !errors.Is(err, failure) {
			t.Fatal(err)
		}
		if _, err := q.Operation(ctx, op.ID); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal("operation escaped rollback")
		}
		var rolledBackJobs int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM river_job WHERE args->>'operation_id'=$1", op.ID.String()).
			Scan(&rolledBackJobs); err != nil ||
			rolledBackJobs != 0 {
			t.Fatalf("job escaped rollback: %d %v", rolledBackJobs, err)
		}
		if err := database.Within(ctx, pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
			var err error
			op, err = service.Enqueue(ctx, tx, input)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if err := service.Execute(ctx, op.ID); err != nil {
			t.Fatal(err)
		}
		if handler.calls.Load() != 0 {
			t.Fatal("unapproved work executed")
		}
		if _, err := q.SetApproval(
			ctx,
			dbgen.SetApprovalParams{ID: op.ID, Approved: true, Revision: 1},
		); err != nil {
			t.Fatal(err)
		}
		err = service.Execute(ctx, op.ID)
		if outcome == jobs.Retryable {
			if !errors.Is(err, jobs.ErrRetry) {
				t.Fatal(err)
			}
		} else if err != nil {
			t.Fatal(err)
		}
		state := string(outcome)
		if outcome == jobs.Retryable {
			state = "pending"
		}
		waitState(t, q, op.ID, state)
		if outcome != jobs.Retryable {
			if err := service.Execute(ctx, op.ID); err != nil {
				t.Fatal(err)
			}
			if handler.calls.Load() != 1 {
				t.Fatal("terminal outcome blindly retried")
			}
		}
		input.OperationKey += "-cancelled"
		input.Approved = true
		if err := database.Within(ctx, pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
			var err error
			op, err = service.Enqueue(ctx, tx, input)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := q.CancelOperation(ctx, op.ID); err != nil {
			t.Fatal(err)
		}
		before := handler.calls.Load()
		if err := service.Execute(ctx, op.ID); err != nil {
			t.Fatal(err)
		}
		if handler.calls.Load() != before {
			t.Fatal("cancelled work executed")
		}
	}
}
