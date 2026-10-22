package registration

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/teams"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestInvitationCodeCannotCrossEventsEvenForConfirmedCompetitor(t *testing.T) {
	f := newRegistrationFixture(t, 3)
	ctx := t.Context()
	events := event.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	other, err := events.Create(
		ctx,
		f.owner.ID,
		event.Input{
			ID:                                uuid.New(),
			Name:                              "Other synthetic hack",
			Slug:                              "other-" + uuid.NewString(),
			Configuration:                     f.event.Configuration,
			AcknowledgeCompetitionRestriction: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	form, err := f.service.SaveForm(ctx, f.owner.ID, other.ID, 0, f.form)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.PublishForm(
		ctx,
		f.owner.ID,
		other.ID,
		f.form.ID,
		form.Revision,
		form.Version,
		true,
	); err != nil {
		t.Fatal(err)
	}
	other, err = events.Read(ctx, f.owner.ID, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	other, err = events.Transition(
		ctx,
		f.owner.ID,
		other.ID,
		other.Revision,
		"registration",
		"",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	captain, joiner := testdb.Person(t, f.pool), testdb.Person(t, f.pool)
	for _, id := range []uuid.UUID{f.event.ID, other.ID} {
		for _, p := range []dbgen.Person{captain, joiner} {
			view, applyErr := f.service.Apply(ctx, p.ID, id, f.application())
			if applyErr != nil || view.State != "confirmed" {
				t.Fatalf("confirmed fixture: %+v %v", view, applyErr)
			}
		}
	}
	service := teams.Service{Pool: f.pool, Now: func() time.Time { return f.now }}
	first := teamAct(t, service, captain.ID, f.event.ID, teams.Command{Action: "create", Name: "Duplicated private team"}).Teams[0]
	second := teamAct(t, service, captain.ID, other.ID, teams.Command{Action: "create", Name: "Duplicated private team"}).Teams[0]
	before, err := service.Read(ctx, joiner.ID, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Teams) != 0 || len(before.People) != 0 {
		t.Fatalf("private roster exposed: %+v", before)
	}
	type facts struct{ Members, Requests, Audit, Operations int }
	snapshot := func(id uuid.UUID) facts {
		t.Helper()
		var v facts
		err := f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM hack_team_members WHERE event_id=$1),(SELECT count(*) FROM hack_team_requests WHERE event_id=$1),(SELECT count(*) FROM audit_events WHERE event_id=$1),(SELECT count(*) FROM operations WHERE event_id=$1)`, id).
			Scan(&v.Members, &v.Requests, &v.Audit, &v.Operations)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	sourceBefore, targetBefore := snapshot(f.event.ID), snapshot(other.ID)
	if _, err = service.Act(
		ctx,
		joiner.ID,
		other.ID,
		before.Revision,
		teams.Command{Action: "join", Code: *first.Code},
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatalf("foreign invitation was accepted: %v", err)
	}
	after, err := service.Read(ctx, joiner.ID, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) || sourceBefore != snapshot(f.event.ID) ||
		targetBefore != snapshot(other.ID) {
		t.Fatal(
			"failed foreign invitation changed membership, revision, requests, audit or delivery",
		)
	}
	// The same actor can request the correct event's opaque code, but a request
	// neither allocates a second seat nor assigns/publishes team membership.
	requested := teamAct(
		t,
		service,
		joiner.ID,
		other.ID,
		teams.Command{Action: "join", Code: *second.Code},
	)
	if len(requested.Requests) != 1 || len(requested.Teams) != 0 {
		t.Fatalf("request exposed or assigned private roster: %+v", requested)
	}
	current := snapshot(other.ID)
	if current.Members != targetBefore.Members {
		t.Fatal("join request assigned membership")
	}
	for _, id := range []uuid.UUID{f.event.ID, other.ID} {
		count, readErr := dbgen.New(f.pool).ConfirmedAttendees(ctx, id)
		if readErr != nil || count != 2 {
			t.Fatalf("request changed confirmed seats: %d %v", count, readErr)
		}
	}
}
