package jobs_test

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type deferredProvider struct {
	provider
	ready  atomic.Bool
	checks atomic.Int32
}

func (p *deferredProvider) Eligible(context.Context, jobs.Delivery) (bool, error) {
	p.checks.Add(1)
	return p.ready.Load(), nil
}

func TestRiverKeepsDeferredOperationsResumable(t *testing.T) {
	for _, scenario := range []struct {
		name            string
		approvalMissing bool
		cancel          bool
	}{{name: "current_eligibility"}, {name: "approval", approvalMissing: true}, {name: "cancellation", cancel: true}} {
		t.Run(scenario.name, func(t *testing.T) {
			pool := testdb.New(t, true)
			ctx := t.Context()
			eventID := testdb.Event(t, pool, testdb.Person(t, pool).ID)
			handler := &deferredProvider{provider: provider{outcome: jobs.Delivered}}
			handler.ready.Store(scenario.approvalMissing)
			service, err := jobs.New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), map[string]jobs.Handler{"deferred": handler})
			if err != nil {
				t.Fatal(err)
			}
			var operation dbgen.Operation
			if err = database.Within(ctx, pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
				var enqueueErr error
				operation, enqueueErr = service.Enqueue(ctx, tx, dbgen.InsertOperationParams{
					EventID: eventID, OperationKey: "synthetic-deferred", Kind: "deferred",
					TargetID: uuid.New(), DueAt: time.Now().Add(-time.Minute), Approved: !scenario.approvalMissing,
				})
				return enqueueErr
			}); err != nil {
				t.Fatal(err)
			}
			if err = service.Client.Start(ctx); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				stop, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
				defer cancel()
				_ = service.Client.StopAndCancel(stop)
			})
			var jobID int64
			var state string
			var attempted bool
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if err = pool.QueryRow(ctx, "SELECT id,state::text,attempted_at IS NOT NULL FROM river_job WHERE args->>'operation_id'=$1 ORDER BY id DESC LIMIT 1", operation.ID.String()).Scan(&jobID, &state, &attempted); err != nil {
					t.Fatal(err)
				}
				if attempted && (state == "completed" || state == "scheduled") {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !attempted || state != "scheduled" || handler.calls.Load() != 0 {
				t.Fatalf("deferred work became %s instead of remaining scheduled; deliveries=%d", state, handler.calls.Load())
			}
			if !scenario.approvalMissing && handler.checks.Load() == 0 {
				t.Fatal("test did not reach current eligibility")
			}
			if _, err = pool.Exec(ctx, "UPDATE operations SET approved=true,cancelled=$2 WHERE id=$1", operation.ID, scenario.cancel); err != nil {
				t.Fatal(err)
			}
			handler.ready.Store(true)
			// Advance the real scheduled River job through its supported retry API;
			// production's bounded snooze does not need to slow this regression.
			if _, err = service.Client.JobRetry(ctx, jobID); err != nil {
				t.Fatal(err)
			}
			if scenario.cancel {
				deadline = time.Now().Add(5 * time.Second)
				for time.Now().Before(deadline) {
					if err = pool.QueryRow(ctx, "SELECT state::text FROM river_job WHERE id=$1", jobID).Scan(&state); err != nil {
						t.Fatal(err)
					}
					if state == "completed" {
						break
					}
					time.Sleep(10 * time.Millisecond)
				}
				if state != "completed" || handler.calls.Load() != 0 {
					t.Fatal("cancelled deferred operation stayed scheduled or delivered")
				}
				return
			}
			waitState(t, dbgen.New(pool), operation.ID, "delivered")
			if handler.calls.Load() != 1 {
				t.Fatal("resumed operation was lost or delivered more than once")
			}
		})
	}
}
