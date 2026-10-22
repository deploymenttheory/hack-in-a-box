package registration

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/teams"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func teamAct(t *testing.T, s teams.Service, actor, eventID uuid.UUID, c teams.Command) teams.Page {
	t.Helper()
	page, err := s.Read(t.Context(), actor, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if c.Reason == "" {
		c.Reason = "Verified organiser allocation"
	}
	result, err := s.Act(t.Context(), actor, eventID, page.Revision, c)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestTeamRebalancingFreezeAndWithdrawalFacts(t *testing.T) {
	f := newRegistrationFixture(t, 6)
	ctx := t.Context()
	s := teams.Service{Pool: f.pool, Now: func() time.Time { return f.now }}
	people := make([]dbgen.Person, 6)
	registrations := make([]View, 6)
	for i := range people {
		people[i] = testdb.Person(t, f.pool)
		var err error
		registrations[i], err = f.service.Apply(ctx, people[i].ID, f.event.ID, f.application())
		if err != nil {
			t.Fatal(err)
		}
	}
	first := teamAct(t, s, people[0].ID, f.event.ID, teams.Command{Action: "create", Name: "Duplicate name"}).Teams[0].ID
	second := teamAct(t, s, people[3].ID, f.event.ID, teams.Command{Action: "create", Name: "Duplicate name"}).Teams[0].ID
	for _, i := range []int{1, 2} {
		teamAct(
			t,
			s,
			f.owner.ID,
			f.event.ID,
			teams.Command{Action: "move", PersonID: people[i].ID, TeamID: first},
		)
	}
	for _, i := range []int{4, 5} {
		teamAct(
			t,
			s,
			f.owner.ID,
			f.event.ID,
			teams.Command{Action: "move", PersonID: people[i].ID, TeamID: second},
		)
	}
	published := teamAct(t, s, f.owner.ID, f.event.ID, teams.Command{Action: "publish"})
	if len(published.Teams) != 2 {
		t.Fatal("duplicate names coalesced")
	}
	own, err := s.Read(ctx, people[1].ID, f.event.ID)
	if err != nil || len(own.Teams) != 1 || len(own.People) != 0 || own.Teams[0].Code != nil {
		t.Fatalf("private roster scope: %+v %v", own, err)
	}
	outsider := testdb.Person(t, f.pool)
	if _, err := s.Read(ctx, outsider.ID, f.event.ID); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("unregistered roster access: %v", err)
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
	p, err := s.Read(ctx, f.owner.ID, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Act(
		ctx,
		f.owner.ID,
		f.event.ID,
		p.Revision,
		teams.Command{Action: "freeze", Reason: "Freeze review"},
	); !errors.Is(
		err,
		teams.ErrUnready,
	) {
		t.Fatalf("froze team below three: %v", err)
	}
	teamAct(t, s, f.owner.ID, f.event.ID, teams.Command{Action: "unlock", TeamID: first, Reason: "Approve reassignment of the depleted team"})
	for _, i := range []int{1, 2} {
		teamAct(
			t,
			s,
			f.owner.ID,
			f.event.ID,
			teams.Command{Action: "move", PersonID: people[i].ID, TeamID: second},
		)
	}
	teamAct(t, s, f.owner.ID, f.event.ID, teams.Command{Action: "disband", TeamID: first})
	frozen := teamAct(t, s, f.owner.ID, f.event.ID, teams.Command{Action: "freeze"})
	if !frozen.Frozen {
		t.Fatal("team freeze missing")
	}
	if _, err := s.Act(
		ctx,
		f.owner.ID,
		f.event.ID,
		frozen.Revision,
		teams.Command{Action: "move", PersonID: people[1].ID, TeamID: first, Reason: "Too late"},
	); !errors.Is(
		err,
		teams.ErrFrozen,
	) {
		t.Fatalf("moved after freeze: %v", err)
	}
	if _, err := f.service.Respond(
		ctx,
		people[3].ID,
		f.event.ID,
		registrations[3].ID,
		registrations[3].Revision,
		"withdraw",
	); err != nil {
		t.Fatal(err)
	}
	teamAct(
		t,
		s,
		f.owner.ID,
		f.event.ID,
		teams.Command{Action: "captain", TeamID: second, PersonID: people[4].ID},
	)
	facts, err := dbgen.New(f.pool).EventFacts(ctx, f.event.ID)
	if err != nil || facts.AssignedAttendees != 4 || facts.TotalTeams != 1 ||
		facts.FrozenTeams != 1 {
		t.Fatalf("withdrawal facts: %+v %v", facts, err)
	}
	// Depletion below three after freeze keeps the remaining team and entry active.
	for _, i := range []int{1, 2} {
		if _, err := f.service.Respond(
			ctx,
			people[i].ID,
			f.event.ID,
			registrations[i].ID,
			registrations[i].Revision,
			"withdraw",
		); err != nil {
			t.Fatal(err)
		}
	}
	remaining, err := s.Read(ctx, people[4].ID, f.event.ID)
	if err != nil || len(remaining.Teams) != 1 || len(remaining.Teams[0].Published) != 2 ||
		remaining.Teams[0].State != "published" {
		t.Fatalf("frozen depleted team did not continue: %+v %v", remaining, err)
	}

	var entries, active int
	if err := f.pool.QueryRow(ctx, "SELECT count(*),count(*) FILTER(WHERE active) FROM competition_entries WHERE event_id=$1", f.event.ID).
		Scan(&entries, &active); err != nil ||
		entries != 2 ||
		active != 1 {
		t.Fatalf("entry invariant: %d %d %v", entries, active, err)
	}
}

func TestPendingTeammatesHoldNoSeatsAndJoiningIsRevisionSafe(t *testing.T) {
	f := newRegistrationFixture(t, 2)
	ctx := t.Context()
	s := teams.Service{Pool: f.pool, Now: func() time.Time { return f.now }}
	captain, joiner, waiting := testdb.Person(
		t,
		f.pool,
	), testdb.Person(
		t,
		f.pool,
	), testdb.Person(
		t,
		f.pool,
	)
	for _, p := range []dbgen.Person{captain, joiner, waiting} {
		if _, err := f.service.Apply(ctx, p.ID, f.event.ID, f.application()); err != nil {
			t.Fatal(err)
		}
	}
	page := teamAct(
		t,
		s,
		captain.ID,
		f.event.ID,
		teams.Command{Action: "create", Name: "Captain team"},
	)
	team := page.Teams[0]
	results := make(chan error, 2)
	for _, p := range []dbgen.Person{joiner, waiting} {
		go func() {
			_, err := s.Act(
				ctx,
				p.ID,
				f.event.ID,
				page.Revision,
				teams.Command{Action: "join", Code: *team.Code},
			)
			results <- err
		}()
	}
	success, conflict := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			success++
		} else if errors.Is(err, access.ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("join race: %d/%d", success, conflict)
	}
	for _, p := range []dbgen.Person{joiner, waiting} {
		teamAct(t, s, p.ID, f.event.ID, teams.Command{Action: "join", Code: *team.Code})
	}
	page, err := s.Read(ctx, captain.ID, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range page.Requests {
		current, err := s.Read(ctx, captain.ID, f.event.ID)
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.Act(
			ctx,
			captain.ID,
			f.event.ID,
			current.Revision,
			teams.Command{Action: "approve", RequestID: r.ID},
		)
		if r.Person.ID == waiting.ID {
			if !errors.Is(err, access.ErrForbidden) {
				t.Fatalf("unconfirmed teammate assigned: %v", err)
			}
		} else if err != nil {
			t.Fatal(err)
		}
	}
	final, err := s.Read(ctx, joiner.ID, f.event.ID)
	if err != nil || len(final.Teams) != 1 || len(final.Teams[0].Draft) != 2 ||
		len(final.Teams[0].Published) != 0 {
		t.Fatalf("draft vs published: %+v %v", final, err)
	}
	count, err := dbgen.New(f.pool).ConfirmedAttendees(ctx, f.event.ID)
	if err != nil || count != 2 {
		t.Fatalf("pending request held seat: %d %v", count, err)
	}
	s.Now = func() time.Time { return f.event.Configuration.TeamFreeze }
	if _, err := s.Act(
		ctx,
		joiner.ID,
		f.event.ID,
		final.Revision,
		teams.Command{Action: "join", Code: *team.Code},
	); !errors.Is(
		err,
		teams.ErrFrozen,
	) {
		t.Fatalf("join at scheduled cutoff: %v", err)
	}
}
