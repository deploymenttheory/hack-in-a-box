package mentors

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestCurrentMentorRosterVisibilityAndReadOnlyProposals(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	owner := testdb.Person(t, pool)
	primary := testdb.Person(t, pool)
	second := testdb.Person(t, pool)
	member := testdb.Person(t, pool)
	otherMember := testdb.Person(t, pool)
	stranger := testdb.Person(t, pool)
	admin := testdb.Person(t, pool)
	id := testdb.Event(t, pool, owner.ID)
	team, otherTeam := uuid.New(), uuid.New()
	c := event.DefaultConfiguration()
	c.LiveStart = time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	c.SubmissionCutoff = c.LiveEnd.Add(-time.Hour)
	encoded, _ := json.Marshal(c)
	exec(
		"UPDATE events SET stage='preparation',configuration=$2,team_frozen_at=now() WHERE id=$1",
		id,
		encoded,
	)
	exec("UPDATE people SET platform_admin=true WHERE id=$1", admin.ID)
	for _, person := range []dbgen.Person{primary, second} {
		exec(
			"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mentor')",
			id,
			person.ID,
		)
	}
	for index, person := range []dbgen.Person{member, otherMember} {
		current := team
		if index == 1 {
			current = otherTeam
		}
		exec(
			"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
			id,
			person.ID,
		)
		exec(
			"INSERT INTO hack_teams(id,event_id,name,state,allocation_locked) VALUES($1,$2,'Synthetic team','published',true)",
			current,
			id,
		)
		exec(
			"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
			id,
			current,
			person.ID,
		)
	}
	availability, _ := json.Marshal([]Window{{Start: c.LiveStart, End: c.LiveEnd}})
	for _, person := range []dbgen.Person{primary, second} {
		exec(
			"INSERT INTO mentor_profiles(event_id,person_id,skills,availability) VALUES($1,$2,ARRAY['design'],$3)",
			id,
			person.ID,
			availability,
		)
	}
	exec(
		"INSERT INTO mentor_conflicts(event_id,mentor_id,competitor_id,kind) VALUES($1,$2,$3,'reporting')",
		id,
		primary.ID,
		otherMember.ID,
	)
	for _, snapshot := range []string{"draft", "published"} {
		exec(
			"INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at) VALUES($1,$2,$3,'primary',$4,$5,$6),($1,$2,$7,'second',$4,$5,$6)",
			id,
			team,
			primary.ID,
			snapshot,
			c.LiveStart,
			c.LiveEnd,
			second.ID,
		)
	}
	q := dbgen.New(pool)
	for _, teamID := range []uuid.UUID{team, otherTeam} {
		if err := q.CreateCompetitionEntry(
			ctx,
			dbgen.CreateCompetitionEntryParams{EventID: id, TeamID: teamID},
		); err != nil {
			t.Fatal(err)
		}
	}
	policy := access.EventPolicy{Queries: q}
	if err := policy.Authorize(ctx, primary.ID, id, team, access.TeamRead); err != nil {
		t.Fatalf("primary mentor denied assigned team: %v", err)
	}
	if err := policy.Authorize(
		ctx,
		second.ID,
		id,
		team,
		access.TeamRead,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatalf("second reviewer received general team access: %v", err)
	}
	entry, err := q.CompetitionEntry(ctx, dbgen.CompetitionEntryParams{EventID: id, TeamID: team})
	if err != nil {
		t.Fatal(err)
	}
	if err = policy.Authorize(ctx, second.ID, id, entry.ID, access.AssessmentRead); err != nil {
		t.Fatalf("second reviewer denied assigned work: %v", err)
	}
	otherEntry, err := q.CompetitionEntry(
		ctx,
		dbgen.CompetitionEntryParams{EventID: id, TeamID: otherTeam},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err = policy.Authorize(
		ctx,
		second.ID,
		id,
		otherEntry.ID,
		access.AssessmentRead,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatalf("second reviewer received unassigned work: %v", err)
	}
	exec("UPDATE events SET published_at=$2 WHERE id=$1", id, c.LiveStart.AddDate(0, 0, -7))
	submissionsService := submissions.Service{
		Pool:  pool,
		Clock: func() time.Time { return c.LiveStart },
	}
	formal, err := submissionsService.Read(ctx, second.ID, id, team)
	if err != nil || formal.CanDraft || formal.CanSubmit || formal.CanReview {
		t.Fatalf("second reviewer access is not formal read-only: %+v %v", formal, err)
	}
	if _, err = submissionsService.Draft(
		ctx,
		second.ID,
		id,
		team,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatalf("second reviewer read private preparation: %v", err)
	}
	s := Service{Pool: pool}
	page, err := s.Read(ctx, owner.ID, id)
	if err != nil || !page.CanManage || len(page.Teams) != 2 || len(page.Conflicts) != 1 ||
		len(page.Draft) != 2 {
		t.Fatalf("organiser roster incomplete: %+v %v", page, err)
	}
	revision := page.Revision
	page, err = s.Read(ctx, member.ID, id)
	if err != nil || page.CanManage || len(page.Teams) != 1 || len(page.Published) != 2 ||
		len(page.Conflicts) != 0 ||
		len(page.Draft) != 0 {
		t.Fatalf("competitor roster scope: %+v %v", page, err)
	}
	for _, p := range page.Profiles {
		if len(p.Availability) != 0 || p.MaximumTeams != nil {
			t.Fatal("competitor received another person's full availability/capacity")
		}
	}
	page, err = s.Read(ctx, primary.ID, id)
	if err != nil || len(page.Published) != 1 || page.Published[0].PersonID != primary.ID ||
		len(page.Profiles) != 1 {
		t.Fatalf("mentor roster scope: %+v %v", page, err)
	}
	for _, person := range []dbgen.Person{stranger, admin} {
		if _, err = s.Read(ctx, person.ID, id); !errors.Is(err, access.ErrForbidden) {
			t.Fatalf("unassigned actor read private coverage: %v", err)
		}
	}
	if _, err = s.Propose(ctx, primary.ID, id, revision); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("mentor generated organiser-only proposal: %v", err)
	}
	if _, err = s.Propose(ctx, owner.ID, id, revision-1); !errors.Is(err, access.ErrConflict) {
		t.Fatalf("stale proposal accepted: %v", err)
	}
	if _, err = s.Propose(ctx, owner.ID, id, revision); err != nil {
		t.Fatalf("competitor freeze blocked mentor planning: %v", err)
	}
	var current int64
	var operations, audits int
	if err = pool.QueryRow(ctx, "SELECT revision,(SELECT count(*) FROM operations WHERE event_id=$1),(SELECT count(*) FROM audit_events WHERE event_id=$1) FROM events WHERE id=$1", id).
		Scan(&current, &operations, &audits); err != nil ||
		current != revision ||
		operations != 0 ||
		audits != 0 {
		t.Fatalf("proposal changed durable state: %d/%d/%d %v", current, operations, audits, err)
	}
	exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mentor'",
		id,
		primary.ID,
	)
	if _, err = s.Read(ctx, primary.ID, id); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("revoked mentor retained roster access: %v", err)
	}
	exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		id,
		member.ID,
	)
	if _, err = s.Read(ctx, member.ID, id); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("withdrawn competitor retained mentor roster: %v", err)
	}
}
