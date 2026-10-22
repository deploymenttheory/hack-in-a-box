package mentors

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type coverageFixture struct {
	t       *testing.T
	pool    *pgxpool.Pool
	s       Service
	owner   dbgen.Person
	id      uuid.UUID
	mentors []dbgen.Person
	members []dbgen.Person
	teams   []uuid.UUID
	live    Window
}

func coverageDatabase(t *testing.T) *coverageFixture {
	t.Helper()
	pool := testdb.New(t, true)
	f := &coverageFixture{t: t, pool: pool, s: Service{Pool: pool}, owner: testdb.Person(t, pool)}
	f.id = testdb.Event(t, pool, f.owner.ID)
	c := event.DefaultConfiguration()
	c.LiveStart = time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	c.SubmissionCutoff = c.LiveEnd.Add(-time.Hour)
	f.live = Window{Start: c.LiveStart, End: c.LiveEnd}
	data, _ := json.Marshal(c)
	f.exec(
		"UPDATE events SET configuration=$2,stage='preparation',team_frozen_at=now() WHERE id=$1",
		f.id,
		data,
	)
	q := dbgen.New(pool)
	for range 2 {
		team := uuid.New()
		f.teams = append(f.teams, team)
		f.exec(
			"INSERT INTO hack_teams(id,event_id,name,state,frozen,allocation_locked) VALUES($1,$2,'Synthetic team','published',true,true)",
			team,
			f.id,
		)
		if err := q.CreateCompetitionEntry(
			t.Context(),
			dbgen.CreateCompetitionEntryParams{EventID: f.id, TeamID: team},
		); err != nil {
			t.Fatal(err)
		}
		for range 3 {
			person := testdb.Person(t, pool)
			f.members = append(f.members, person)
			f.exec(
				"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
				f.id,
				person.ID,
			)
			for _, snapshot := range []string{"draft", "published"} {
				f.exec(
					"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,$4)",
					f.id,
					team,
					person.ID,
					snapshot,
				)
			}
		}
	}
	for range 3 {
		mentor := testdb.Person(t, pool)
		f.mentors = append(f.mentors, mentor)
		f.exec(
			"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mentor')",
			f.id,
			mentor.ID,
		)
		if err := f.act(
			Command{
				Action:       "profile",
				PersonID:     mentor.ID,
				Availability: []Window{f.live},
				Skills:       []string{"design"},
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f *coverageFixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.t.Context(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *coverageFixture) revision() int64 {
	f.t.Helper()
	row, err := dbgen.New(f.pool).Event(f.t.Context(), f.id)
	if err != nil {
		f.t.Fatal(err)
	}
	return row.Revision
}

func (f *coverageFixture) act(c Command) error {
	f.t.Helper()
	c.Reason = "Synthetic organiser review"
	_, err := f.s.Change(f.t.Context(), f.owner.ID, f.id, f.revision(), c)
	return err
}

func (f *coverageFixture) assign(team, mentor int, slot string) error {
	return f.act(
		Command{
			Action:   "assign",
			TeamID:   f.teams[team],
			PersonID: f.mentors[mentor].ID,
			Slot:     slot,
			Window:   f.live,
		},
	)
}

func (f *coverageFixture) operationCount() int {
	f.t.Helper()
	var n int
	if err := f.pool.QueryRow(f.t.Context(), "SELECT count(*) FROM operations WHERE event_id=$1 AND kind='mentors.published'", f.id).
		Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func (f *coverageFixture) teamsState() string {
	f.t.Helper()
	q := dbgen.New(f.pool)
	teams, err := q.HackTeams(f.t.Context(), f.id)
	if err != nil {
		f.t.Fatal(err)
	}
	members, err := q.HackTeamMembers(f.t.Context(), f.id)
	if err != nil {
		f.t.Fatal(err)
	}
	data, _ := json.Marshal([]any{teams, members})
	return string(data)
}

func TestMentorDraftPublicationReplacementAndHistoryAfterCompetitorFreeze(t *testing.T) {
	f := coverageDatabase(t)
	before := f.teamsState()
	ctx := t.Context()
	q := dbgen.New(f.pool)
	policy := access.EventPolicy{Queries: q}
	for _, a := range []struct {
		team, mentor int
		slot         string
	}{{0, 0, "primary"}, {0, 1, "second"}, {1, 1, "primary"}, {1, 2, "second"}} {
		if err := f.assign(a.team, a.mentor, a.slot); err != nil {
			t.Fatal(err)
		}
	}
	if f.operationCount() != 0 {
		t.Fatal("drafts emitted publication operations")
	}
	if err := policy.Authorize(
		ctx,
		f.mentors[0].ID,
		f.id,
		f.teams[0],
		access.TeamRead,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatalf("draft leaked team access: %v", err)
	}
	if err := f.act(Command{Action: "publish"}); err != nil {
		t.Fatal(err)
	}
	if f.operationCount() != 1 {
		t.Fatal("publication did not emit exactly one change")
	}
	if err := policy.Authorize(
		ctx,
		f.mentors[0].ID,
		f.id,
		f.teams[0],
		access.TeamRead,
	); err != nil {
		t.Fatal(err)
	}
	capacityRevision := f.revision()
	if err := f.act(Command{Action: "settings", DefaultCapacity: 1}); !errors.Is(err, ErrCoverage) {
		t.Fatalf("published workload silently exceeded new capacity: %v", err)
	}
	if f.revision() != capacityRevision {
		t.Fatal("rejected capacity change advanced revision")
	}
	if err := audit.Append(
		ctx,
		q,
		audit.Record{
			ActorID:  &f.mentors[0].ID,
			EventID:  &f.id,
			TargetID: f.mentors[0].ID,
			Action:   audit.SignedIn,
			Reason:   audit.UserRequest,
		},
	); err != nil {
		t.Fatal(err)
	}
	// Replacement remains a draft until explicit publication; old access then disappears.
	if err := f.assign(0, 2, "primary"); err != nil {
		t.Fatal(err)
	}
	if err := policy.Authorize(
		ctx,
		f.mentors[0].ID,
		f.id,
		f.teams[0],
		access.TeamRead,
	); err != nil {
		t.Fatal("draft revoked published access")
	}
	var auditsBefore int
	f.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE event_id=$1", f.id).
		Scan(&auditsBefore)
	if err := f.act(Command{Action: "publish"}); err != nil {
		t.Fatal(err)
	}
	if err := policy.Authorize(
		ctx,
		f.mentors[0].ID,
		f.id,
		f.teams[0],
		access.TeamRead,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatalf("old mentor retained access: %v", err)
	}
	if err := policy.Authorize(
		ctx,
		f.mentors[2].ID,
		f.id,
		f.teams[0],
		access.TeamRead,
	); err != nil {
		t.Fatal(err)
	}
	var auditsAfter int
	f.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE event_id=$1", f.id).
		Scan(&auditsAfter)
	var authored int
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE event_id=$1 AND actor_id=$2 AND action='identity.signed_in'", f.id, f.mentors[0].ID).
		Scan(&authored); err != nil ||
		authored != 1 {
		t.Fatalf("former mentor's authored history was lost: %d %v", authored, err)
	}
	if auditsAfter != auditsBefore+1 {
		t.Fatal("reassignment lost audit history")
	}
	if err := f.act(Command{Action: "publish"}); err != nil {
		t.Fatal(err)
	}
	if f.operationCount() != 2 {
		t.Fatal("unchanged publication duplicated delivery")
	}
	if f.teamsState() != before {
		t.Fatal("mentor replacement moved competitors or reopened freeze")
	}
}

func TestMentorEditsRejectCapacityConflictsAndUnavailableSelections(t *testing.T) {
	f := coverageDatabase(t)
	one := 1
	if err := f.act(Command{Action: "settings", DefaultCapacity: 1}); err != nil {
		t.Fatal(err)
	}
	if err := f.assign(0, 0, "primary"); err != nil {
		t.Fatal(err)
	}
	if err := f.assign(1, 0, "primary"); !errors.Is(err, ErrCoverage) {
		t.Fatalf("event mentor capacity ignored: %v", err)
	}
	if err := f.assign(0, 0, "second"); !errors.Is(err, ErrCoverage) {
		t.Fatalf("same primary/second assigned: %v", err)
	}
	half := Window{Start: f.live.Start, End: f.live.Start.Add(4 * time.Hour)}
	if err := f.act(
		Command{
			Action:       "profile",
			PersonID:     f.mentors[1].ID,
			Availability: []Window{half},
			MaximumTeams: &one,
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := f.assign(1, 1, "primary"); !errors.Is(err, ErrCoverage) {
		t.Fatalf("partial availability treated as full day: %v", err)
	}
	if err := f.act(
		Command{
			Action:   "assign",
			TeamID:   f.teams[1],
			PersonID: f.mentors[1].ID,
			Slot:     "primary",
			Window:   half,
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := f.act(Command{Action: "publish"}); !errors.Is(err, ErrCoverage) {
		t.Fatalf("coverage gaps published without approval: %v", err)
	}
	if err := f.act(Command{Action: "publish", AllowGaps: true}); err != nil {
		t.Fatal(err)
	}
	if err := f.act(
		Command{
			Action:       "conflict",
			PersonID:     f.mentors[2].ID,
			CompetitorID: f.members[0].ID,
			Kind:         "reporting",
			Enabled:      true,
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := f.assign(0, 2, "second"); !errors.Is(err, ErrCoverage) {
		t.Fatalf("direct report assigned: %v", err)
	}
	f.exec(
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'draft')",
		f.id,
		f.teams[0],
		f.mentors[2].ID,
	)
	if err := f.assign(0, 2, "second"); !errors.Is(err, ErrCoverage) {
		t.Fatalf("own team assigned: %v", err)
	}
}

func TestMentorChangesRejectStaleForeignAndUnprivilegedCommands(t *testing.T) {
	f := coverageDatabase(t)
	revision := f.revision()
	command := Command{Action: "settings", DefaultCapacity: 3, Reason: "Synthetic change"}
	if err := f.act(command); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Change(
		t.Context(),
		f.owner.ID,
		f.id,
		revision,
		command,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatalf("stale update accepted: %v", err)
	}
	for _, actor := range []uuid.UUID{f.mentors[0].ID, f.members[0].ID, testdb.Person(t, f.pool).ID} {
		if _, err := f.s.Change(
			t.Context(),
			actor,
			f.id,
			f.revision(),
			command,
		); !errors.Is(
			err,
			access.ErrForbidden,
		) {
			t.Fatalf("nonorganiser changed coverage: %v", err)
		}
	}
	other := testdb.Person(t, f.pool)
	if err := f.act(
		Command{Action: "profile", PersonID: other.ID, Availability: []Window{f.live}},
	); !errors.Is(
		err,
		access.ErrInvalid,
	) {
		t.Fatalf("foreign mentor profile accepted: %v", err)
	}
	page, err := f.s.Read(t.Context(), f.owner.ID, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.act(Command{Action: "apply_suggestion"}); err != nil {
		t.Fatal(err)
	}
	updated, err := f.s.Read(t.Context(), f.owner.ID, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Draft) == 0 || reflect.DeepEqual(page.Draft, updated.Draft) ||
		f.operationCount() != 0 {
		t.Fatal("explicit suggestion application failed or auto-published")
	}
}

func TestMentorConflictRemovalRemainsScopedAfterWithdrawal(t *testing.T) {
	f := coverageDatabase(t)
	c := Command{
		Action:       "conflict",
		PersonID:     f.mentors[0].ID,
		CompetitorID: f.members[0].ID,
		Kind:         "reporting",
		Enabled:      true,
	}
	if err := f.act(c); err != nil {
		t.Fatal(err)
	}
	f.exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		f.id,
		f.members[0].ID,
	)
	c.Enabled = false
	if err := f.act(c); err != nil {
		t.Fatalf("could not remove stale conflict after withdrawal: %v", err)
	}
	c.CompetitorID = testdb.Person(t, f.pool).ID
	if err := f.act(c); !errors.Is(err, access.ErrInvalid) {
		t.Fatalf("foreign conflict removal accepted: %v", err)
	}
}
