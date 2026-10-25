package sessionbooking_test

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/sessionbooking"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type fixture struct {
	pool             *pgxpool.Pool
	q                *dbgen.Queries
	s                sessionbooking.Service
	owner, event, id uuid.UUID
	definition       sessionbooking.Definition
}

func setup(t *testing.T, capacity int, audience []string) fixture {
	t.Helper()
	pool := testdb.New(t, true)
	owner := testdb.Person(t, pool)
	id := testdb.Event(t, pool, owner.ID)
	q := dbgen.New(pool)
	if err := q.InitializeEventFacts(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	session := uuid.New()
	c := event.DefaultConfiguration()
	c.Timezone = "Europe/London"
	c.LiveStart = time.Date(2027, 1, 20, 10, 0, 0, 0, time.UTC)
	c.Schedule = []event.AgendaItem{
		{ID: session, Title: "Synthetic office hour", Audience: audience, Anchor: "live_start"},
	}
	raw, _ := json.Marshal(c)
	if _, err := pool.Exec(
		t.Context(),
		`UPDATE events SET configuration=$2 WHERE id=$1`,
		id,
		raw,
	); err != nil {
		t.Fatal(err)
	}
	s := sessionbooking.Service{
		Pool: pool,
		Now:  func() time.Time { return time.Date(2027, 1, 19, 12, 0, 0, 0, time.UTC) },
	}
	d := sessionbooking.Definition{
		Capacity:        capacity,
		DurationMinutes: 60,
		Room:            "Synthetic room",
		MeetingURL:      "https://example.test/synthetic-meeting",
		FacilitatorIDs:  []uuid.UUID{},
		BookingOpens:    sessionbooking.LocalTime{Wall: "2027-01-18T10:00:00"},
		BookingCloses:   sessionbooking.LocalTime{Wall: "2027-01-20T09:00:00"},
	}
	if _, err := s.Configure(t.Context(), owner.ID, id, session, 0, d); err != nil {
		t.Fatal(err)
	}
	return fixture{pool, q, s, owner.ID, id, session, d}
}

func (f fixture) attendee(t *testing.T) uuid.UUID {
	t.Helper()
	p := testdb.Person(t, f.pool)
	if err := f.q.SetCompetitionState(
		t.Context(),
		dbgen.SetCompetitionStateParams{EventID: f.event, PersonID: p.ID, State: "confirmed"},
	); err != nil {
		t.Fatal(err)
	}
	return p.ID
}

func TestConcurrentSessionPlacesHaveIndependentFIFOAndOriginalReceipts(t *testing.T) {
	f := setup(t, 3, []string{"attendee"})
	ctx := t.Context()
	people := []uuid.UUID{}
	commands := map[uuid.UUID]sessionbooking.Command{}
	for range 16 {
		person := f.attendee(t)
		people = append(people, person)
		commands[person] = sessionbooking.Command{
			SessionRevision: 1,
			RequestID:       uuid.New(),
			Action:          "reserve",
		}
	}
	var wg sync.WaitGroup
	failures := make(chan error, len(people))
	for _, person := range people {
		wg.Go(func() {
			_, err := f.s.Change(ctx, person, f.event, f.id, commands[person])
			if err != nil {
				failures <- err
			}
		})
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	var booked, waiting, confirmed int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE state='booked'),count(*) FILTER(WHERE state='waitlisted') FROM session_bookings WHERE event_id=$1`, f.event).
		Scan(&booked, &waiting); err != nil {
		t.Fatal(err)
	}
	if booked != 3 || waiting != 13 {
		t.Fatalf("oversubscribed or lost FIFO: %d/%d", booked, waiting)
	}
	var bookedPerson, next uuid.UUID
	if err := f.pool.QueryRow(ctx, `SELECT person_id FROM session_bookings WHERE event_id=$1 AND state='booked' ORDER BY queue_sequence LIMIT 1`, f.event).
		Scan(&bookedPerson); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT person_id FROM session_bookings WHERE event_id=$1 AND state='waitlisted' ORDER BY queue_sequence LIMIT 1`, f.event).
		Scan(&next); err != nil {
		t.Fatal(err)
	}
	original, err := f.s.OriginalReceipt(
		ctx,
		bookedPerson,
		f.event,
		f.id,
		commands[bookedPerson].RequestID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Change(
		ctx,
		bookedPerson,
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
	page, err := f.s.Read(ctx, next, f.event, f.id)
	if err != nil || page.OwnBooking == nil || page.OwnBooking.State != "booked" {
		t.Fatalf("FIFO not promoted: %+v %v", page, err)
	}
	retry, err := f.s.Change(ctx, bookedPerson, f.event, f.id, commands[bookedPerson])
	if err != nil || retry != original {
		t.Fatal("original ACK not preserved after cancellation", retry, err)
	}
	changed := commands[bookedPerson]
	changed.Action = "cancel"
	if _, err = f.s.Change(
		ctx,
		bookedPerson,
		f.event,
		f.id,
		changed,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("request identity reused for changed payload", err)
	}
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM competition_claims WHERE event_id=$1 AND state='confirmed'`, f.event).
		Scan(&confirmed); err != nil ||
		confirmed != 16 {
		t.Fatal("session changed competition admission", confirmed, err)
	}
	if _, err = f.s.OriginalReceipt(
		ctx,
		next,
		f.event,
		f.id,
		commands[bookedPerson].RequestID,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("other person's receipt exposed", err)
	}
}

func TestCapacityReductionAndCurrentAssignedFacilitatorScope(t *testing.T) {
	f := setup(t, 2, []string{"attendee"})
	ctx := t.Context()
	a, b, facilitator := f.attendee(t), f.attendee(t), f.attendee(t)
	for _, person := range []uuid.UUID{a, b} {
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
	d := f.definition
	d.Capacity = 1
	d.FacilitatorIDs = []uuid.UUID{facilitator}
	p, err := f.s.Configure(ctx, f.owner, f.event, f.id, 1, d)
	if err != nil || p.Booked != 2 || p.CapacityShortfall != 1 {
		t.Fatalf("capacity silently evicted: %+v %v", p, err)
	}
	roster, err := f.s.Roster(ctx, facilitator, f.event, f.id)
	if err != nil || len(roster) != 2 {
		t.Fatal("exact assigned facilitator unavailable", err)
	}
	if err = (access.EventPolicy{Queries: f.q}).Authorize(
		ctx,
		facilitator,
		f.event,
		f.event,
		access.RosterRead,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("facilitator inherited event logistics", err)
	}
	f.s.Now = func() time.Time { return time.Date(2027, 1, 20, 10, 5, 0, 0, time.UTC) }
	command := sessionbooking.AttendanceCommand{RequestID: uuid.New(), Present: true}
	receipt, err := f.s.Mark(ctx, facilitator, f.event, f.id, a, 0, command)
	if err != nil || receipt.State != "present" {
		t.Fatal("assigned session attendance failed", err)
	}
	d.FacilitatorIDs = []uuid.UUID{}
	if _, err = f.s.Configure(ctx, f.owner, f.event, f.id, 2, d); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Roster(ctx, facilitator, f.event, f.id); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("removed assignment retained roster", err)
	}
	if _, err = f.s.Mark(
		ctx,
		facilitator,
		f.event,
		f.id,
		a,
		0,
		command,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("replayed attendance bypassed current assignment", err)
	}
}

func TestPromotionSkipsCurrentRoleLossAndNeverAdmitsEventWaitlistee(t *testing.T) {
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
	if err := f.q.SetMembership(
		ctx,
		dbgen.SetMembershipParams{EventID: f.event, PersonID: b, Role: "mentor", Active: false},
	); err != nil {
		t.Fatal(err)
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
	page, err := f.s.Read(ctx, c, f.event, f.id)
	if err != nil || page.OwnBooking.State != "booked" {
		t.Fatal("revoked audience consumed FIFO place", err)
	}
	var state string
	if err = f.pool.QueryRow(ctx, `SELECT state FROM session_bookings WHERE event_id=$1 AND person_id=$2`, f.event, b).
		Scan(&state); err != nil ||
		state != "ineligible" {
		t.Fatal("role loss not invalidated", state, err)
	}
	if err = f.q.SetCompetitionState(
		ctx,
		dbgen.SetCompetitionStateParams{EventID: f.event, PersonID: a, State: "waitlisted"},
	); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Change(
		ctx,
		a,
		f.event,
		f.id,
		sessionbooking.Command{
			SessionRevision:  1,
			RequestID:        uuid.New(),
			Action:           "reserve",
			ExpectedRevision: 2,
		},
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("event waitlistee acquired session", err)
	}
}

func TestAllSessionAudienceDoesNotGrantUnrelatedPlatformAdministrationContent(t *testing.T) {
	f := setup(t, 1, []string{"all"})
	admin := testdb.Person(t, f.pool)
	ctx := t.Context()
	if _, err := f.q.BootstrapAdmin(ctx, admin.ID); err != nil {
		t.Fatal(err)
	}
	if err := (access.EventPolicy{Queries: f.q}).Authorize(
		ctx,
		admin.ID,
		f.event,
		f.event,
		access.EventRead,
	); err != nil {
		t.Fatal("fixture metadata reader unavailable", err)
	}
	if _, err := f.s.Read(ctx, admin.ID, f.event, f.id); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("platform metadata authority became session content authority", err)
	}
	page, err := f.s.List(ctx, admin.ID, f.event, uuid.Nil, 1, false)
	if err != nil || len(page.Items) != 0 || page.NextCursor != nil {
		t.Fatal("unrelated platform administrator discovered private session identities", page, err)
	}
}

func TestReceiptStoreFailureIsNotAuthoritativeAbsence(t *testing.T) {
	f := setup(t, 1, []string{"attendee"})
	person := f.attendee(t)
	ctx := t.Context()
	command := sessionbooking.Command{SessionRevision: 1, RequestID: uuid.New(), Action: "reserve"}
	original, err := f.s.Change(ctx, person, f.event, f.id, command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(
		ctx,
		`ALTER TABLE session_commands RENAME TO synthetic_unavailable_commands`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.OriginalReceipt(
		ctx,
		person,
		f.event,
		f.id,
		command.RequestID,
	); err == nil ||
		errors.Is(err, access.ErrNotFound) {
		t.Fatal("receipt-store failure became definitive missing result", err)
	}
	if _, err = f.pool.Exec(
		ctx,
		`ALTER TABLE synthetic_unavailable_commands RENAME TO session_commands`,
	); err != nil {
		t.Fatal(err)
	}
	restored, err := f.s.OriginalReceipt(ctx, person, f.event, f.id, command.RequestID)
	if err != nil || restored != original {
		t.Fatal("store recovery changed original acknowledgement", err)
	}
}

func TestRosterReadCapabilityIsIndependentFromAttendanceWriteWindow(t *testing.T) {
	f := setup(t, 1, []string{"all"})
	ctx := t.Context()
	attendee := f.attendee(t)
	facilitator := f.attendee(t)
	d := f.definition
	d.FacilitatorIDs = []uuid.UUID{facilitator}
	if _, err := f.s.Configure(ctx, f.owner, f.event, f.id, 1, d); err != nil {
		t.Fatal(err)
	}
	for _, actor := range []uuid.UUID{f.owner, facilitator} {
		page, err := f.s.Read(ctx, actor, f.event, f.id)
		if err != nil || !page.CanReadAttendance || page.CanRecordAttendance {
			t.Fatalf("pre-start read/write capabilities: %+v %v", page, err)
		}
		if _, err := f.s.Roster(ctx, actor, f.event, f.id); err != nil {
			t.Fatal(err)
		}
	}
	page, err := f.s.Read(ctx, attendee, f.event, f.id)
	if err != nil || page.CanReadAttendance || page.CanRecordAttendance {
		t.Fatalf("attendee inherited roster authority: %+v %v", page, err)
	}
	d.FacilitatorIDs = []uuid.UUID{}
	if _, err := f.s.Configure(ctx, f.owner, f.event, f.id, 2, d); err != nil {
		t.Fatal(err)
	}
	page, err = f.s.Read(ctx, facilitator, f.event, f.id)
	if err != nil || page.CanReadAttendance || page.CanRecordAttendance {
		t.Fatalf("removed assignment retained roster capability in otherwise readable session: %+v %v", page, err)
	}
	if _, err := f.s.Roster(ctx, facilitator, f.event, f.id); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("removed assignment retained native roster: %v", err)
	}
	if _, err := f.pool.Exec(ctx, "UPDATE events SET stage='archived' WHERE id=$1", f.event); err != nil {
		t.Fatal(err)
	}
	page, err = f.s.Read(ctx, f.owner, f.event, f.id)
	if err != nil || !page.CanReadAttendance || page.CanRecordAttendance {
		t.Fatalf("read-only roster diverged: %+v %v", page, err)
	}
	if _, err := f.s.Roster(ctx, f.owner, f.event, f.id); err != nil {
		t.Fatal(err)
	}
}
