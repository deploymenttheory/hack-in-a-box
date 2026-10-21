package registration

import (
	"encoding/json"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type registrationFixture struct {
	pool    *pgxpool.Pool
	service Service
	owner   dbgen.Person
	event   event.View
	form    forms.Definition
	now     time.Time
}

func newRegistrationFixture(t *testing.T, capacity int) registrationFixture {
	t.Helper()
	pool := testdb.New(t, true)
	owner := testdb.Person(t, pool)
	q := dbgen.New(pool)
	if _, err := q.BootstrapAdmin(t.Context(), owner.ID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	c := event.DefaultConfiguration()
	c.Capacity = capacity
	c.MinimumAttendance = 1
	c.LiveStart = now.AddDate(0, 1, 0)
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	c.SubmissionCutoff = c.LiveStart.Add(6 * time.Hour)
	c.TeamFreeze = c.LiveStart.AddDate(0, 0, -8)
	c.AdmissionCutoff = c.TeamFreeze.Add(-time.Hour)
	events := event.Service{Pool: pool, Clock: func() time.Time { return now }}
	v, err := events.Create(
		t.Context(),
		owner.ID,
		event.Input{
			ID:                                uuid.New(),
			Name:                              "Registration fixture",
			Slug:                              "registration-" + uuid.NewString(),
			Configuration:                     c,
			AcknowledgeCompetitionRestriction: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := Service{Pool: pool, Now: func() time.Time { return now }}
	d := formFixture()
	f, err := s.SaveForm(t.Context(), owner.ID, v.ID, 0, d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PublishForm(
		t.Context(),
		owner.ID,
		v.ID,
		d.ID,
		f.Revision,
		f.Version,
		true,
	); err != nil {
		t.Fatal(err)
	}
	v, err = events.Read(t.Context(), owner.ID, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	v, err = events.Transition(t.Context(), owner.ID, v.ID, v.Revision, "registration", "", false)
	if err != nil {
		t.Fatal(err)
	}
	return registrationFixture{pool: pool, service: s, owner: owner, event: v, form: d, now: now}
}

func (f registrationFixture) application() Application {
	return Application{
		ID:             uuid.New(),
		FormID:         f.form.ID,
		FormVersion:    1,
		TeamPreference: "solo",
		Answers: forms.Answers{
			f.form.Fields[0].ID.String(): json.RawMessage(`"Starting out"`),
		},
	}
}

func TestLastPlaceFIFOIdempotencyAndConfirmedGate(t *testing.T) {
	f := newRegistrationFixture(t, 1)
	ctx := t.Context()
	q := dbgen.New(f.pool)
	people := make([]dbgen.Person, 8)
	inputs := make([]Application, 8)
	for i := range people {
		people[i] = testdb.Person(t, f.pool)
		inputs[i] = f.application()
	}
	type outcome struct {
		view  View
		err   error
		index int
	}
	results := make(chan outcome, len(people))
	start := make(chan struct{})
	for i := range people {
		go func() {
			<-start
			v, err := f.service.Apply(ctx, people[i].ID, f.event.ID, inputs[i])
			results <- outcome{v, err, i}
		}()
	}
	close(start)
	all := make([]outcome, 0, len(people))
	for range people {
		r := <-results
		if r.err != nil {
			t.Fatal(r.err)
		}
		all = append(all, r)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].view.Sequence < all[j].view.Sequence })
	for i, r := range all {
		expected := "waitlisted"
		if i == 0 {
			expected = "confirmed"
		}
		if r.view.State != expected {
			t.Fatal("last place/FIFO order broken")
		}
		if i > 0 && (r.view.WaitlistPosition == nil || *r.view.WaitlistPosition != int64(i)) {
			t.Fatal("unstable waitlist position")
		}
	}
	first := all[0]
	retry, err := f.service.Apply(ctx, people[first.index].ID, f.event.ID, inputs[first.index])
	if err != nil || retry.ID != first.view.ID || retry.Sequence != first.view.Sequence {
		t.Fatal("retry changed allocation", err)
	}
	changed := inputs[first.index]
	changed.TeamPreference = "captain"
	if _, err := f.service.Apply(
		ctx,
		people[first.index].ID,
		f.event.ID,
		changed,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("changed idempotency payload accepted")
	}
	if _, err := f.service.Apply(
		ctx,
		people[first.index].ID,
		f.event.ID,
		f.application(),
	); !errors.Is(
		err,
		ErrDuplicate,
	) {
		t.Fatal("duplicate active registration accepted")
	}
	count, err := q.ConfirmedAttendees(ctx, f.event.ID)
	if err != nil || count != 1 {
		t.Fatal("incorrect confirmed count", count, err)
	}
	events := event.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	current, err := events.Read(ctx, f.owner.ID, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Stage != "registration" {
		t.Fatal("minimum auto-advanced event")
	}
	if _, err := events.Transition(
		ctx,
		f.owner.ID,
		f.event.ID,
		current.Revision,
		"preparation",
		"",
		false,
	); err != nil {
		t.Fatal(err)
	}
	late := testdb.Person(t, f.pool)
	if _, err := f.service.Apply(
		ctx,
		late.ID,
		f.event.ID,
		f.application(),
	); !errors.Is(
		err,
		ErrClosed,
	) {
		t.Fatal("application accepted after go")
	}
	if retry, err := f.service.Apply(
		ctx,
		people[first.index].ID,
		f.event.ID,
		inputs[first.index],
	); err != nil ||
		retry.State != "confirmed" {
		t.Fatal("committed retry failed after closure", err)
	}
}

func TestEligibilityFacilitatorsPromotionAndCapacityReduction(t *testing.T) {
	f := newRegistrationFixture(t, 2)
	ctx := t.Context()
	q := dbgen.New(f.pool)
	if _, err := f.service.Apply(ctx, f.owner.ID, f.event.ID, f.application()); err == nil {
		t.Fatal("organiser competed")
	}
	first, second, waiting := testdb.Person(
		t,
		f.pool,
	), testdb.Person(
		t,
		f.pool,
	), testdb.Person(
		t,
		f.pool,
	)
	for _, person := range []dbgen.Person{first, second} {
		if _, err := f.service.Apply(ctx, person.ID, f.event.ID, f.application()); err != nil {
			t.Fatal(err)
		}
	}
	w, err := f.service.Apply(ctx, waiting.ID, f.event.ID, f.application())
	if err != nil || w.State != "waitlisted" {
		t.Fatal(err)
	}
	facilitator := formFixture()
	facilitator.Audience = "mentor"
	v, err := f.service.SaveForm(ctx, f.owner.ID, f.event.ID, 0, facilitator)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.PublishForm(
		ctx,
		f.owner.ID,
		f.event.ID,
		v.Definition.ID,
		v.Revision,
		v.Version,
		true,
	); err != nil {
		t.Fatal(err)
	}
	staff := testdb.Person(t, f.pool)
	a := f.application()
	a.FormID = facilitator.ID
	a.Answers = forms.Answers{facilitator.Fields[0].ID.String(): json.RawMessage(`"Experienced"`)}
	if v, err := f.service.Apply(
		ctx,
		staff.ID,
		f.event.ID,
		a,
	); err != nil ||
		v.State != "received" {
		t.Fatal("facilitator consumed a place", err)
	}
	events := event.Service{Pool: f.pool}
	current, err := events.Read(ctx, f.owner.ID, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	c := current.Configuration
	c.Capacity = 1
	if _, err := events.Update(
		ctx,
		f.owner.ID,
		f.event.ID,
		current.Revision,
		event.Input{Name: current.Name, Slug: current.Slug, Configuration: c},
	); !errors.Is(
		err,
		event.ErrBlocked,
	) {
		t.Fatal("capacity silently reduced below allocations")
	}
	if _, err := f.service.Promote(
		ctx,
		f.owner.ID,
		f.event.ID,
		w.ID,
		w.Revision,
		"Reasoned exception",
	); !errors.Is(
		err,
		ErrCapacity,
	) {
		t.Fatal("promotion over capacity")
	}
	// Increase capacity, then test the recorded explicit exception without changing FIFO history.
	c.Capacity = 3
	if _, err := events.Update(
		ctx,
		f.owner.ID,
		f.event.ID,
		current.Revision,
		event.Input{Name: current.Name, Slug: current.Slug, Configuration: c},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Promote(ctx, f.owner.ID, f.event.ID, w.ID, w.Revision, ""); err == nil {
		t.Fatal("unexplained exception accepted")
	}
	if _, err := f.service.Promote(
		ctx,
		f.owner.ID,
		f.event.ID,
		w.ID,
		w.Revision,
		"Additional capacity approved",
	); err != nil {
		t.Fatal(err)
	}
	history, err := q.RegistrationHistory(
		ctx,
		dbgen.RegistrationHistoryParams{EventID: f.event.ID, RegistrationID: w.ID},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[1].Reason != "Additional capacity approved" {
		t.Fatal("promotion reason missing")
	}
	f.service.Now = func() time.Time { return f.event.Configuration.AdmissionCutoff }
	if _, err := f.service.Apply(
		ctx,
		testdb.Person(t, f.pool).ID,
		f.event.ID,
		f.application(),
	); !errors.Is(
		err,
		ErrClosed,
	) {
		t.Fatal("application after cutoff")
	}
}
