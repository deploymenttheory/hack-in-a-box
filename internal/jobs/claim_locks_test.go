package jobs_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type eventLockProvider struct {
	provider
	pool    *pgxpool.Pool
	entered chan struct{}
}

func (p *eventLockProvider) Eligible(ctx context.Context, d jobs.Delivery) (bool, error) {
	err := p.currentEvent(ctx, d)
	return err == nil, err
}

func (p *eventLockProvider) Reconcile(ctx context.Context, d jobs.Delivery) jobs.Outcome {
	if p.currentEvent(ctx, d) != nil {
		return jobs.Ambiguous
	}
	return jobs.Retryable
}

func (p *eventLockProvider) currentEvent(ctx context.Context, d jobs.Delivery) error {
	close(p.entered)
	err := database.Within(ctx, p.pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		_, err := q.LockEvent(ctx, d.EventID)
		return err
	})
	return err
}

// Event commands consistently lock Event before changing their operations. A
// provider's current-authority check also locks Event on its own transaction.
// Holding Operation while calling that provider creates a client-mediated cycle:
// the claim waits on the callback, the callback waits on Event, and the event
// command waits on Operation. PostgreSQL cannot see the first dependency.
func TestEligibilityCannotBlockEventCancellation(t *testing.T) {
	testEventCancellation(t, false)
}

func TestRecoveryCannotBlockEventCancellation(t *testing.T) {
	testEventCancellation(t, true)
}

func testEventCancellation(t *testing.T, recovery bool) {
	t.Helper()
	pool := testdb.New(t, true)
	ctx := t.Context()
	eventID := testdb.Event(t, pool, testdb.Person(t, pool).ID)
	handler := &eventLockProvider{
		provider: provider{outcome: jobs.Delivered},
		pool:     pool,
		entered:  make(chan struct{}),
	}
	service, err := jobs.New(
		pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{"event-lock": handler},
	)
	if err != nil {
		t.Fatal(err)
	}
	var op dbgen.Operation
	if err = database.Within(ctx, pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		var enqueueErr error
		op, enqueueErr = service.Enqueue(
			ctx,
			tx,
			dbgen.InsertOperationParams{
				EventID:      eventID,
				OperationKey: "event-lock",
				Kind:         "event-lock",
				TargetID:     uuid.New(),
				DueAt:        time.Now().Add(-time.Minute),
				Approved:     true,
			},
		)
		return enqueueErr
	}); err != nil {
		t.Fatal(err)
	}

	if recovery {
		token := uuid.New()
		if _, err = pool.Exec(
			ctx,
			"UPDATE operations SET state='running',attempts=1,lease_token=$2,lease_until=now()-interval '1 minute' WHERE id=$1",
			op.ID,
			token,
		); err != nil {
			t.Fatal(err)
		}
		if err = dbgen.New(pool).
			BeginAttempt(ctx, dbgen.BeginAttemptParams{OperationID: op.ID, LeaseToken: token}); err != nil {
			t.Fatal(err)
		}
	}

	command, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = command.Rollback(context.WithoutCancel(ctx)) }()
	if _, err = dbgen.New(command).LockEvent(ctx, eventID); err != nil {
		t.Fatal(err)
	}
	executeCtx, stop := context.WithCancel(ctx)
	defer stop()
	executed := make(chan error, 1)
	go func() {
		if recovery {
			executed <- service.Reconcile(executeCtx)
			return
		}
		executed <- service.Execute(executeCtx, op.ID)
	}()
	select {
	case <-handler.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("worker never entered current-authority callback")
	}

	// This deadline bounds the regression itself; it does not rely on production's
	// much longer worker timeout or the database's deadlock detector recovering it.
	commandCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cancelled := make(chan error, 1)
	go func() {
		cancelled <- dbgen.New(command).CancelEventOperations(commandCtx, dbgen.CancelEventOperationsParams{
			EventID: eventID,
			AtTime:  time.Now(),
		})
	}()
	blocked := 0
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
waiting:
	for {
		select {
		case err = <-cancelled:
			break waiting
		case <-ticker.C:
			var currentBlocked int
			if scanErr := pool.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND cardinality(pg_blocking_pids(pid))>0").Scan(&currentBlocked); scanErr != nil {
				t.Fatal(scanErr)
			}
			if currentBlocked > blocked {
				blocked = currentBlocked
			}
		}
	}
	if err != nil {
		stop()
		_ = command.Rollback(ctx)
		<-executed
		t.Fatalf(
			"event cancellation blocked behind eligibility callback (blocked PostgreSQL sessions=%d): %v",
			blocked,
			err,
		)
	}
	if err = command.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-executed:
		if err != nil {
			t.Fatal(fmt.Errorf("execute after event cancellation: %w", err))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not resume after event command committed")
	}
	current, err := dbgen.New(pool).Operation(ctx, op.ID)
	if err != nil {
		t.Fatal(err)
	}
	expectedAttempts := int32(0)
	if recovery {
		expectedAttempts = 1
	}
	if !current.Cancelled || current.Attempts != expectedAttempts || handler.calls.Load() != 0 {
		t.Fatalf(
			"cancelled operation was claimed or delivered: state=%s attempts=%d calls=%d",
			current.State,
			current.Attempts,
			handler.calls.Load(),
		)
	}
}

type approvalChangeProvider struct {
	provider
	entered chan struct{}
	resume  chan struct{}
	checks  atomic.Int32
}

func (p *approvalChangeProvider) Eligible(ctx context.Context, _ jobs.Delivery) (bool, error) {
	if p.checks.Add(1) > 1 {
		return true, nil
	}
	close(p.entered)
	select {
	case <-p.resume:
		return true, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func TestApprovalChangedDuringEligibilityRequiresFreshCheck(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	eventID := testdb.Event(t, pool, testdb.Person(t, pool).ID)
	handler := &approvalChangeProvider{
		provider: provider{outcome: jobs.Delivered},
		entered:  make(chan struct{}),
		resume:   make(chan struct{}),
	}
	service, err := jobs.New(
		pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{"review": handler},
	)
	if err != nil {
		t.Fatal(err)
	}
	var op dbgen.Operation
	if err = database.Within(ctx, pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		var enqueueErr error
		op, enqueueErr = service.Enqueue(
			ctx,
			tx,
			dbgen.InsertOperationParams{
				EventID:      eventID,
				OperationKey: "review",
				Kind:         "review",
				TargetID:     uuid.New(),
				DueAt:        time.Now().Add(-time.Minute),
				Approved:     true,
			},
		)
		return enqueueErr
	}); err != nil {
		t.Fatal(err)
	}
	executed := make(chan error, 1)
	executeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer close(handler.resume)
	go func() { executed <- service.Execute(executeCtx, op.ID) }()
	select {
	case <-handler.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("eligibility never began")
	}
	commandCtx, stop := context.WithTimeout(ctx, 2*time.Second)
	defer stop()
	if err = database.Within(commandCtx, pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, lockErr := q.LockEvent(commandCtx, eventID); lockErr != nil {
			return lockErr
		}
		revoked, changeErr := q.SetApproval(
			commandCtx,
			dbgen.SetApprovalParams{ID: op.ID, Approved: false, Revision: op.Revision},
		)
		if changeErr != nil {
			return changeErr
		}
		_, changeErr = q.SetApproval(
			commandCtx,
			dbgen.SetApprovalParams{ID: op.ID, Approved: true, Revision: revoked.Revision},
		)
		return changeErr
	}); err != nil {
		t.Fatal(err)
	}
	handler.resume <- struct{}{}
	if err = <-executed; err != nil {
		t.Fatal(err)
	}
	current, err := dbgen.New(pool).Operation(ctx, op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.State != "pending" || current.Attempts != 0 || handler.calls.Load() != 0 {
		t.Fatal("stale eligibility consumed the replacement approval")
	}
	if err = service.Execute(ctx, op.ID); err != nil {
		t.Fatal(err)
	}
	if handler.checks.Load() != 2 || handler.calls.Load() != 1 {
		t.Fatal("fresh current approval did not resume exactly once")
	}
}
