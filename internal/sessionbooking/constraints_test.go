package sessionbooking_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/permissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/sessionbooking"
)

func TestSessionPromotionWaitsForNativeRoleLossTransaction(t *testing.T) {
	f := setup(t, 1, []string{"mentor"})
	ctx := t.Context()
	a, b, c := f.attendee(t), f.attendee(t), f.attendee(t)
	for _, person := range []uuid.UUID{a, b, c} {
		if err := f.q.SetMembership(
			ctx,
			dbgen.SetMembershipParams{
				EventID:  f.event,
				PersonID: person,
				Role:     "mentor",
				Active:   true,
			},
		); err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.Change(
			ctx,
			person,
			f.event,
			f.id,
			sessionbooking.Command{SessionRevision: 1, RequestID: uuid.New(), Action: "reserve"},
		); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, f.event)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = permissions.ChangeWithin(
		ctx,
		q,
		permissions.Change{
			Actor:    f.owner,
			Event:    f.event,
			Person:   b,
			Role:     "mentor",
			Active:   false,
			Revision: row.Revision,
		},
	); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := f.s.Change(
			ctx,
			a,
			f.event,
			f.id,
			sessionbooking.Command{
				SessionRevision:  1,
				RequestID:        uuid.New(),
				Action:           "cancel",
				ExpectedRevision: 1,
			},
		)
		result <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	blocked := false
	for time.Now().Before(deadline) {
		if err = f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND pid<>pg_backend_pid())`).
			Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !blocked {
		t.Fatal("promotion did not acquire the same native event lock")
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-result; err != nil {
		t.Fatal(err)
	}
	page, err := f.s.Read(ctx, c, f.event, f.id)
	if err != nil || page.OwnBooking.State != "booked" {
		t.Fatal("FIFO promotion used stale role snapshot", page, err)
	}
	var state string
	if err = f.pool.QueryRow(ctx, `SELECT state FROM session_bookings WHERE event_id=$1 AND session_id=$2 AND person_id=$3`, f.event, f.id, b).
		Scan(&state); err != nil ||
		state != "ineligible" {
		t.Fatal("revoked candidate consumed place", state, err)
	}
}

func TestSessionWindowEdgesAreExactAndClosingDoesNotPromote(t *testing.T) {
	f := setup(t, 1, []string{"attendee"})
	ctx := t.Context()
	a, b, c := f.attendee(t), f.attendee(t), f.attendee(t)
	now := time.Date(2027, 1, 18, 9, 59, 59, 999999999, time.UTC)
	f.s.Now = func() time.Time { return now }
	if _, err := f.s.Change(
		ctx,
		a,
		f.event,
		f.id,
		sessionbooking.Command{SessionRevision: 1, RequestID: uuid.New(), Action: "reserve"},
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("booking accepted before opening", err)
	}
	now = now.Add(time.Nanosecond)
	if _, err := f.s.Change(
		ctx,
		a,
		f.event,
		f.id,
		sessionbooking.Command{SessionRevision: 1, RequestID: uuid.New(), Action: "reserve"},
	); err != nil {
		t.Fatal("opening instant rejected", err)
	}
	now = time.Date(2027, 1, 20, 8, 59, 59, 999999999, time.UTC)
	if result, err := f.s.Change(
		ctx,
		b,
		f.event,
		f.id,
		sessionbooking.Command{SessionRevision: 1, RequestID: uuid.New(), Action: "reserve"},
	); err != nil ||
		result.State != "waitlisted" {
		t.Fatal("last instant / independent waitlist", result, err)
	}
	now = now.Add(time.Nanosecond)
	if _, err := f.s.Change(
		ctx,
		c,
		f.event,
		f.id,
		sessionbooking.Command{SessionRevision: 1, RequestID: uuid.New(), Action: "reserve"},
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("closing instant accepted", err)
	}
	if _, err := f.s.Change(
		ctx,
		a,
		f.event,
		f.id,
		sessionbooking.Command{
			SessionRevision:  1,
			RequestID:        uuid.New(),
			Action:           "cancel",
			ExpectedRevision: 1,
		},
	); err != nil {
		t.Fatal(err)
	}
	page, err := f.s.Read(ctx, b, f.event, f.id)
	if err != nil || page.OwnBooking.State != "waitlisted" || page.Booked != 0 || page.CanReserve {
		t.Fatal("closed window silently promoted or reopened", page, err)
	}
}
