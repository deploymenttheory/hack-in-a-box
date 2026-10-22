package preparation

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestReadinessAudiencePrivacyAndRevocation(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	owner, member, mentor, outsider := testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	)
	id := testdb.Event(t, pool, owner.ID)
	q := dbgen.New(pool)
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	s := Service{Pool: pool, Now: func() time.Time { return now }}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	c := event.DefaultConfiguration()
	c.LiveStart = now.Add(time.Hour)
	c.LiveEnd = now.Add(8 * time.Hour)
	c.SubmissionCutoff = now.Add(7 * time.Hour)
	c.TeamFreeze = now.Add(-8 * 24 * time.Hour)
	c.AdmissionCutoff = c.TeamFreeze.Add(-time.Hour)
	c.Schedule = []event.AgendaItem{
		{
			ID:       uuid.New(),
			Title:    "Unreleased raw agenda secret",
			Audience: []string{"attendee", "mentor", "organiser"},
			Anchor:   "live_start",
		},
	}
	data, _ := json.Marshal(c)
	exec("UPDATE events SET configuration=$2 WHERE id=$1", id, data)
	exec(
		"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
		id,
		member.ID,
	)
	exec(
		"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mentor')",
		id,
		mentor.ID,
	)
	exec(
		"INSERT INTO operational_profiles(event_id,person_id,contact_email,dietary,accessibility) VALUES($1,$2,'secret@example.invalid','diet-secret','access-secret')",
		id,
		member.ID,
	)
	task := func(title, role string) {
		t.Helper()
		page, err := s.List(ctx, owner.ID, id, Filter{})
		if err != nil {
			t.Fatal(err)
		}
		due := now.Add(-time.Hour)
		_, err = s.Save(
			ctx,
			owner.ID,
			id,
			page.Revision,
			Update{
				Definition: Definition{
					ID:          uuid.New(),
					Title:       title,
					Description: "private-staff-notes",
					OwnerRole:   role,
					Status:      "not_started",
					FixedDueAt:  &due,
				},
			},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	task("Organiser confidential task", "organiser")
	task("Mentor preparation", "mentor")
	task("Participant preparation", "attendee")
	team, err := q.CreateHackTeam(
		ctx,
		dbgen.CreateHackTeamParams{
			ID:        uuid.New(),
			EventID:   id,
			Name:      "Visible team",
			CaptainID: &member.ID,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err = q.CreateCompetitionEntry(
		ctx,
		dbgen.CreateCompetitionEntryParams{EventID: id, TeamID: team.ID},
	); err != nil {
		t.Fatal(err)
	}
	exec("UPDATE hack_teams SET state='published' WHERE event_id=$1", id)
	exec(
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
		id,
		team.ID,
		member.ID,
	)
	defs := templates.EmptyDefinitions()
	defs.Deliverables = []templates.Deliverable{
		{
			ID:       uuid.New(),
			Title:    "Embargoed challenge deliverable",
			Required: true,
			Audience: []string{"mentor"},
		},
	}
	data, _ = json.Marshal(defs)
	exec(
		"INSERT INTO event_definitions(event_id,definitions) VALUES($1,$2) ON CONFLICT(event_id) DO UPDATE SET definitions=EXCLUDED.definitions",
		id,
		data,
	)
	// Platform administration and unrelated event access never grant export access.
	exec("UPDATE people SET platform_admin=true WHERE id=$1", outsider.ID)
	for _, aud := range []string{"organiser", "mentor", "participant"} {
		if _, err = s.Export(ctx, outsider.ID, id, aud); !errors.Is(err, access.ErrForbidden) {
			t.Fatalf("outsider %s: %v", aud, err)
		}
	}
	for _, v := range []struct {
		p            uuid.UUID
		aud, yes, no string
	}{{owner.ID, "organiser", "Organiser confidential task", "secret@example.invalid"}, {mentor.ID, "mentor", "Mentor preparation", "Organiser confidential task"}, {member.ID, "participant", "Participant preparation", "Organiser confidential task"}} {
		report, err := s.Export(ctx, v.p, id, v.aud)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := report.HTML()
		text := string(b)
		if !strings.Contains(text, v.yes) || strings.Contains(text, v.no) ||
			strings.Contains(text, "private-staff-notes") ||
			strings.Contains(text, "diet-secret") ||
			strings.Contains(text, "access-secret") {
			t.Fatalf("audience leak %s: %s", v.aud, text)
		}
		if v.aud != "organiser" &&
			(strings.Contains(text, "Embargoed challenge deliverable") || strings.Contains(text, "Unreleased raw agenda secret")) {
			t.Fatal("early challenge definition leak")
		}
	}
	// Actual publication reveals the participant's required formal deliverables,
	// without opening formal submissions before the live start.
	exec("UPDATE events SET published_at=$2 WHERE id=$1", id, now.Add(-time.Minute))
	report, err := s.Export(ctx, member.ID, id, "participant")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := report.CSV()
	if !strings.Contains(string(b), "Embargoed challenge deliverable") ||
		!strings.Contains(string(b), "Missing formal evidence") {
		t.Fatalf("published participant deliverables: %s", b)
	}
	// At live start, assigned mentors see only assigned formal deliverables.
	exec(
		"INSERT INTO resource_assignments(event_id,resource_id,person_id) VALUES($1,$2,$3)",
		id,
		team.ID,
		mentor.ID,
	)
	other, err := q.CreateHackTeam(
		ctx,
		dbgen.CreateHackTeamParams{ID: uuid.New(), EventID: id, Name: "Other private team"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err = q.CreateCompetitionEntry(
		ctx,
		dbgen.CreateCompetitionEntryParams{EventID: id, TeamID: other.ID},
	); err != nil {
		t.Fatal(err)
	}
	exec("UPDATE hack_teams SET state='published' WHERE event_id=$1", id)
	now = c.LiveStart.Add(time.Minute)
	report, err = s.Export(ctx, mentor.ID, id, "mentor")
	if err != nil {
		t.Fatal(err)
	}
	b, _ = report.CSV()
	if !strings.Contains(string(b), "Visible team: Embargoed challenge deliverable") ||
		strings.Contains(string(b), "Other private team") {
		t.Fatalf("mentor resource scope: %s", b)
	}
	exec("DELETE FROM resource_assignments WHERE event_id=$1 AND person_id=$2", id, mentor.ID)
	report, err = s.Export(ctx, mentor.ID, id, "mentor")
	if err != nil {
		t.Fatal(err)
	}
	b, _ = report.CSV()
	if strings.Contains(string(b), "Visible team") {
		t.Fatalf("reassigned mentor stale scope: %s", b)
	}
	now = c.SubmissionCutoff.Add(time.Second)
	report, err = s.Export(ctx, member.ID, id, "participant")
	if err != nil {
		t.Fatal(err)
	}
	b, _ = report.CSV()
	if !strings.Contains(string(b), "not_submitted") ||
		strings.Contains(string(b), "Other private team") {
		t.Fatalf("explicit outcome and participant team scope: %s", b)
	}
	exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		id,
		member.ID,
	)
	if _, err = s.Export(ctx, member.ID, id, "participant"); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("withdrawn export: %v", err)
	}
	exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2",
		id,
		mentor.ID,
	)
	if _, err = s.Export(ctx, mentor.ID, id, "mentor"); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("revoked mentor export: %v", err)
	}
}

func TestReadinessEmptyEvent(t *testing.T) {
	pool := testdb.New(t, true)
	p := testdb.Person(t, pool)
	id := testdb.Event(t, pool, p.ID)
	r, err := (Service{Pool: pool}).Export(t.Context(), p.ID, id, "organiser")
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.HTML()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Not configured", "Confirmed competitors", "Minimum required: 30", "Unaccepted reservations", "Visible missing required deliverables"} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("empty report missing %q", want)
		}
	}
}
