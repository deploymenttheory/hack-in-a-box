package automation

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
)

type pausedReadOnlyHandler struct {
	*Service
	entered chan struct{}
}

func (h pausedReadOnlyHandler) ReadOnlyEligible(
	ctx context.Context,
	d jobs.Delivery,
) (bool, error) {
	close(h.entered)
	return h.Service.ReadOnlyEligible(ctx, d)
}

func TestRetainedReadOnlyCallbackCannotBlockCurrentEventCancellation(t *testing.T) {
	f := newFixture(t)
	entered := make(chan struct{})
	outbox, err := jobs.New(
		f.pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{HandlerKind: pausedReadOnlyHandler{Service: f.s, entered: entered}},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.s.Jobs = outbox
	f.s.Composer.Jobs = outbox
	f.retainedBinding()
	f.now = f.now.Add(70 * time.Second)
	id := f.maintain().Occurrences[0].ID
	if _, err = (event.Service{Pool: f.pool}).Transition(
		t.Context(),
		f.owner.ID,
		f.id,
		f.revision(),
		"archived",
		"Archive ordinary work",
		false,
	); err != nil {
		t.Fatal(err)
	}
	occurrence, err := dbgen.New(f.pool).
		AutomationOccurrence(t.Context(), dbgen.AutomationOccurrenceParams{EventID: f.id, ID: id})
	if err != nil || occurrence.OperationID == nil {
		t.Fatal("retained operation missing", err)
	}
	tx, err := f.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(t.Context())) }()
	if _, err = dbgen.New(tx).LockEvent(t.Context(), f.id); err != nil {
		t.Fatal(err)
	}
	executeCtx, cancelExecute := context.WithCancel(t.Context())
	defer cancelExecute()
	finished := make(chan error, 1)
	go func() { finished <- outbox.Execute(executeCtx, *occurrence.OperationID) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("retained callback never entered")
	}
	commandCtx, cancelCommand := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancelCommand()
	changed, err := dbgen.New(tx).CancelOperation(commandCtx, *occurrence.OperationID)
	if err != nil {
		t.Fatalf("event cancellation blocked behind retained callback: %v", err)
	}
	if changed != 1 {
		t.Fatal("current retained operation was not cancelled")
	}
	if err = tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("retained worker did not resume after cancellation")
	}
	current, err := dbgen.New(f.pool).Operation(t.Context(), *occurrence.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if !current.Cancelled || current.Attempts != 0 || f.count() != 0 {
		t.Fatal("cancelled retained work reached an effect or claim")
	}
}
