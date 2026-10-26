package jobs_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type startLatencyProvider struct{ entered chan uuid.UUID }

func (*startLatencyProvider) Eligible(
	context.Context,
	jobs.Delivery,
) (bool, error) {
	return true, nil
}

func (p *startLatencyProvider) Deliver(_ context.Context, d jobs.Delivery) jobs.Outcome {
	p.entered <- d.ID
	return jobs.Delivered
}

func (*startLatencyProvider) Reconcile(context.Context, jobs.Delivery) jobs.Outcome {
	return jobs.Ambiguous
}

// This is a normal-wall-clock worker component probe, not the five-event load
// profile. The provider alone is deterministic. Run/River/guards/attempts and
// PostgreSQL remain real; no test invokes Reconcile, Execute or Deliver itself.
func TestApprovedIntentCreatedAfterWorkerStartupStartsWithinThirtySeconds(t *testing.T) {
	pool := testdb.New(t, true)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	eventID := testdb.Event(t, pool, testdb.Person(t, pool).ID)
	provider := &startLatencyProvider{entered: make(chan uuid.UUID, 64)}
	service, err := jobs.New(
		pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{"load-start-probe": provider},
	)
	if err != nil {
		t.Fatal(err)
	}
	var barrier dbgen.Operation
	if err := database.Within(ctx, pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		var err error
		barrier, err = service.Enqueue(
			ctx,
			tx,
			dbgen.InsertOperationParams{
				EventID:      eventID,
				TargetID:     uuid.New(),
				OperationKey: "startup-barrier",
				Kind:         "load-start-probe",
				DueAt:        time.Now(),
				Approved:     true,
			},
		)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- service.Run(ctx, 10*time.Second) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-finished:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(15 * time.Second):
			t.Error("normal worker did not terminate")
		}
	})
	select {
	case id := <-provider.entered:
		if id != barrier.ID {
			t.Fatal("unexpected startup delivery")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("normal River worker did not execute its startup barrier")
	}
	waitState(t, dbgen.New(pool), barrier.ID, "delivered")
	// Initial reconciliation cannot rescue this ordinary InsertTx control,
	// because it is created after the actual worker startup barrier.
	var direct dbgen.Operation
	if err := database.Within(ctx, pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		var err error
		direct, err = service.Enqueue(
			ctx,
			tx,
			dbgen.InsertOperationParams{
				EventID:      eventID,
				TargetID:     uuid.New(),
				OperationKey: "post-start-insert-tx-control",
				Kind:         "load-start-probe",
				DueAt:        time.Now(),
				Approved:     true,
			},
		)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-provider.entered:
		if id != direct.ID {
			t.Fatal("unexpected transactional control delivery")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("ordinary transactional River enqueue did not start promptly after worker startup")
	}
	waitState(t, dbgen.New(pool), direct.ID, "delivered")

	// Repositories/repository-access persist approved durable intents through
	// this native query without River insertion. Creating them AFTER the actual
	// worker barrier prevents startup reconciliation from hiding its cadence.
	const count = 31
	ids := make([]uuid.UUID, 0, count)
	q := dbgen.New(pool)
	for index := range count {
		op, err := q.InsertOperation(
			ctx,
			dbgen.InsertOperationParams{
				EventID:      eventID,
				TargetID:     uuid.New(),
				OperationKey: fmt.Sprintf("late-intent-%02d", index),
				Kind:         "load-start-probe",
				DueAt:        time.Now(),
				Approved:     true,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, op.ID)
	}
	blocked, err := q.InsertOperation(
		ctx,
		dbgen.InsertOperationParams{
			EventID:      eventID,
			TargetID:     uuid.New(),
			OperationKey: "unapproved-control",
			Kind:         "load-start-probe",
			DueAt:        time.Now(),
			Approved:     false,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := q.InsertOperation(
		ctx,
		dbgen.InsertOperationParams{
			EventID:      eventID,
			TargetID:     uuid.New(),
			OperationKey: "cancelled-control",
			Kind:         "load-start-probe",
			DueAt:        time.Now(),
			Approved:     true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := q.CancelOperation(ctx, cancelled.ID); err != nil || changed != 1 {
		t.Fatal("cancel control", changed, err)
	}

	// Wait for the actual START timestamp even on the old minute cadence.
	// The acceptance assertion below remains exactly 30 seconds.
	deadline := time.NewTimer(75 * time.Second)
	defer deadline.Stop()
	seen := make(map[uuid.UUID]bool, count)
	for len(seen) < count {
		select {
		case id := <-provider.entered:
			if id == blocked.ID || id == cancelled.ID {
				t.Fatal("normal worker dispatched unapproved/cancelled control")
			}
			if seen[id] {
				t.Fatal("normal worker dispatched duplicate healthy effect")
			}
			seen[id] = true
		case <-deadline.C:
			t.Fatalf(
				"only %d/%d approved native intents produced an actual start timestamp during the bounded probe",
				len(seen),
				count,
			)
		}
	}
	rows, err := pool.Query(
		ctx,
		`SELECT o.id,o.due_at,min(a.started_at),count(a.id) FROM operations o LEFT JOIN delivery_attempts a ON a.operation_id=o.id WHERE o.id=ANY($1::uuid[]) GROUP BY o.id,o.due_at`,
		ids,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var delays []time.Duration
	for rows.Next() {
		var id uuid.UUID
		var due, started time.Time
		var attempts int64
		if err := rows.Scan(&id, &due, &started, &attempts); err != nil {
			t.Fatal(err)
		}
		if attempts != 1 || started.Before(due) {
			t.Fatal("invalid native guarded attempt", attempts, started.Sub(due))
		}
		delays = append(delays, started.Sub(due))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(delays) != count {
		t.Fatal("missing native attempt facts", len(delays))
	}
	sort.Slice(delays, func(a, b int) bool { return delays[a] < delays[b] })
	p95 := delays[29]
	if p95 > 30*time.Second {
		t.Fatal("native due-to-first-attempt p95 exceeds 30 seconds", p95)
	}
	t.Logf(
		"normal-wall due-to-guarded-attempt start: samples=%d p95=%s max=%s; no manual reconciliation/handler invocation",
		count,
		p95,
		delays[len(delays)-1],
	)
}
