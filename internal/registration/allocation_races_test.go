package registration

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
	"github.com/deploymenttheory/hack-in-a-box/internal/permissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/teams"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type allocationLifecycle struct {
	registrationFixture
	teams         teams.Service
	people        []dbgen.Person
	registrations []View
	groups        []uuid.UUID
}

// Register real confirmed competitors, publish two cohorts, and take the go
// decision through the application services before exercising cross-domain races.
func newAllocationLifecycle(t *testing.T, size int) *allocationLifecycle {
	t.Helper()
	f := &allocationLifecycle{registrationFixture: newRegistrationFixture(t, size*2)}
	f.teams = teams.Service{Pool: f.pool, Now: func() time.Time { return f.now }}
	for range size * 2 {
		person := testdb.Person(t, f.pool)
		registration, err := f.service.Apply(t.Context(), person.ID, f.event.ID, f.application())
		if err != nil {
			t.Fatal(err)
		}
		f.people = append(f.people, person)
		f.registrations = append(f.registrations, registration)
	}
	for group := range 2 {
		captain := group * size
		page := teamAct(
			t,
			f.teams,
			f.people[captain].ID,
			f.event.ID,
			teams.Command{Action: "create", Name: fmt.Sprintf("Cohort %d", group)},
		)
		id := page.Teams[0].ID
		f.groups = append(f.groups, id)
		for i := captain + 1; i < captain+size; i++ {
			teamAct(
				t,
				f.teams,
				f.owner.ID,
				f.event.ID,
				teams.Command{Action: "move", PersonID: f.people[i].ID, TeamID: id},
			)
		}
	}
	teamAct(t, f.teams, f.owner.ID, f.event.ID, teams.Command{Action: "publish"})
	events := event.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	row, err := events.Read(t.Context(), f.owner.ID, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = events.Transition(
		t.Context(),
		f.owner.ID,
		f.event.ID,
		row.Revision,
		"preparation",
		"",
		false,
	); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *allocationLifecycle) revision(t *testing.T) int64 {
	t.Helper()
	row, err := dbgen.New(f.pool).Event(t.Context(), f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	return row.Revision
}

func (f *allocationLifecycle) grant(t *testing.T, person uuid.UUID, role string) {
	t.Helper()
	_, err := (permissions.Service{Pool: f.pool}).Change(
		t.Context(),
		permissions.Change{
			Actor:                             f.owner.ID,
			Event:                             f.event.ID,
			Person:                            person,
			Role:                              role,
			Active:                            true,
			AcknowledgeCompetitionRestriction: true,
			Revision:                          f.revision(t),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
}

func (f *allocationLifecycle) operationCount(t *testing.T, kind string) int {
	t.Helper()
	var count int
	if err := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM operations WHERE event_id=$1 AND kind=$2", f.event.ID, kind).
		Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func (f *allocationLifecycle) authority(t *testing.T, person, team uuid.UUID) access.TeamAuthority {
	t.Helper()
	a, err := access.TeamPermissions(t.Context(), dbgen.New(f.pool), person, f.event.ID, team)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func (f *allocationLifecycle) withdraw(t *testing.T, index int) {
	t.Helper()
	r := f.registrations[index]
	if _, err := f.service.Respond(
		t.Context(),
		f.people[index].ID,
		f.event.ID,
		r.ID,
		r.Revision,
		"withdraw",
	); err != nil {
		t.Fatal(err)
	}
}

func simultaneous(actions ...func() error) []error {
	start := make(chan struct{})
	results := make(chan struct {
		index int
		err   error
	}, len(actions))
	for i, action := range actions {
		go func() {
			<-start
			results <- struct {
				index int
				err   error
			}{i, action()}
		}()
	}
	close(start)
	out := make([]error, len(actions))
	for range actions {
		r := <-results
		out[r.index] = r.err
	}
	return out
}

func oneReviewedWinner(t *testing.T, results []error) {
	t.Helper()
	success, conflict := 0, 0
	for _, err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, access.ErrConflict) {
			conflict++
		} else {
			t.Fatalf("unexpected concurrent outcome: %v", err)
		}
	}
	if success != 1 || conflict != len(results)-1 {
		t.Fatalf("expected one committed review, got %d successes/%d conflicts", success, conflict)
	}
}

func TestTwoOrganiserPublicationRaceWithRealRegistrations(t *testing.T) {
	f := newAllocationLifecycle(t, 4)
	ctx := t.Context()
	other := testdb.Person(t, f.pool)
	f.grant(t, other.ID, "organiser")
	teamAct(
		t,
		f.teams,
		f.owner.ID,
		f.event.ID,
		teams.Command{Action: "unlock", TeamID: f.groups[0]},
	)
	teamAct(
		t,
		f.teams,
		f.owner.ID,
		f.event.ID,
		teams.Command{Action: "move", PersonID: f.people[1].ID, TeamID: f.groups[1]},
	)
	if !f.authority(t, f.people[1].ID, f.groups[0]).Member ||
		f.authority(t, f.people[1].ID, f.groups[1]).Member {
		t.Fatal("unpublished move changed participant access")
	}
	before := f.operationCount(t, "teams.published")
	revision := f.revision(t)
	actions := []func() error{}
	for _, actor := range []uuid.UUID{f.owner.ID, other.ID} {
		actions = append(actions, func() error {
			_, err := f.teams.Act(
				ctx,
				actor,
				f.event.ID,
				revision,
				teams.Command{Action: "publish", Reason: "Reviewed the current allocation"},
			)
			return err
		})
	}
	oneReviewedWinner(t, simultaneous(actions...))
	if f.operationCount(t, "teams.published") != before+1 {
		t.Fatal("duplicate or missing publication delivery intent")
	}
	if f.authority(t, f.people[1].ID, f.groups[0]).Member ||
		!f.authority(t, f.people[1].ID, f.groups[1]).Member {
		t.Fatal("publication failed to replace access atomically")
	}
	var invalid int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM hack_team_members m LEFT JOIN competition_claims c ON c.event_id=m.event_id AND c.person_id=m.person_id WHERE m.event_id=$1 AND c.state IS DISTINCT FROM 'confirmed'`, f.event.ID).
		Scan(&invalid); err != nil ||
		invalid != 0 {
		t.Fatalf("unconfirmed assigned competitor: %d %v", invalid, err)
	}
	page, err := f.teams.Read(ctx, f.owner.ID, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	sizes := map[int]int{}
	for _, team := range page.Teams {
		sizes[len(team.Published)]++
	}
	if sizes[3] != 1 || sizes[5] != 1 {
		t.Fatalf("explicit valid exception to preferred size was lost: %v", sizes)
	}
}

func TestFreezeSerializesWithManualMovesAndQueuedPublication(t *testing.T) {
	for _, action := range []string{"move", "publish"} {
		t.Run(action, func(t *testing.T) {
			f := newAllocationLifecycle(t, 4)
			ctx := t.Context()
			teamAct(
				t,
				f.teams,
				f.owner.ID,
				f.event.ID,
				teams.Command{Action: "unlock", TeamID: f.groups[0]},
			)
			revision := f.revision(t)
			command := teams.Command{
				Action:   action,
				PersonID: f.people[1].ID,
				TeamID:   f.groups[1],
				Reason:   "Review queued before freeze",
			}
			oneReviewedWinner(t, simultaneous(func() error {
				_, err := f.teams.Act(
					ctx,
					f.owner.ID,
					f.event.ID,
					revision,
					teams.Command{Action: "freeze", Reason: "Reviewed freeze"},
				)
				return err
			}, func() error { _, err := f.teams.Act(ctx, f.owner.ID, f.event.ID, revision, command); return err }))
			page, err := f.teams.Read(ctx, f.owner.ID, f.event.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !page.Frozen {
				page = teamAct(t, f.teams, f.owner.ID, f.event.ID, teams.Command{Action: "freeze"})
			}
			before, err := dbgen.New(f.pool).HackTeamMembers(ctx, f.event.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, queued := range []int64{revision, page.Revision} {
				_, err = f.teams.Act(ctx, f.owner.ID, f.event.ID, queued, command)
				if queued == revision && !errors.Is(err, access.ErrConflict) {
					t.Fatalf("stale queued action: %v", err)
				}
				if queued == page.Revision && !errors.Is(err, teams.ErrFrozen) {
					t.Fatalf("fresh post-freeze action: %v", err)
				}
			}
			after, err := dbgen.New(f.pool).HackTeamMembers(ctx, f.event.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("rejected queued action changed frozen membership")
			}
		})
	}
}

func TestFreezeRacesActualWithdrawalWithoutMovingSurvivors(t *testing.T) {
	f := newAllocationLifecycle(t, 3)
	ctx := t.Context()
	revision := f.revision(t)
	r := f.registrations[1]
	results := simultaneous(func() error {
		_, err := f.teams.Act(
			ctx,
			f.owner.ID,
			f.event.ID,
			revision,
			teams.Command{Action: "freeze", Reason: "Current confirmed cohort"},
		)
		return err
	}, func() error {
		_, err := f.service.Respond(ctx, f.people[1].ID, f.event.ID, r.ID, r.Revision, "withdraw")
		return err
	})
	if results[1] != nil {
		t.Fatal(results[1])
	}
	if results[0] != nil && !errors.Is(results[0], access.ErrConflict) {
		t.Fatalf("freeze outcome: %v", results[0])
	}
	page, err := f.teams.Read(ctx, f.owner.ID, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	if page.Frozen != (results[0] == nil) {
		t.Fatal("freeze result differs from committed state")
	}
	if f.authority(t, f.people[1].ID, f.groups[0]).Member {
		t.Fatal("withdrawn competitor retained access")
	}
	for _, index := range []int{0, 2} {
		if !f.authority(t, f.people[index].ID, f.groups[0]).Member ||
			f.authority(t, f.people[index].ID, f.groups[1]).Member {
			t.Fatal("withdrawal moved a survivor")
		}
	}
	if !page.Frozen {
		_, err = f.teams.Act(
			ctx,
			f.owner.ID,
			f.event.ID,
			page.Revision,
			teams.Command{Action: "freeze", Reason: "Fresh depleted cohort review"},
		)
		if !errors.Is(err, teams.ErrUnready) {
			t.Fatalf("pre-freeze team below three was not remediated: %v", err)
		}
	} else {
		if !f.authority(t, f.people[0].ID, f.groups[0]).Active {
			t.Fatal("post-freeze depleted team cannot continue")
		}
	}
}

func TestUnderThreeRemediationRetiresEntryAndPreservesRepositoryReference(t *testing.T) {
	f := newAllocationLifecycle(t, 3)
	ctx := t.Context()
	// A private preparation artifact is historical evidence, not a formal release.
	// Its repository reference stays with the retired team; no transfer is required.
	reference := `[{"kind":"url","label":"Retired team repository","url":"https://github.com/synthetic/unused-team"}]`
	if _, err := f.pool.Exec(
		ctx,
		`INSERT INTO submission_drafts(event_id,team_id,revision,actor_id,updated_at,evidence) VALUES($1,$2,1,$3,$4,$5)`,
		f.event.ID,
		f.groups[0],
		f.people[0].ID,
		f.now,
		reference,
	); err != nil {
		t.Fatal(err)
	}
	f.withdraw(t, 0)
	teamAct(
		t,
		f.teams,
		f.owner.ID,
		f.event.ID,
		teams.Command{Action: "unlock", TeamID: f.groups[0]},
	)
	for _, index := range []int{1, 2} {
		teamAct(
			t,
			f.teams,
			f.owner.ID,
			f.event.ID,
			teams.Command{Action: "move", TeamID: f.groups[1], PersonID: f.people[index].ID},
		)
	}
	page := teamAct(t, f.teams, f.owner.ID, f.event.ID, teams.Command{Action: "freeze"})
	if !page.Frozen {
		t.Fatal("remediated cohort not frozen")
	}
	authority := f.authority(t, f.people[1].ID, f.groups[0])
	if authority.Active || authority.Member {
		t.Fatal("retired entry remains eligible")
	}
	var retained string
	if err := f.pool.QueryRow(ctx, "SELECT evidence->0->>'url' FROM submission_drafts WHERE event_id=$1 AND team_id=$2", f.event.ID, f.groups[0]).
		Scan(&retained); err != nil ||
		retained != "https://github.com/synthetic/unused-team" {
		t.Fatalf("retired repository reference was lost: %s %v", retained, err)
	}
	var transferred int
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM submission_drafts WHERE event_id=$1 AND team_id=$2", f.event.ID, f.groups[1]).
		Scan(&transferred); err != nil ||
		transferred != 0 {
		t.Fatalf("silently transferred discarded work: %d %v", transferred, err)
	}
	submission := submissions.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	if _, err := submission.Read(
		ctx,
		f.people[1].ID,
		f.event.ID,
		f.groups[0],
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatalf("retired team's work still accessible: %v", err)
	}
	f.withdraw(t, 1)
	f.withdraw(t, 2)
	f.withdraw(t, 3)
	remaining, err := f.teams.Read(ctx, f.people[4].ID, f.event.ID)
	if err != nil || len(remaining.Teams) != 1 || len(remaining.Teams[0].Published) != 2 {
		t.Fatalf("depleted frozen team did not continue in place: %+v %v", remaining, err)
	}
	var active int
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM competition_entries WHERE event_id=$1 AND active", f.event.ID).
		Scan(&active); err != nil ||
		active != 1 {
		t.Fatalf("entry cardinality: %d %v", active, err)
	}
	submission.Clock = func() time.Time { return f.event.Configuration.SubmissionCutoff.Add(time.Second) }
	if err = submission.Maintain(ctx); err != nil {
		t.Fatal(err)
	}
	state, err := dbgen.New(f.pool).
		SubmissionState(ctx, dbgen.SubmissionStateParams{EventID: f.event.ID, TeamID: f.groups[0]})
	if err != nil || state.Outcome != "withdrawn" || state.ActiveRevision.Valid {
		t.Fatalf("retired repository contributed scored evidence: %+v %v", state, err)
	}
}

func TestMentorPublicationAndConflictSerializeAfterCompetitorFreeze(t *testing.T) {
	f := newAllocationLifecycle(t, 3)
	ctx := t.Context()
	teamAct(t, f.teams, f.owner.ID, f.event.ID, teams.Command{Action: "freeze"})
	mentor := testdb.Person(t, f.pool)
	f.grant(t, mentor.ID, "mentor")
	service := mentors.Service{Pool: f.pool}
	window := mentors.Window{
		Start: f.event.Configuration.LiveStart,
		End:   f.event.Configuration.LiveEnd,
	}
	change := func(command mentors.Command) {
		t.Helper()
		command.Reason = "Reviewed mentor coverage"
		if _, err := service.Change(
			ctx,
			f.owner.ID,
			f.event.ID,
			f.revision(t),
			command,
		); err != nil {
			t.Fatal(err)
		}
	}
	change(
		mentors.Command{
			Action:       "profile",
			PersonID:     mentor.ID,
			Availability: []mentors.Window{window},
		},
	)
	change(
		mentors.Command{
			Action:   "assign",
			TeamID:   f.groups[0],
			PersonID: mentor.ID,
			Slot:     "primary",
			Window:   window,
		},
	)
	revision := f.revision(t)
	results := simultaneous(func() error {
		_, err := service.Change(
			ctx,
			f.owner.ID,
			f.event.ID,
			revision,
			mentors.Command{
				Action:    "publish",
				AllowGaps: true,
				Reason:    "Recorded uncovered secondary slots",
			},
		)
		return err
	}, func() error {
		_, err := service.Change(
			ctx,
			f.owner.ID,
			f.event.ID,
			revision,
			mentors.Command{
				Action:       "conflict",
				PersonID:     mentor.ID,
				CompetitorID: f.people[0].ID,
				Kind:         "reporting",
				Enabled:      true,
				Reason:       "Declared reporting relationship",
			},
		)
		return err
	})
	oneReviewedWinner(t, results)
	if results[1] != nil {
		change(
			mentors.Command{
				Action:       "conflict",
				PersonID:     mentor.ID,
				CompetitorID: f.people[0].ID,
				Kind:         "reporting",
				Enabled:      true,
			},
		)
	}
	if f.authority(t, mentor.ID, f.groups[0]).Mentor {
		t.Fatal("known conflict retained mentor authority")
	}
	if _, err := service.Change(
		ctx,
		f.owner.ID,
		f.event.ID,
		f.revision(t),
		mentors.Command{Action: "publish", AllowGaps: true, Reason: "Attempted conflict waiver"},
	); !errors.Is(
		err,
		mentors.ErrCoverage,
	) {
		t.Fatalf("coverage exception waived a hard conflict: %v", err)
	}
	for i, p := range f.people {
		expected := f.groups[i/3]
		if !f.authority(t, p.ID, expected).Member {
			t.Fatal("mentor replacement changed frozen competitors")
		}
	}
}

func TestAllocationProposalsDoNotReachParticipantOrDeliveryState(t *testing.T) {
	f := newAllocationLifecycle(t, 4)
	ctx := t.Context()
	for _, team := range f.groups {
		teamAct(t, f.teams, f.owner.ID, f.event.ID, teams.Command{Action: "unlock", TeamID: team})
	}
	before, err := f.teams.Read(ctx, f.people[1].ID, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	var operationsBefore int
	if err = f.pool.QueryRow(ctx, "SELECT count(*) FROM operations WHERE event_id=$1", f.event.ID).
		Scan(&operationsBefore); err != nil {
		t.Fatal(err)
	}
	proposal, err := f.teams.Suggest(
		ctx,
		f.owner.ID,
		f.event.ID,
		f.revision(t),
		teams.AllocationRequest{Seed: "review-only"},
	)
	if err != nil || len(proposal.Teams) != 2 {
		t.Fatalf("proposal: %+v %v", proposal, err)
	}
	for _, team := range proposal.Teams {
		if len(team.Members) != 4 {
			t.Fatalf("preferred 4+4 allocation: %+v", proposal)
		}
	}
	after, err := f.teams.Read(ctx, f.people[1].ID, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("unpublished proposal changed participant snapshot")
	}
	var operationsAfter int
	if err = f.pool.QueryRow(ctx, "SELECT count(*) FROM operations WHERE event_id=$1", f.event.ID).
		Scan(&operationsAfter); err != nil ||
		operationsBefore != operationsAfter {
		t.Fatalf(
			"proposal emitted notification/provisioning intent: %d -> %d %v",
			operationsBefore,
			operationsAfter,
			err,
		)
	}
}

func TestMentorReplacementRacesActualWithdrawalAfterFreeze(t *testing.T) {
	f := newAllocationLifecycle(t, 3)
	ctx := t.Context()
	teamAct(t, f.teams, f.owner.ID, f.event.ID, teams.Command{Action: "freeze"})
	oldMentor, replacement := testdb.Person(t, f.pool), testdb.Person(t, f.pool)
	service := mentors.Service{Pool: f.pool}
	window := mentors.Window{
		Start: f.event.Configuration.LiveStart,
		End:   f.event.Configuration.LiveEnd,
	}
	change := func(command mentors.Command) {
		t.Helper()
		command.Reason = "Reviewed mentor replacement"
		if _, err := service.Change(
			ctx,
			f.owner.ID,
			f.event.ID,
			f.revision(t),
			command,
		); err != nil {
			t.Fatal(err)
		}
	}
	for _, person := range []dbgen.Person{oldMentor, replacement} {
		f.grant(t, person.ID, "mentor")
		change(
			mentors.Command{
				Action:       "profile",
				PersonID:     person.ID,
				Availability: []mentors.Window{window},
			},
		)
	}
	change(
		mentors.Command{
			Action:   "assign",
			TeamID:   f.groups[0],
			PersonID: oldMentor.ID,
			Slot:     "primary",
			Window:   window,
		},
	)
	change(mentors.Command{Action: "publish", AllowGaps: true})
	change(
		mentors.Command{
			Action:   "assign",
			TeamID:   f.groups[0],
			PersonID: replacement.ID,
			Slot:     "primary",
			Window:   window,
		},
	)
	if !f.authority(t, oldMentor.ID, f.groups[0]).Mentor ||
		f.authority(t, replacement.ID, f.groups[0]).Mentor {
		t.Fatal("unpublished replacement changed mentor access")
	}
	revision := f.revision(t)
	registration := f.registrations[1]
	results := simultaneous(func() error {
		_, err := service.Change(
			ctx,
			f.owner.ID,
			f.event.ID,
			revision,
			mentors.Command{
				Action:    "publish",
				AllowGaps: true,
				Reason:    "Reviewed replacement with explicit coverage gaps",
			},
		)
		return err
	}, func() error {
		_, err := f.service.Respond(
			ctx,
			f.people[1].ID,
			f.event.ID,
			registration.ID,
			registration.Revision,
			"withdraw",
		)
		return err
	})
	if results[1] != nil {
		t.Fatal(results[1])
	}
	if results[0] != nil {
		if !errors.Is(results[0], access.ErrConflict) {
			t.Fatalf("replacement race: %v", results[0])
		}
		// The organiser must review the new roster and submit its new revision.
		change(mentors.Command{Action: "publish", AllowGaps: true})
	}
	if f.authority(t, oldMentor.ID, f.groups[0]).Mentor ||
		!f.authority(t, replacement.ID, f.groups[0]).Mentor {
		t.Fatal("published replacement failed to revoke old access and grant new access")
	}
	if f.authority(t, f.people[1].ID, f.groups[0]).Member {
		t.Fatal("withdrawn competitor retained access")
	}
	for i, person := range f.people {
		if i == 1 {
			continue
		}
		if !f.authority(t, person.ID, f.groups[i/3]).Member {
			t.Fatal("mentor replacement or withdrawal moved a frozen survivor")
		}
	}
	page, err := f.teams.Read(ctx, f.owner.ID, f.event.ID)
	if err != nil || !page.Frozen {
		t.Fatalf("replacement changed freeze: %+v %v", page, err)
	}
}
