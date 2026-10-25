package sessionbooking_test

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
	"github.com/deploymenttheory/hack-in-a-box/internal/sessionbooking"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func (f fixture) editEvent(t *testing.T, change func(*event.Input)) event.View {
	t.Helper()
	owner := event.Service{
		Pool:              f.pool,
		Clock:             f.s.Now,
		ReconcileSessions: f.s.ReconcileEventInTransaction,
	}
	current, err := owner.Read(t.Context(), f.owner, f.event)
	if err != nil {
		t.Fatal(err)
	}
	input := event.Input{
		ID:            f.event,
		Name:          current.Name,
		Slug:          current.Slug,
		Configuration: current.Configuration,
	}
	change(&input)
	result, err := owner.Update(t.Context(), f.owner, f.event, current.Revision, input)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestNativeAgendaEditsKeepCalendarIdentityAndCancelRemovedSession(t *testing.T) {
	f := setup(t, 2, []string{"attendee"})
	ctx := t.Context()
	person := f.attendee(t)
	if _, err := f.s.Change(
		ctx,
		person,
		f.event,
		f.id,
		sessionbooking.Command{RequestID: uuid.New(), Action: "reserve"},
	); err != nil {
		t.Fatal(err)
	}
	initial, err := f.s.Read(ctx, person, f.event, f.id)
	if err != nil {
		t.Fatal(err)
	}
	f.editEvent(t, func(input *event.Input) { input.Name = "Renamed event without session change" })
	unchanged, err := f.s.Read(ctx, person, f.event, f.id)
	if err != nil || unchanged.CalendarSequence != initial.CalendarSequence {
		t.Fatal("unrelated event edit changed session sequence", err)
	}
	f.editEvent(t, func(input *event.Input) {
		input.Configuration.LiveStart = input.Configuration.LiveStart.Add(8 * 24 * time.Hour)
	})
	changed, err := f.s.Read(ctx, person, f.event, f.id)
	if err != nil || changed.ID != initial.ID ||
		changed.CalendarSequence != initial.CalendarSequence+1 ||
		!changed.Start.Equal(initial.Start.Add(8*24*time.Hour)) {
		t.Fatal("date edit lost stable identity or sequence", changed, err)
	}
	f.editEvent(t, func(input *event.Input) { input.Configuration.Schedule = []event.AgendaItem{} })
	cancelled, err := f.s.Read(ctx, person, f.event, f.id)
	if err != nil || !cancelled.Cancelled || cancelled.OwnBooking.State != "cancelled" ||
		cancelled.MeetingURL != nil {
		t.Fatal("agenda removal retained active session/link", cancelled, err)
	}
	if _, err = f.s.Change(
		ctx,
		person,
		f.event,
		f.id,
		sessionbooking.Command{RequestID: uuid.New(), Action: "reserve", ExpectedRevision: 2},
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("removed session accepted booking", err)
	}
}

func TestNativeWithdrawalAndSessionCancellationSerializeWithoutAdmissionBypass(t *testing.T) {
	f := setup(t, 1, []string{"attendee"})
	ctx := t.Context()
	f.editEvent(t, func(input *event.Input) {
		c := &input.Configuration
		c.LiveStart = f.s.Now().AddDate(0, 1, 0)
		c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
		c.SubmissionCutoff = c.LiveStart.Add(6 * time.Hour)
		c.TeamFreeze = c.LiveStart.AddDate(0, 0, -8)
		c.AdmissionCutoff = c.TeamFreeze.Add(-time.Hour)
	})
	r := registration.Service{
		Pool:              f.pool,
		Now:               f.s.Now,
		ReconcileSessions: f.s.ReconcileEventInTransaction,
	}
	definition := forms.Definition{
		ID:       uuid.New(),
		Name:     "Synthetic booking test registration",
		Audience: "attendee",
		Fields:   []forms.Field{},
	}
	draft, err := r.SaveForm(ctx, f.owner, f.event, 0, definition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.PublishForm(
		ctx,
		f.owner,
		f.event,
		definition.ID,
		draft.Revision,
		draft.Version,
		true,
	); err != nil {
		t.Fatal(err)
	}
	owner := event.Service{
		Pool:              f.pool,
		Clock:             f.s.Now,
		ReconcileSessions: f.s.ReconcileEventInTransaction,
	}
	current, err := owner.Read(ctx, f.owner, f.event)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = owner.Transition(
		ctx,
		f.owner,
		f.event,
		current.Revision,
		"registration",
		"",
		false,
	); err != nil {
		t.Fatal(err)
	}
	a, b := testdb.Person(t, f.pool), testdb.Person(t, f.pool)
	apply := func(person uuid.UUID) registration.View {
		result, err := r.Apply(
			ctx,
			person,
			f.event,
			registration.Application{
				ID:             uuid.New(),
				FormID:         definition.ID,
				FormVersion:    1,
				Answers:        forms.Answers{},
				TeamPreference: "solo",
			},
		)
		if err != nil || result.State != "confirmed" {
			t.Fatal("native event admission fixture failed", err)
		}
		return result
	}
	first := apply(a.ID)
	_ = apply(b.ID)
	for _, person := range []uuid.UUID{a.ID, b.ID} {
		if _, err = f.s.Change(
			ctx,
			person,
			f.event,
			f.id,
			sessionbooking.Command{RequestID: uuid.New(), Action: "reserve"},
		); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	var withdrawErr, cancelErr error
	wg.Go(
		func() { _, withdrawErr = r.Respond(ctx, a.ID, f.event, first.ID, first.Revision, "withdraw") },
	)
	wg.Go(func() {
		_, cancelErr = f.s.Change(
			ctx,
			a.ID,
			f.event,
			f.id,
			sessionbooking.Command{RequestID: uuid.New(), Action: "cancel", ExpectedRevision: 1},
		)
	})
	wg.Wait()
	if withdrawErr != nil {
		t.Fatal(withdrawErr)
	}
	if cancelErr != nil && !errors.Is(cancelErr, access.ErrForbidden) &&
		!errors.Is(cancelErr, access.ErrNotFound) {
		t.Fatal("unexpected serialized cancellation result", cancelErr)
	}
	page, err := f.s.Read(ctx, b.ID, f.event, f.id)
	if err != nil || page.Booked != 1 || page.OwnBooking.State != "booked" {
		t.Fatal("withdrawal did not atomically free/promote session place", page, err)
	}
	var state string
	if err = f.pool.QueryRow(ctx, `SELECT state FROM competition_claims WHERE event_id=$1 AND person_id=$2`, f.event, a.ID).
		Scan(&state); err != nil ||
		state != "withdrawn" {
		t.Fatal("session cancellation re-admitted competition", state, err)
	}
}

func TestScheduleConflictsAreWarningsAndPersonalAgendaIsPrivate(t *testing.T) {
	f := setup(t, 2, []string{"attendee"})
	ctx := t.Context()
	person, other := f.attendee(t), f.attendee(t)
	second := uuid.New()
	f.editEvent(t, func(input *event.Input) {
		item := input.Configuration.Schedule[0]
		item.ID = second
		item.Title = "Overlapping session"
		item.Minutes = 30
		input.Configuration.Schedule = append(input.Configuration.Schedule, item)
	})
	if _, err := f.s.Configure(ctx, f.owner, f.event, second, 0, f.definition); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{f.id, second} {
		if _, err := f.s.Change(
			ctx,
			person,
			f.event,
			id,
			sessionbooking.Command{RequestID: uuid.New(), Action: "reserve"},
		); err != nil {
			t.Fatal("warning became invented booking gate", err)
		}
	}
	page, err := f.s.Read(ctx, person, f.event, second)
	if err != nil || len(page.Conflicts) != 1 || page.Conflicts[0] != f.id {
		t.Fatal("conflict warning missing", page, err)
	}
	own, err := f.s.List(ctx, person, f.event, uuid.Nil, 50, true)
	if err != nil || len(own.Items) != 2 {
		t.Fatal("personal agenda incomplete", err)
	}
	stranger, err := f.s.List(ctx, other, f.event, uuid.Nil, 50, true)
	if err != nil || len(stranger.Items) != 0 {
		t.Fatal("other person's bookings exposed", err)
	}
}

func TestBookingWindowsUseNativeDSTGapAndFoldChoices(t *testing.T) {
	f := setup(t, 2, []string{"attendee"})
	ctx := t.Context()
	var raw []byte
	if err := f.pool.QueryRow(ctx, `SELECT configuration FROM events WHERE id=$1`, f.event).
		Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var config event.Configuration
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	config.LiveStart = time.Date(2027, 11, 1, 10, 0, 0, 0, time.UTC)
	raw, _ = json.Marshal(config)
	if _, err := f.pool.Exec(
		ctx,
		`UPDATE events SET configuration=$2 WHERE id=$1`,
		f.event,
		raw,
	); err != nil {
		t.Fatal(err)
	}
	d := f.definition
	d.BookingOpens.Wall = "2027-10-31T01:30:00"
	d.BookingCloses.Wall = "2027-11-01T09:00:00"
	if _, err := f.s.Configure(
		ctx,
		f.owner,
		f.event,
		f.id,
		1,
		d,
	); !errors.Is(
		err,
		schedule.ErrAmbiguous,
	) {
		t.Fatal("fold silently chosen", err)
	}
	d.BookingOpens.DSTChoice = "earlier"
	earlier, err := f.s.Configure(ctx, f.owner, f.event, f.id, 1, d)
	if err != nil {
		t.Fatal(err)
	}
	d.BookingOpens.DSTChoice = "later"
	later, err := f.s.Configure(ctx, f.owner, f.event, f.id, 2, d)
	if err != nil || later.BookingOpens.Sub(earlier.BookingOpens) != time.Hour {
		t.Fatal("fold instants conflated", err)
	}
	d.BookingOpens = sessionbooking.LocalTime{Wall: "2027-03-28T01:30:00"}
	if _, err = f.s.Configure(
		ctx,
		f.owner,
		f.event,
		f.id,
		3,
		d,
	); !errors.Is(
		err,
		schedule.ErrLocalTime,
	) {
		t.Fatal("DST gap normalized silently", err)
	}
}
