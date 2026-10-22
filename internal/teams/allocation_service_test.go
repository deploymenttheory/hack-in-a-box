package teams

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type allocationDB struct {
	t      *testing.T
	pool   *pgxpool.Pool
	q      *dbgen.Queries
	s      Service
	owner  dbgen.Person
	id     uuid.UUID
	people []dbgen.Person
	groups []uuid.UUID
	now    time.Time
	c      event.Configuration
}

func allocationDatabase(t *testing.T, pool *pgxpool.Pool, sizes ...int) *allocationDB {
	t.Helper()
	f := &allocationDB{t: t, pool: pool, q: dbgen.New(pool), owner: testdb.Person(t, pool), now: time.Date(2027, 6, 1, 9, 0, 0, 0, time.UTC)}
	f.id = testdb.Event(t, pool, f.owner.ID)
	f.s = Service{Pool: pool, Now: func() time.Time { return f.now }}
	f.c = event.DefaultConfiguration()
	f.c.MinimumAttendance = 3
	f.c.LiveStart = f.now.AddDate(0, 0, 10)
	f.c.LiveEnd = f.c.LiveStart.Add(8 * time.Hour)
	f.c.SubmissionCutoff = f.c.LiveStart.Add(7 * time.Hour)
	f.c.TeamFreeze = f.now.Add(24 * time.Hour)
	f.c.AdmissionCutoff = f.now.Add(12 * time.Hour)
	data, _ := json.Marshal(f.c)
	f.exec("UPDATE events SET stage='preparation',configuration=$2 WHERE id=$1", f.id, data)
	for _, size := range sizes {
		id := uuid.New()
		f.groups = append(f.groups, id)
		members := make([]dbgen.Person, size)
		for i := range members {
			members[i] = testdb.Person(t, pool)
			f.people = append(f.people, members[i])
			f.exec("INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')", f.id, members[i].ID)
		}
		state := "draft"
		if size >= 3 {
			state = "published"
		}
		f.exec("INSERT INTO hack_teams(id,event_id,name,captain_id,published_captain_id,state,allocation_locked) VALUES($1,$2,'Synthetic team',$3,$3,$4,$5)", id, f.id, members[0].ID, state, size >= 3)
		for _, person := range members {
			f.exec("INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'draft')", f.id, id, person.ID)
			if size >= 3 {
				f.exec("INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')", f.id, id, person.ID)
			}
		}
	}
	return f
}
func (f *allocationDB) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.t.Context(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}
func (f *allocationDB) revision() int64 {
	f.t.Helper()
	r, err := f.q.Event(f.t.Context(), f.id)
	if err != nil {
		f.t.Fatal(err)
	}
	return r.Revision
}
func (f *allocationDB) act(c Command) error {
	f.t.Helper()
	c.Reason = "Synthetic organiser review"
	_, err := f.s.Act(f.t.Context(), f.owner.ID, f.id, f.revision(), c)
	return err
}
func (f *allocationDB) state() string {
	f.t.Helper()
	members, err := f.q.HackTeamMembers(f.t.Context(), f.id)
	if err != nil {
		f.t.Fatal(err)
	}
	groups, err := f.q.HackTeams(f.t.Context(), f.id)
	if err != nil {
		f.t.Fatal(err)
	}
	var operations, audits int
	if err = f.pool.QueryRow(f.t.Context(), "SELECT (SELECT count(*) FROM operations WHERE event_id=$1),(SELECT count(*) FROM audit_events WHERE event_id=$1)", f.id).Scan(&operations, &audits); err != nil {
		f.t.Fatal(err)
	}
	data, _ := json.Marshal(struct {
		Revision           int64
		Members            []dbgen.HackTeamMembersRow
		Groups             []dbgen.HackTeam
		Operations, Audits int
	}{f.revision(), members, groups, operations, audits})
	return string(data)
}

func TestAllocationServiceCurrentRosterPrivacyAndNoSideEffects(t *testing.T) {
	pool := testdb.New(t, true)
	f := allocationDatabase(t, pool, 6, 2)
	ctx := t.Context()
	six := 6
	in := AllocationRequest{Seed: "reviewed", Limits: AllocationLimits{MaximumSize: &six}, Declarations: []AllocationCandidate{{ID: f.people[0].ID, Skills: []string{"Design"}, Experience: 3, PreferWith: []uuid.UUID{f.people[6].ID}}}}
	before := f.state()
	out, err := f.s.Suggest(ctx, f.owner.ID, f.id, f.revision(), in)
	if err != nil || len(out.Unresolved) != 2 || len(out.Teams) != 1 {
		t.Fatalf("locked full destination: %+v %v", out, err)
	}
	if f.state() != before {
		t.Fatal("suggestions changed memberships, revision, audit or delivery state")
	}
	f.exec("INSERT INTO operational_profiles(event_id,person_id,dietary,accessibility) VALUES($1,$2,'PRIVATE-DIET-CANARY','PRIVATE-ACCESS-CANARY')", f.id, f.people[0].ID)
	got, err := f.s.Suggest(ctx, f.owner.ID, f.id, f.revision(), in)
	if err != nil || !reflect.DeepEqual(got, out) {
		t.Fatalf("private fields influenced allocation: %v", err)
	}
	data, _ := json.Marshal(got)
	if strings.Contains(string(data), "PRIVATE-") {
		t.Fatal("private fields leaked")
	}
	if err = f.act(Command{Action: "unlock", TeamID: f.groups[0]}); err != nil {
		t.Fatal(err)
	}
	out, err = f.s.Suggest(ctx, f.owner.ID, f.id, f.revision(), in)
	if err != nil || len(out.Unresolved) != 0 || len(out.Teams) != 2 {
		t.Fatalf("unlocked rebalancing: %+v %v", out, err)
	}
	for _, g := range out.Teams {
		if len(g.Members) != 4 {
			t.Fatalf("expected reviewed 4+4 proposal: %+v", out)
		}
	}
	otherOwner := testdb.Person(t, pool)
	otherEvent := testdb.Event(t, pool, otherOwner.ID)
	for _, actor := range []dbgen.Person{f.people[0], otherOwner} {
		if _, err = f.s.Suggest(ctx, actor.ID, f.id, f.revision(), in); !errors.Is(err, access.ErrForbidden) {
			t.Fatalf("unauthorised suggestion: %v", err)
		}
	}
	if _, err = f.s.Suggest(ctx, f.owner.ID, otherEvent, 1, in); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("cross-event suggestion: %v", err)
	}
	in.Declarations = append(in.Declarations, AllocationCandidate{ID: otherOwner.ID})
	if _, err = f.s.Suggest(ctx, f.owner.ID, f.id, f.revision(), in); !errors.Is(err, access.ErrInvalid) {
		t.Fatalf("manufactured competitor: %v", err)
	}
	if _, err = f.s.Suggest(ctx, f.owner.ID, f.id, f.revision()-1, in); !errors.Is(err, access.ErrConflict) {
		t.Fatalf("stale snapshot: %v", err)
	}
}

func TestAllocationControlsProtectLocksCapacitiesAndPendingPublication(t *testing.T) {
	pool := testdb.New(t, true)
	f := allocationDatabase(t, pool, 3, 3, 1)
	ctx := t.Context()
	if err := f.act(Command{Action: "move", TeamID: f.groups[1], PersonID: f.people[0].ID}); !errors.Is(err, ErrLocked) {
		t.Fatalf("moved locked competitor: %v", err)
	}
	three := 3
	if err := f.act(Command{Action: "capacity", TeamID: f.groups[0], Capacity: &three}); err != nil {
		t.Fatal(err)
	}
	if err := f.act(Command{Action: "move", TeamID: f.groups[0], PersonID: f.people[6].ID}); !errors.Is(err, ErrCapacity) {
		t.Fatalf("overfilled team: %v", err)
	}
	if err := f.act(Command{Action: "unlock", TeamID: f.groups[0]}); err != nil {
		t.Fatal(err)
	}
	if err := f.act(Command{Action: "move", TeamID: f.groups[1], PersonID: f.people[0].ID}); err != nil {
		t.Fatal(err)
	}
	if err := f.act(Command{Action: "lock", TeamID: f.groups[0]}); !errors.Is(err, ErrLocked) {
		t.Fatalf("silently locked a divergent published cohort: %v", err)
	}
	// Simulate a pending pre-upgrade draft move. Publication must not treat the
	// lock flag alone as proof that the confirmed cohort remained intact.
	f.exec("UPDATE hack_teams SET allocation_locked=true WHERE event_id=$1 AND id=$2", f.id, f.groups[0])
	if err := f.act(Command{Action: "publish"}); !errors.Is(err, ErrLocked) {
		t.Fatalf("published a stale locked move: %v", err)
	}
	if _, err := f.s.Suggest(ctx, f.owner.ID, f.id, f.revision(), AllocationRequest{Seed: "locked"}); !errors.Is(err, ErrLocked) {
		t.Fatalf("suggested from divergent locked cohorts: %v", err)
	}
}

func TestAllocationFreezeRaceAndDeadline(t *testing.T) {
	pool := testdb.New(t, true)
	f := allocationDatabase(t, pool, 3, 3)
	ctx := t.Context()
	revision := f.revision()
	var wg sync.WaitGroup
	results := make(chan error, 2)
	wg.Go(func() {
		_, err := f.s.Suggest(ctx, f.owner.ID, f.id, revision, AllocationRequest{Seed: "race"})
		results <- err
	})
	wg.Go(func() {
		_, err := f.s.Act(ctx, f.owner.ID, f.id, revision, Command{Action: "freeze", Reason: "Reviewed freeze"})
		results <- err
	})
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil && !errors.Is(err, access.ErrConflict) && !errors.Is(err, ErrFrozen) {
			t.Fatal(err)
		}
	}
	row, err := f.q.Event(ctx, f.id)
	if err != nil || !row.TeamFrozenAt.Valid {
		t.Fatalf("freeze did not persist: %v", err)
	}
	for _, action := range []string{"unlock", "lock", "capacity", "move"} {
		if err = f.act(Command{Action: action, TeamID: f.groups[0], PersonID: f.people[3].ID}); !errors.Is(err, ErrFrozen) {
			t.Fatalf("%s after freeze: %v", action, err)
		}
	}
	if _, err = f.s.Suggest(ctx, f.owner.ID, f.id, f.revision(), AllocationRequest{Seed: "after"}); !errors.Is(err, ErrFrozen) {
		t.Fatalf("suggested post-freeze: %v", err)
	}
	f2 := allocationDatabase(t, pool, 3, 3)
	f2.now = f2.c.TeamFreeze
	if _, err = f2.s.Suggest(ctx, f2.owner.ID, f2.id, f2.revision(), AllocationRequest{Seed: "deadline"}); !errors.Is(err, ErrFrozen) {
		t.Fatalf("deadline bypass: %v", err)
	}
	if err = f2.act(Command{Action: "unlock", TeamID: f2.groups[0]}); !errors.Is(err, ErrFrozen) {
		t.Fatalf("deadline unlock: %v", err)
	}
}

func TestAllocationUpgradeLocksPublishedTeamsWithoutChangingMembers(t *testing.T) {
	pool := testdb.New(t, false)
	ctx := t.Context()
	if err := database.Migrate(ctx, pool, 19); err != nil {
		t.Fatal(err)
	}
	owner := testdb.Person(t, pool)
	id := testdb.Event(t, pool, owner.ID)
	group := uuid.New()
	for index := range 3 {
		person := testdb.Person(t, pool)
		if index == 0 {
			if _, err := pool.Exec(ctx, "INSERT INTO hack_teams(id,event_id,name,captain_id,published_captain_id,state) VALUES($1,$2,'Existing confirmed team',$3,$3,'published')", group, id, person.ID); err != nil {
				t.Fatal(err)
			}
		}
		for _, snapshot := range []string{"draft", "published"} {
			if _, err := pool.Exec(ctx, "INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,$4)", id, group, person.ID, snapshot); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	row, err := dbgen.New(pool).HackTeam(ctx, dbgen.HackTeamParams{EventID: id, ID: group})
	if err != nil || !row.AllocationLocked || row.Capacity.Valid {
		t.Fatalf("existing published team not protected: %+v %v", row, err)
	}
	var count int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM hack_team_members WHERE event_id=$1", id).Scan(&count); err != nil || count != 6 {
		t.Fatalf("migration changed existing snapshots: %d %v", count, err)
	}
}
