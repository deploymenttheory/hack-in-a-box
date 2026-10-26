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
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type domainLeaseObservation struct {
	wall, lease, attempt time.Time
	err                  error
}

// This synthetic external-delivery boundary observes a real committed lease
// from inside an actual River callback. It is not a native outcome source or a
// substitute for the app-level queued retained-review/inbox regression.
type domainLeaseProbe struct {
	pool      *pgxpool.Pool
	checks    atomic.Int32
	calls     atomic.Int32
	delivered chan domainLeaseObservation
}

func (p *domainLeaseProbe) Eligible(context.Context, jobs.Delivery) (bool, error) {
	p.checks.Add(1)
	return true, nil
}

func (p *domainLeaseProbe) Deliver(ctx context.Context, delivery jobs.Delivery) jobs.Outcome {
	p.calls.Add(1)
	observation := domainLeaseObservation{wall: time.Now()}
	operation, err := dbgen.New(p.pool).Operation(ctx, delivery.ID)
	observation.err = err
	if err == nil {
		observation.lease = operation.LeaseUntil.Time
		if !operation.LeaseUntil.Valid || operation.LeaseToken == nil {
			observation.err = errors.New("provider callback did not own an actual committed delivery lease")
		} else {
			observation.err = p.pool.QueryRow(ctx, "SELECT started_at FROM delivery_attempts WHERE operation_id=$1 AND lease_token=$2 AND outcome='running'", delivery.ID, *operation.LeaseToken).Scan(&observation.attempt)
		}
	}
	p.delivered <- observation
	return jobs.Delivered
}

func (*domainLeaseProbe) Reconcile(context.Context, jobs.Delivery) jobs.Outcome {
	return jobs.Ambiguous
}

func TestRiverDomainDueEligibilityKeepsLeasesAndNilDefaultOnWallTime(t *testing.T) {
	for _, historical := range []bool{false, true} {
		name := "nil_default"
		if historical {
			name = "historical_domain"
		}
		t.Run(name, func(t *testing.T) {
			pool := testdb.New(t, true)
			ctx := t.Context()
			eventID := testdb.Event(t, pool, testdb.Person(t, pool).ID)
			probe := &domainLeaseProbe{pool: pool, delivered: make(chan domainLeaseObservation, 2)}
			service, err := jobs.New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), map[string]jobs.Handler{"synthetic_clock_delivery": probe})
			if err != nil {
				t.Fatal(err)
			}
			started := time.Now()
			due := started.Add(-time.Second)
			var phase atomic.Int64
			if historical {
				phase.Store(started.AddDate(0, 0, -120).UnixNano())
				due = started.AddDate(0, 0, -90)
				service.DomainNow = func() time.Time { return time.Unix(0, phase.Load()) }
			}
			var operation dbgen.Operation
			if err := database.Within(ctx, pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
				var err error
				operation, err = service.Enqueue(ctx, tx, dbgen.InsertOperationParams{
					EventID: eventID, OperationKey: "synthetic-domain-clock", Kind: "synthetic_clock_delivery",
					TargetID: uuid.New(), DueAt: due, Approved: true,
				})
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if err := service.Client.Start(ctx); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				stop, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
				defer cancel()
				if err := service.Client.StopAndCancel(stop); err != nil {
					t.Error(err)
				}
			})
			if historical {
				deadline := time.Now().Add(8 * time.Second)
				var state string
				var attempted bool
				var scheduled time.Time
				for time.Now().Before(deadline) {
					if err := pool.QueryRow(ctx, "SELECT state::text,attempted_at IS NOT NULL,scheduled_at FROM river_job WHERE args->>'operation_id'=$1 ORDER BY id DESC LIMIT 1", operation.ID.String()).Scan(&state, &attempted, &scheduled); err != nil {
						t.Fatal(err)
					}
					if attempted && state == "scheduled" {
						break
					}
					time.Sleep(20 * time.Millisecond)
				}
				if !attempted || state != "scheduled" || probe.checks.Load() != 0 || probe.calls.Load() != 0 ||
					!scheduled.After(time.Now()) || !scheduled.Before(time.Now().Add(40*time.Second)) {
					t.Fatal("early domain operation attempted provider work or lost its real wall-time queued snooze")
				}
				early, err := dbgen.New(pool).Operation(ctx, operation.ID)
				if err != nil || early.Attempts != 0 || early.LeaseToken != nil || early.LeaseUntil.Valid {
					t.Fatal("early domain operation acquired an application delivery attempt/lease", err)
				}
				phase.Store(due.UnixNano())
			}
			var observed domainLeaseObservation
			wait := 8 * time.Second
			if historical {
				wait = 45 * time.Second
			}
			select {
			case observed = <-probe.delivered:
			case <-time.After(wait):
				t.Fatal("actual River delivery did not run when due without a queue retry shortcut")
			}
			if observed.err != nil {
				t.Fatal(observed.err)
			}
			if observed.lease.Before(observed.wall.Add(117*time.Second)) || observed.lease.After(observed.wall.Add(121*time.Second)) {
				t.Fatal("real delivery lease did not retain its wall-time two-minute lifetime")
			}
			waitState(t, dbgen.New(pool), operation.ID, "delivered")
			attempt := observed.attempt
			if attempt.Before(started.Add(-time.Second)) || attempt.After(time.Now().Add(time.Second)) || probe.calls.Load() != 1 {
				t.Fatal("actual attempt timestamps or delivery count departed from wall-time execution")
			}
			t.Logf("actual queued delivery: due=%s lease=%s attempt=%s domain_clock=%t", due.UTC().Format(time.RFC3339Nano), observed.lease.UTC().Format(time.RFC3339Nano), attempt.UTC().Format(time.RFC3339Nano), historical)
		})
	}
}
