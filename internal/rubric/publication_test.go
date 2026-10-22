package rubric_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type publicationFixture struct {
	t                    *testing.T
	pool                 *pgxpool.Pool
	owner, first, second dbgen.Person
	eventID              uuid.UUID
	now, due             time.Time
	rubric               rubric.Service
	content              content.Service
	configuration        rubric.Configuration
}

func publicationSetup(t *testing.T) *publicationFixture {
	t.Helper()
	pool := testdb.New(t, false)
	// Development schema slot is provisional; the final ordered30→31 proof is
	// separate once the preceding migrations land on main.
	if err := database.Migrate(t.Context(), pool, 31); err != nil {
		t.Fatal(err)
	}
	f := &publicationFixture{
		t:      t,
		pool:   pool,
		owner:  testdb.Person(t, pool),
		first:  testdb.Person(t, pool),
		second: testdb.Person(t, pool),
	}
	f.eventID = testdb.Event(t, pool, f.owner.ID)
	c := event.DefaultConfiguration()
	c.Timezone = "Europe/London"
	c.MinimumAttendance = 2
	zone, _ := time.LoadLocation(c.Timezone)
	c.LiveStart = time.Date(2027, 3, 29, 9, 0, 0, 0, zone)
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	c.SubmissionCutoff = c.LiveEnd.Add(-time.Hour)
	f.due, _ = schedule.ChallengePublication(c.LiveStart, c.Timezone, c.DSTChoice)
	f.now = f.due.Add(-time.Minute)
	c.TeamFreeze = f.due.Add(-time.Hour)
	c.AdmissionCutoff = c.TeamFreeze.Add(-time.Hour)
	encoded, _ := json.Marshal(c)
	f.exec(
		"UPDATE events SET stage='preparation',configuration=$2,team_frozen_at=$3 WHERE id=$1",
		f.eventID,
		encoded,
		c.TeamFreeze,
	)
	f.exec(
		"INSERT INTO event_gate_facts(event_id,attendance_ready,teams_ready,total_teams,frozen_teams,assigned_attendees) VALUES($1,true,true,1,1,2)",
		f.eventID,
	)
	team := uuid.New()
	f.exec(
		"INSERT INTO hack_teams(event_id,id,name,state,frozen) VALUES($1,$2,'Synthetic team','published',true)",
		f.eventID,
		team,
	)
	for _, person := range []dbgen.Person{f.first, f.second} {
		f.exec(
			"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
			f.eventID,
			person.ID,
		)
		f.exec(
			"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
			f.eventID,
			team,
			person.ID,
		)
	}
	f.rubric = rubric.Service{Pool: pool, Clock: func() time.Time { return f.now }}
	f.content = content.Service{Pool: pool, Clock: func() time.Time { return f.now }}
	definition := rubric.Default(rubric.AllTeams, f.eventID)
	definition.PrivateJudgeGuidance = "SYNTHETIC-PRIVATE-JUDGE-GUIDANCE"
	f.change(rubric.Command{Action: "save", Definition: &definition})
	f.change(rubric.Command{Action: "approve", Version: f.configuration.Reference.Version})
	return f
}

func (f *publicationFixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.t.Context(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *publicationFixture) change(command rubric.Command) {
	f.t.Helper()
	latest, err := f.rubric.Configuration(f.t.Context(), f.owner.ID, f.eventID, 0)
	if err != nil {
		f.t.Fatal(err)
	}
	f.configuration.Revision = latest.Revision
	command.Reason = "Synthetic reviewed rubric decision"
	result, err := f.rubric.Change(
		f.t.Context(),
		f.owner.ID,
		f.eventID,
		f.configuration.Revision,
		command,
	)
	if err != nil {
		f.t.Fatal(err)
	}
	f.configuration = result
}

func (f *publicationFixture) challenge(old content.Page, ref *rubric.Reference) content.Page {
	f.t.Helper()
	if old.ID == uuid.Nil {
		old.ID = uuid.New()
	}
	p, err := f.content.Save(
		f.t.Context(),
		f.owner.ID,
		f.eventID,
		old.ID,
		old.Revision,
		content.Input{
			Kind: "challenge",
			Document: content.Document{
				Title:    "Synthetic common challenge",
				Markdown: "Common evidence and expectations.",
				Audience: access.ContentAudience{Confirmed: true},
				Rubric:   ref,
			},
		},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, action := range []string{"review", "approve"} {
		p, err = f.content.Action(
			f.t.Context(),
			f.owner.ID,
			f.eventID,
			p.ID,
			p.Revision,
			action,
			"Synthetic challenge review",
		)
		if err != nil {
			f.t.Fatal(err)
		}
	}
	return p
}

func (f *publicationFixture) common(person dbgen.Person) rubric.PublicPage {
	f.t.Helper()
	p, err := f.rubric.Public(f.t.Context(), person.ID, f.eventID)
	if err != nil {
		f.t.Fatal(err)
	}
	return p
}

func TestRubricSharesActualChallengeGateAndPublishesReviewedCorrectionAtomically(t *testing.T) {
	f := publicationSetup(t)
	ref := f.configuration.Reference
	challenge := f.challenge(content.Page{}, &ref)
	for _, person := range []dbgen.Person{f.first, f.second, f.owner} {
		if _, err := f.rubric.Public(
			t.Context(),
			person.ID,
			f.eventID,
		); !errors.Is(
			err,
			access.ErrNotFound,
		) {
			t.Fatal("standalone rubric disclosed before common challenge publication")
		}
	}
	f.now = f.due.Add(-time.Nanosecond)
	if err := f.content.Dispatch(
		t.Context(),
		f.eventID,
		challenge.ID,
		challenge.Revision,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rubric.Public(
		t.Context(),
		f.first.ID,
		f.eventID,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("early content worker disclosed criteria")
	}
	f.now = f.due
	if err := f.content.Dispatch(
		t.Context(),
		f.eventID,
		challenge.ID,
		challenge.Revision,
	); err != nil {
		t.Fatal(err)
	}
	first, second := f.common(f.first), f.common(f.second)
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(second)
	if string(a) != string(b) || first.Reference != ref ||
		strings.Contains(string(a), "SYNTHETIC-PRIVATE") ||
		strings.Contains(string(a), "private_judge_guidance") {
		t.Fatal("confirmed attendees received different criteria or private judge guidance")
	}
	definition := f.configuration.Definition
	definition.Categories[0].Weight = 2
	f.change(rubric.Command{Action: "save", Definition: &definition})
	f.change(rubric.Command{Action: "approve", Version: f.configuration.Reference.Version})
	if f.common(f.first).Reference != ref {
		t.Fatal("saving/approving a replacement silently changed published criteria")
	}
	released, err := f.content.Read(t.Context(), f.owner.ID, f.eventID, challenge.ID, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	next := f.configuration.Reference
	replacement := f.challenge(released, &next)
	if _, err = f.content.Action(
		t.Context(),
		f.owner.ID,
		f.eventID,
		replacement.ID,
		replacement.Revision,
		"release",
		"Synthetic correction",
	); err == nil {
		t.Fatal("postpublication criteria changed without impact review and reviewed notice")
	}
	f.change(
		rubric.Command{
			Action:       "review_impact",
			Version:      next.Version,
			ImpactReview: "SYNTHETIC-PRIVATE-IMPACT",
			Notice:       "Reviewed criteria update: demo weight is now two.",
		},
	)
	f.exec(
		"CREATE FUNCTION reject_rubric_notice() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic notice outage'; END $$",
	)
	f.exec(
		"CREATE TRIGGER rubric_notice_fault BEFORE INSERT ON rubric_notice_recipients FOR EACH ROW EXECUTE FUNCTION reject_rubric_notice()",
	)
	if _, err = f.content.Action(
		t.Context(),
		f.owner.ID,
		f.eventID,
		replacement.ID,
		replacement.Revision,
		"release",
		"Synthetic correction",
	); err == nil {
		t.Fatal("notice storage failure did not fail correction transaction")
	}
	if f.common(f.first).Reference != ref {
		t.Fatal("failed common notice partially published replacement criteria")
	}
	f.exec("DROP TRIGGER rubric_notice_fault ON rubric_notice_recipients")
	if _, err = f.content.Action(
		t.Context(),
		f.owner.ID,
		f.eventID,
		replacement.ID,
		replacement.Revision,
		"release",
		"Synthetic correction",
	); err != nil {
		t.Fatal(err)
	}
	first, second = f.common(f.first), f.common(f.second)
	a, _ = json.Marshal(first)
	b, _ = json.Marshal(second)
	if string(a) != string(b) || first.Reference != next || first.Notice == "" ||
		strings.Contains(string(a), "SYNTHETIC-PRIVATE") {
		t.Fatal("reviewed replacement omitted common notice or leaked private review/guidance")
	}
	var notified int
	if err = f.pool.QueryRow(t.Context(), "SELECT count(*) FROM rubric_notice_recipients").
		Scan(&notified); err != nil ||
		notified != 2 {
		t.Fatal("correction did not retain notice for every affected confirmed attendee")
	}
	f.exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		f.eventID,
		f.first.ID,
	)
	if _, err = f.rubric.Public(
		t.Context(),
		f.first.ID,
		f.eventID,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("withdrawal retained rubric audience access")
	}
	f.common(f.second)
}
