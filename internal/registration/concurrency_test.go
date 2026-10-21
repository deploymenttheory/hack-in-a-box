package registration

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/permissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/teams"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestPreparationPreservesFIFOOffersAndConfirmedOnlyMinimum(t *testing.T) {
	f := newRegistrationFixture(t, 2)
	ctx := t.Context()
	q := dbgen.New(f.pool)
	c := f.event.Configuration
	c.MinimumAttendance = 2
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(
		ctx,
		"UPDATE events SET configuration=$2 WHERE id=$1",
		f.event.ID,
		data,
	); err != nil {
		t.Fatal(err)
	}
	people := make([]dbgen.Person, 4)
	registrations := make([]View, 4)
	for i := range people {
		people[i] = testdb.Person(t, f.pool)
		registrations[i], err = f.service.Apply(ctx, people[i].ID, f.event.ID, f.application())
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.service.SaveOfferPolicy(
		ctx,
		f.owner.ID,
		f.event.ID,
		OfferSettings{Automated: true, WorkingDays: 2},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Respond(
		ctx,
		people[1].ID,
		f.event.ID,
		registrations[1].ID,
		registrations[1].Revision,
		"withdraw",
	); err != nil {
		t.Fatal(err)
	}
	events := event.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	v, err := events.Read(ctx, f.owner.ID, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := events.Transition(
		ctx,
		f.owner.ID,
		f.event.ID,
		v.Revision,
		"preparation",
		"",
		false,
	); !errors.Is(
		err,
		event.ErrBlocked,
	) {
		t.Fatalf("unaccepted reservation satisfied minimum: %v", err)
	}
	offered, err := f.service.Own(ctx, people[2].ID, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Respond(
		ctx,
		people[2].ID,
		f.event.ID,
		offered[0].ID,
		offered[0].Revision,
		"accept",
	); err != nil {
		t.Fatal(err)
	}
	v, err = events.Read(ctx, f.owner.ID, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := events.Transition(
		ctx,
		f.owner.ID,
		f.event.ID,
		v.Revision,
		"preparation",
		"",
		false,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Respond(
		ctx,
		people[0].ID,
		f.event.ID,
		registrations[0].ID,
		registrations[0].Revision,
		"withdraw",
	); err != nil {
		t.Fatal(err)
	}
	afterGo, err := f.service.Own(ctx, people[3].ID, f.event.ID)
	if err != nil || afterGo[0].State != "offered" {
		t.Fatalf("FIFO offer did not continue during preparation: %+v %v", afterGo, err)
	}
	row, err := q.Event(ctx, f.event.ID)
	if err != nil || row.Stage != "preparation" {
		t.Fatalf("withdrawal undid go decision: %v", err)
	}
	if _, err := f.service.Respond(
		ctx,
		people[3].ID,
		f.event.ID,
		afterGo[0].ID,
		afterGo[0].Revision,
		"accept",
	); err != nil {
		t.Fatal(err)
	}
}

func TestFreezeAndMembershipMoveSerializeAgainstPublicationFacts(t *testing.T) {
	f := newRegistrationFixture(t, 6)
	ctx := t.Context()
	s := teams.Service{Pool: f.pool, Now: func() time.Time { return f.now }}
	people := make([]dbgen.Person, 6)
	for i := range people {
		people[i] = testdb.Person(t, f.pool)
		if _, err := f.service.Apply(ctx, people[i].ID, f.event.ID, f.application()); err != nil {
			t.Fatal(err)
		}
	}
	a := teamAct(t, s, people[0].ID, f.event.ID, teams.Command{Action: "create", Name: "Team A"}).Teams[0].ID
	b := teamAct(t, s, people[3].ID, f.event.ID, teams.Command{Action: "create", Name: "Team B"}).Teams[0].ID
	for _, i := range []int{1, 2} {
		teamAct(
			t,
			s,
			f.owner.ID,
			f.event.ID,
			teams.Command{Action: "move", TeamID: a, PersonID: people[i].ID},
		)
	}
	for _, i := range []int{4, 5} {
		teamAct(
			t,
			s,
			f.owner.ID,
			f.event.ID,
			teams.Command{Action: "move", TeamID: b, PersonID: people[i].ID},
		)
	}
	events := event.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	v, err := events.Read(ctx, f.owner.ID, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := events.Transition(
		ctx,
		f.owner.ID,
		f.event.ID,
		v.Revision,
		"preparation",
		"",
		false,
	); err != nil {
		t.Fatal(err)
	}
	current, err := s.Read(ctx, f.owner.ID, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	freeze, move := make(chan error, 1), make(chan error, 1)
	go func() {
		<-start
		_, err := s.Act(
			ctx,
			f.owner.ID,
			f.event.ID,
			current.Revision,
			teams.Command{Action: "freeze", Reason: "Freeze reviewed teams"},
		)
		freeze <- err
	}()
	go func() {
		<-start
		_, err := s.Act(
			ctx,
			f.owner.ID,
			f.event.ID,
			current.Revision,
			teams.Command{
				Action:   "move",
				TeamID:   b,
				PersonID: people[1].ID,
				Reason:   "Proposed rebalance",
			},
		)
		move <- err
	}()
	close(start)
	freezeErr, moveErr := <-freeze, <-move
	if freezeErr == nil {
		if !errors.Is(moveErr, access.ErrConflict) {
			t.Fatalf("move bypassed frozen revision: %v", moveErr)
		}
	} else if !errors.Is(freezeErr, access.ErrConflict) || moveErr != nil {
		t.Fatalf("freeze/move result: %v/%v", freezeErr, moveErr)
	}
	latest, err := s.Read(ctx, f.owner.ID, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !latest.Frozen {
		if _, err := s.Act(
			ctx,
			f.owner.ID,
			f.event.ID,
			latest.Revision,
			teams.Command{Action: "freeze", Reason: "Recheck changed teams"},
		); !errors.Is(
			err,
			teams.ErrUnready,
		) {
			t.Fatalf("froze undersized result: %v", err)
		}
	} else {
		facts, err := dbgen.New(f.pool).EventFacts(ctx, f.event.ID)
		if err != nil || facts.AssignedAttendees != 6 || facts.FrozenTeams != 2 {
			t.Fatalf("inconsistent freeze facts: %+v %v", facts, err)
		}
	}
}

func TestPreparationAndApplicationRaceHasOneSerializableOutcome(t *testing.T) {
	f := newRegistrationFixture(t, 2)
	ctx := t.Context()
	first, second := testdb.Person(t, f.pool), testdb.Person(t, f.pool)
	if _, err := f.service.Apply(ctx, first.ID, f.event.ID, f.application()); err != nil {
		t.Fatal(err)
	}
	events := event.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	current, err := events.Read(ctx, f.owner.ID, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	application := make(chan error, 1)
	transition := make(chan error, 1)
	go func() {
		<-start
		_, err := f.service.Apply(ctx, second.ID, f.event.ID, f.application())
		application <- err
	}()
	go func() {
		<-start
		_, err := events.Transition(
			ctx,
			f.owner.ID,
			f.event.ID,
			current.Revision,
			"preparation",
			"",
			false,
		)
		transition <- err
	}()
	close(start)
	applyErr, goErr := <-application, <-transition
	if applyErr == nil {
		if !errors.Is(goErr, access.ErrConflict) {
			t.Fatalf("registration should advance decision revision: %v", goErr)
		}
	} else if !errors.Is(applyErr, ErrClosed) || goErr != nil {
		t.Fatalf("go/apply outcome: %v / %v", goErr, applyErr)
	}
	current, err = events.Read(ctx, f.owner.ID, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Stage == "registration" {
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
	}
	if _, err := f.service.Apply(
		ctx,
		testdb.Person(t, f.pool).ID,
		f.event.ID,
		f.application(),
	); !errors.Is(
		err,
		ErrClosed,
	) {
		t.Fatalf("new application after go: %v", err)
	}
}

func TestConcurrentOfferResponseWithdrawalAndWorkerRetriesPreserveCapacity(t *testing.T) {
	f := newRegistrationFixture(t, 1)
	ctx := t.Context()
	a, b, c := testdb.Person(t, f.pool), testdb.Person(t, f.pool), testdb.Person(t, f.pool)
	first, err := f.service.Apply(ctx, a.ID, f.event.ID, f.application())
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.service.Apply(ctx, b.ID, f.event.ID, f.application())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Apply(ctx, c.ID, f.event.ID, f.application()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.SaveOfferPolicy(
		ctx,
		f.owner.ID,
		f.event.ID,
		OfferSettings{Automated: true, WorkingDays: 2},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Respond(
		ctx,
		a.ID,
		f.event.ID,
		first.ID,
		first.Revision,
		"withdraw",
	); err != nil {
		t.Fatal(err)
	}
	own, err := f.service.Own(ctx, b.ID, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	offered := own[0]
	var wg sync.WaitGroup
	results := make(chan error, 18)
	start := make(chan struct{})
	for i := range 18 {
		wg.Go(func() {
			<-start
			var err error
			switch i % 3 {
			case 0:
				_, err = f.service.Respond(
					ctx,
					b.ID,
					f.event.ID,
					second.ID,
					offered.Revision,
					"accept",
				)
			case 1:
				_, err = f.service.Respond(
					ctx,
					b.ID,
					f.event.ID,
					second.ID,
					offered.Revision,
					"withdraw",
				)
			default:
				err = f.service.MaintainOffers(ctx)
			}
			results <- err
		})
	}
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil && !errors.Is(err, access.ErrConflict) {
			t.Fatalf("burst result: %v", err)
		}
	}
	q := dbgen.New(f.pool)
	confirmed, err := q.ConfirmedAttendees(ctx, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	reserved, err := q.ReservedAttendees(ctx, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed+reserved != 1 {
		t.Fatalf("capacity reservation invariant: %d confirmed + %d reserved", confirmed, reserved)
	}
	var offers, notices int
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM registration_offers WHERE event_id=$1", f.event.ID).
		Scan(&offers); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM operations WHERE event_id=$1 AND kind='registration.offer'", f.event.ID).
		Scan(&notices); err != nil {
		t.Fatal(err)
	}
	if offers != notices || offers < 1 || offers > 2 {
		t.Fatalf("worker replay duplicated offers: %d/%d", offers, notices)
	}
}

func TestOfferAcceptanceAndOrganiserGrantCannotCreateCompetingOrganiser(t *testing.T) {
	f := newRegistrationFixture(t, 1)
	ctx := t.Context()
	a, b := testdb.Person(t, f.pool), testdb.Person(t, f.pool)
	first, err := f.service.Apply(ctx, a.ID, f.event.ID, f.application())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Apply(ctx, b.ID, f.event.ID, f.application()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.SaveOfferPolicy(
		ctx,
		f.owner.ID,
		f.event.ID,
		OfferSettings{Automated: true, WorkingDays: 2},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Respond(
		ctx,
		a.ID,
		f.event.ID,
		first.ID,
		first.Revision,
		"withdraw",
	); err != nil {
		t.Fatal(err)
	}
	own, err := f.service.Own(ctx, b.ID, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	offered := own[0]
	current, err := dbgen.New(f.pool).Event(ctx, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	accepted, granted := make(chan error, 1), make(chan error, 1)
	go func() {
		<-start
		_, err := f.service.Respond(ctx, b.ID, f.event.ID, offered.ID, offered.Revision, "accept")
		accepted <- err
	}()
	go func() {
		<-start
		_, err := (permissions.Service{Pool: f.pool}).Change(
			ctx,
			permissions.Change{
				Actor:                             f.owner.ID,
				Event:                             f.event.ID,
				Person:                            b.ID,
				Role:                              "organiser",
				Active:                            true,
				AcknowledgeCompetitionRestriction: true,
				Revision:                          current.Revision,
			},
		)
		granted <- err
	}()
	close(start)
	if err := <-accepted; err != nil {
		t.Fatal(err)
	}
	if err := <-granted; !errors.Is(err, permissions.ErrWithdrawal) &&
		!errors.Is(err, access.ErrConflict) {
		t.Fatalf("incompatible role granted: %v", err)
	}
	roles, err := dbgen.New(f.pool).
		ActiveRoles(ctx, dbgen.ActiveRolesParams{EventID: f.event.ID, PersonID: b.ID})
	if err != nil || len(roles) != 0 {
		t.Fatalf("competing organiser exists: %v %v", roles, err)
	}
}

func TestBusinessCalendarAndConcurrentFinalCutoffNeverBackfill(t *testing.T) {
	f := newRegistrationFixture(t, 1)
	ctx := t.Context()
	friday := time.Date(2027, 5, 28, 9, 0, 0, 0, time.UTC)
	c := f.event.Configuration
	c.LiveStart = time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	c.SubmissionCutoff = c.LiveStart.Add(7 * time.Hour)
	c.TeamFreeze = c.LiveStart.AddDate(0, 0, -8)
	c.AdmissionCutoff = c.TeamFreeze.Add(-time.Hour)
	c.Calendar.ExcludedDates = []string{"2027-05-31"}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(
		ctx,
		"UPDATE events SET configuration=$2 WHERE id=$1",
		f.event.ID,
		data,
	); err != nil {
		t.Fatal(err)
	}
	f.service.Now = func() time.Time { return friday }
	a, b, d := testdb.Person(t, f.pool), testdb.Person(t, f.pool), testdb.Person(t, f.pool)
	first, err := f.service.Apply(ctx, a.ID, f.event.ID, f.application())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []dbgen.Person{b, d} {
		if _, err := f.service.Apply(ctx, p.ID, f.event.ID, f.application()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.service.SaveOfferPolicy(
		ctx,
		f.owner.ID,
		f.event.ID,
		OfferSettings{Automated: true, WorkingDays: 2},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Respond(
		ctx,
		a.ID,
		f.event.ID,
		first.ID,
		first.Revision,
		"withdraw",
	); err != nil {
		t.Fatal(err)
	}
	own, err := f.service.Own(ctx, b.ID, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	offered := own[0]
	if offered.Offer == nil ||
		!offered.Offer.ExpiresAt.Equal(time.Date(2027, 6, 2, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("weekend/holiday expiry: %+v", offered.Offer)
	}
	// At the final boundary, acceptance and duplicate workers see the same clock.
	cutoff := f.service
	cutoff.Now = func() time.Time { return c.AdmissionCutoff }
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for i := range 12 {
		wg.Go(func() {
			if i%2 == 0 {
				v, err := cutoff.Respond(
					ctx,
					b.ID,
					f.event.ID,
					offered.ID,
					offered.Revision,
					"accept",
				)
				if err == nil && (v.State != "declined" || v.Offer.State != "expired") {
					err = errors.New("accepted expired offer")
				}
				results <- err
			} else {
				results <- cutoff.MaintainOffers(ctx)
			}
		})
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	q := dbgen.New(f.pool)
	confirmed, err := q.ConfirmedAttendees(ctx, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	reserved, err := q.ReservedAttendees(ctx, f.event.ID)
	if err != nil || confirmed+reserved != 0 {
		t.Fatalf("place after cutoff: %d/%d %v", confirmed, reserved, err)
	}
	remaining, err := f.service.Own(ctx, d.ID, f.event.ID)
	if err != nil || remaining[0].State != "waitlisted" || remaining[0].Offer != nil {
		t.Fatalf("late backfill: %+v %v", remaining, err)
	}
}
