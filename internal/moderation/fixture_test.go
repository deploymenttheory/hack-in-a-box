package moderation

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type fixture struct {
	t                                     *testing.T
	pool                                  *pgxpool.Pool
	s                                     scorecards.Service
	owner, primary, second, member, other dbgen.Person
	event, team, entry                    uuid.UUID
	now                                   time.Time
	config                                event.Configuration
	rubric                                rubric.Service
	criteria                              rubric.Configuration
	checkpoint                            scorecards.Checkpoint
}

func setup(t *testing.T) *fixture {
	t.Helper()
	pool := testdb.New(t, true)
	// Development-only unnumbered owner schema, replaced by the normal migration
	// after actual103 lands. This is not an ordered-upgrade verification claim.
	rawSchema, err := os.ReadFile("schema.provisional.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(t.Context(), string(rawSchema)); err != nil {
		t.Fatal(err)
	}
	f := &fixture{
		t:       t,
		pool:    pool,
		owner:   testdb.Person(t, pool),
		primary: testdb.Person(t, pool),
		second:  testdb.Person(t, pool),
		member:  testdb.Person(t, pool),
		other:   testdb.Person(t, pool),
		team:    uuid.New(),
		entry:   uuid.New(),
	}
	f.event = testdb.Event(t, pool, f.owner.ID)
	f.config = event.DefaultConfiguration()
	f.config.Timezone = "Europe/London"
	f.config.MinimumAttendance = 2
	f.config.LiveStart = time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	f.config.LiveEnd = f.config.LiveStart.Add(8 * time.Hour)
	f.config.SubmissionCutoff = f.config.LiveEnd.Add(-time.Hour)
	due, err := schedule.ChallengePublication(
		f.config.LiveStart,
		f.config.Timezone,
		f.config.DSTChoice,
	)
	if err != nil {
		t.Fatal(err)
	}
	f.config.TeamFreeze = due.Add(-time.Hour)
	f.config.AdmissionCutoff = f.config.TeamFreeze.Add(-time.Hour)
	f.now = due.Add(-time.Minute)
	config, _ := json.Marshal(f.config)
	f.exec(
		"UPDATE events SET stage='preparation',configuration=$2,team_frozen_at=$3 WHERE id=$1",
		f.event,
		config,
		f.config.TeamFreeze,
	)
	f.exec(
		"INSERT INTO event_gate_facts(event_id,attendance_ready,teams_ready,total_teams,frozen_teams,assigned_attendees) VALUES($1,true,true,1,1,2)",
		f.event,
	)
	f.exec(
		"INSERT INTO hack_teams(event_id,id,name,state,frozen) VALUES($1,$2,'Synthetic scorecard team','published',true)",
		f.event,
		f.team,
	)
	f.exec(
		"INSERT INTO competition_entries(id,event_id,team_id) VALUES($1,$2,$3)",
		f.entry,
		f.event,
		f.team,
	)
	for _, p := range []dbgen.Person{f.member, f.other} {
		f.exec(
			"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
			f.event,
			p.ID,
		)
		f.exec(
			"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
			f.event,
			f.team,
			p.ID,
		)
	}
	for _, p := range []dbgen.Person{f.primary, f.second} {
		f.exec(
			"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mentor')",
			f.event,
			p.ID,
		)
	}
	f.exec(
		"INSERT INTO policy_resources(event_id,id,kind) VALUES($1,$2,'team'),($1,$3,'assessment')",
		f.event,
		f.team,
		f.entry,
	)
	f.exec(
		"INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at) VALUES($1,$2,$3,'primary','published',$5,$6),($1,$2,$4,'second','published',$5,$6)",
		f.event,
		f.team,
		f.primary.ID,
		f.second.ID,
		f.config.LiveStart,
		f.config.LiveEnd,
	)
	for _, person := range []dbgen.Person{f.primary, f.second} {
		availability, _ := json.Marshal(
			[]map[string]time.Time{{"start": f.config.LiveStart, "end": f.config.LiveEnd}},
		)
		f.exec(
			`INSERT INTO mentor_profiles(event_id,person_id,skills,availability)VALUES($1,$2,'{}',$3)`,
			f.event,
			person.ID,
			availability,
		)
	}
	f.s = scorecards.Service{Pool: pool, Now: func() time.Time { return f.now }}
	f.rubric = rubric.Service{Pool: pool, Clock: func() time.Time { return f.now }}
	definition := rubric.Default(rubric.MentorQualification, f.event)
	definition.PrivateJudgeGuidance = "PRIVATE-JUDGE-CANARY"
	f.rubricChange(rubric.Command{Action: "save", Definition: &definition})
	f.rubricChange(rubric.Command{Action: "approve", Version: f.criteria.Reference.Version})
	c := content.Service{Pool: pool, Clock: func() time.Time { return f.now }}
	page, err := c.Save(
		t.Context(),
		f.owner.ID,
		f.event,
		uuid.New(),
		0,
		content.Input{
			Kind: "challenge",
			Document: content.Document{
				Title:    "Actual guarded challenge",
				Markdown: "Synthetic approved competition output",
				Rubric:   &f.criteria.Reference,
				Audience: access.ContentAudience{Confirmed: true},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"review", "approve"} {
		page, err = c.Action(
			t.Context(),
			f.owner.ID,
			f.event,
			page.ID,
			page.Revision,
			action,
			"Synthetic explicit common challenge",
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	f.now = due
	if err = c.Dispatch(t.Context(), f.event, page.ID, page.Revision); err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE events SET stage='live' WHERE id=$1", f.event)
	f.now = f.config.LiveStart
	f.checkpoint = f.configure("checkpoint")
	return f
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.t.Context(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) revision() int64 {
	f.t.Helper()
	row, err := dbgen.New(f.pool).Event(f.t.Context(), f.event)
	if err != nil {
		f.t.Fatal(err)
	}
	return row.Revision
}

func (f *fixture) rubricChange(c rubric.Command) {
	f.t.Helper()
	latest, err := f.rubric.Configuration(f.t.Context(), f.owner.ID, f.event, 0)
	if err != nil {
		f.t.Fatal(err)
	}
	f.criteria.Revision = latest.Revision
	c.Reason = "Synthetic reviewed criteria"
	out, err := f.rubric.Change(f.t.Context(), f.owner.ID, f.event, f.criteria.Revision, c)
	if err != nil {
		f.t.Fatal(err)
	}
	f.criteria = out
}

func (f *fixture) configure(purpose string) scorecards.Checkpoint {
	f.t.Helper()
	f.rubricChange(
		rubric.Command{
			Action:  "create_run",
			Version: f.criteria.Reference.Version,
			Purpose: purpose,
		},
	)
	var run uuid.UUID
	for _, r := range f.criteria.Runs {
		if r.Purpose == purpose {
			run = r.ID
		}
	}
	zone, _ := time.LoadLocation(f.config.Timezone)
	opens, closes := f.config.LiveStart, f.config.SubmissionCutoff
	if purpose == "mentor_final" {
		opens, closes = f.config.SubmissionCutoff, f.config.LiveEnd.Add(time.Hour)
	}
	out, err := f.s.Configure(
		f.t.Context(),
		f.owner.ID,
		f.event,
		f.revision(),
		scorecards.CheckpointCommand{
			Definition: scorecards.CheckpointDefinition{
				ID:             uuid.New(),
				Name:           "Synthetic " + purpose,
				RunID:          run,
				OpensWallTime:  opens.In(zone).Format("2006-01-02T15:04:05"),
				ClosesWallTime: closes.In(zone).Format("2006-01-02T15:04:05"),
			},
			Reason: "Explicit configured checkpoint",
		},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	return out
}

func (f *fixture) command(
	cp scorecards.Checkpoint,
	action string,
	complete bool,
) scorecards.CardCommand {
	scores := []scorecards.CategoryScore{}
	if complete {
		for _, category := range cp.Criteria.Categories {
			v := category.MinScore
			scores = append(
				scores,
				scorecards.CategoryScore{
					CategoryID: category.ID,
					Value:      &v,
					Evidence: []scorecards.EvidenceReference{
						{Kind: "note", Note: "PRIVATE-SCORE-EVIDENCE-CANARY"},
					},
				},
			)
		}
	}
	return scorecards.CardCommand{
		Action:            action,
		CheckpointID:      cp.Definition.ID,
		CheckpointVersion: cp.Version,
		Scores:            scores,
	}
}

func (f *fixture) freeze() {
	f.t.Helper()
	f.exec("INSERT INTO submission_states(event_id,team_id) VALUES($1,$2)", f.event, f.team)
	f.exec(
		"INSERT INTO submission_revisions(event_id,team_id,revision,actor_id,submitted_at,late,document) VALUES($1,$2,1,$3,$4,false,'{\"definitions\":[],\"evidence\":[]}')",
		f.event,
		f.team,
		f.member.ID,
		f.config.SubmissionCutoff.Add(-time.Minute),
	)
	f.exec(
		"UPDATE submission_states SET active_revision=1,outcome='frozen',frozen_at=$3 WHERE event_id=$1 AND team_id=$2",
		f.event,
		f.team,
		f.config.SubmissionCutoff,
	)
}
