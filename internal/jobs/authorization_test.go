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

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type authorizedProvider struct {
	policy access.Policy
	actor  uuid.UUID
	calls  atomic.Int32
}

func (p *authorizedProvider) Eligible(ctx context.Context, d jobs.Delivery) (bool, error) {
	return p.policy.Authorize(
		ctx,
		p.actor,
		d.EventID,
		d.TargetID,
		access.AssessmentRead,
	) == nil, nil
}

func (p *authorizedProvider) Deliver(context.Context, jobs.Delivery) jobs.Outcome {
	p.calls.Add(1)
	return jobs.Delivered
}

func (*authorizedProvider) Reconcile(context.Context, jobs.Delivery) jobs.Outcome {
	return jobs.Ambiguous
}

func TestQueuedOperationsRecheckCurrentRoleAssignmentAndEvent(t *testing.T) {
	f := testdb.NewAuthorization(t)
	ctx := t.Context()
	f.Grant(t, f.First, f.Person.ID, "judge", true)
	f.Grant(t, f.Second, f.Person.ID, "photographer", true)
	first := f.Resource(t, f.First, "assessment", f.Person.ID)
	second := f.Resource(t, f.Second, "assessment", f.Person.ID)
	handler := &authorizedProvider{policy: f.Policy, actor: f.Person.ID}
	service, err := jobs.New(
		f.Pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{"policy-contract": handler},
	)
	if err != nil {
		t.Fatal(err)
	}
	enqueue := func(event, target uuid.UUID) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		err := database.Within(ctx, f.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
			op, err := service.Enqueue(
				ctx,
				tx,
				dbgen.InsertOperationParams{
					EventID:      event,
					TargetID:     target,
					Kind:         "policy-contract",
					OperationKey: uuid.NewString(),
					DueAt:        time.Now().Add(-time.Minute),
					Approved:     true,
				},
			)
			id = op.ID
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	pending := enqueue(f.First, first)
	other := enqueue(f.Second, second)
	f.Grant(t, f.First, f.Person.ID, "judge", false)
	for _, id := range []uuid.UUID{pending, other} {
		if err := service.Execute(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if handler.calls.Load() != 0 {
		t.Fatal("queued role or cross-event privilege trusted")
	}
	f.Grant(t, f.First, f.Person.ID, "judge", true)
	if err := service.Execute(ctx, pending); err != nil {
		t.Fatal(err)
	}
	if handler.calls.Load() != 1 {
		t.Fatal("authorized delivery did not run")
	}
	staleAssignment := enqueue(f.First, first)
	if _, err := f.Pool.Exec(
		ctx,
		"DELETE FROM resource_assignments WHERE event_id=$1",
		f.First,
	); err != nil {
		t.Fatal(err)
	}
	if err := service.Execute(ctx, staleAssignment); err != nil {
		t.Fatal(err)
	}
	if handler.calls.Load() != 1 {
		t.Fatal("queued resource assignment trusted")
	}
	archived := enqueue(f.First, first)
	if _, err := f.Pool.Exec(
		ctx,
		"UPDATE events SET writes_allowed=false,stage='archived' WHERE id=$1",
		f.First,
	); err != nil {
		t.Fatal(err)
	}
	if err := service.Execute(ctx, archived); err != nil {
		t.Fatal(err)
	}
	if handler.calls.Load() != 1 {
		t.Fatal("archive did not block background effects")
	}
}
