package app

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/moderation"
	"github.com/deploymenttheory/hack-in-a-box/internal/qualification"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type qualificationFinalTeam struct {
	id     uuid.UUID
	member dbgen.Person
}

type qualificationFinalFixture struct {
	teams         []qualificationFinalTeam
	runtime       *liveRehearsal
	admin, public rehearsalSurface
	run           uuid.UUID
	checkpoint    scorecards.Checkpoint
	second        dbgen.Person
	scores        []scorecards.CategoryScore
}

// Shared real native setup: formal submit while live, monotonic cutoff,
// independent final cards and signed current second-review/huddle source.
func newQualificationFinalFixture(t *testing.T) qualificationFinalFixture {
	return newQualificationFinalCohort(t, 1)
}

func newQualificationFinalCohort(t *testing.T, count int) qualificationFinalFixture {
	t.Helper()
	if count < 1 || count > 5 {
		t.Fatal("bounded fixture cohort must contain one to five teams")
	}
	var teams []qualificationFinalTeam
	var cp scorecards.Checkpoint
	var second dbgen.Person
	r, admin, public, run := qualificationHTTPFixtureInitialised(
		t,
		func(r *liveRehearsal) {
			r.mentor.DisplayName = "Native primary mentor"
			if _, err := r.pool.Exec(
				t.Context(),
				"UPDATE people SET display_name=$2 WHERE id=$1",
				r.mentor.ID,
				r.mentor.DisplayName,
			); err != nil {
				t.Fatal(err)
			}
			teams = append(teams, qualificationFinalTeam{id: r.teamID, member: r.competitor})
			var configRaw []byte
			if err := r.pool.QueryRow(t.Context(), "SELECT configuration FROM events WHERE id=$1", r.eventID).
				Scan(&configRaw); err != nil {
				t.Fatal(err)
			}
			var config struct {
				LiveStart time.Time `json:"live_start"`
				LiveEnd   time.Time `json:"live_end"`
			}
			if err := json.Unmarshal(configRaw, &config); err != nil {
				t.Fatal(err)
			}
			for index := 1; index < count; index++ {
				team := qualificationFinalTeam{id: uuid.New(), member: testdb.Person(t, r.pool)}
				for _, statement := range []struct {
					sql  string
					args []any
				}{
					{"INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')", []any{r.eventID, team.member.ID}},
					{"INSERT INTO hack_teams(event_id,id,name,state,frozen,published_captain_id)VALUES($1,$2,$3,'published',true,$4)", []any{r.eventID, team.id, fmt.Sprintf("Native panel team %d", index+1), team.member.ID}},
					{"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot)VALUES($1,$2,$3,'published')", []any{r.eventID, team.id, team.member.ID}},
					{"INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at)VALUES($1,$2,$3,'primary','published',$4,$5)", []any{r.eventID, team.id, r.mentor.ID, config.LiveStart, config.LiveEnd}},
				} {
					if _, err := r.pool.Exec(
						t.Context(),
						statement.sql,
						statement.args...); err != nil {
						t.Fatal(err)
					}
				}
				if err := dbgen.New(r.pool).
					CreateCompetitionEntry(t.Context(), dbgen.CreateCompetitionEntryParams{EventID: r.eventID, TeamID: team.id}); err != nil {
					t.Fatal(err)
				}
				teams = append(teams, team)
			}
			if _, err := r.pool.Exec(
				t.Context(),
				"UPDATE event_gate_facts SET total_teams=$2,frozen_teams=$2,assigned_attendees=$2 WHERE event_id=$1",
				r.eventID,
				count,
			); err != nil {
				t.Fatal(err)
			}
		},
		func(r *liveRehearsal, surface rehearsalSurface, run uuid.UUID) {
			page, err := surface.deps.Events.Read(t.Context(), r.owner.ID, r.eventID)
			if err != nil {
				t.Fatal(err)
			}
			second = testdb.Person(t, r.pool)
			second.DisplayName = "Native independent second mentor"
			if _, err := r.pool.Exec(
				t.Context(),
				"UPDATE people SET display_name=$2 WHERE id=$1",
				second.ID,
				second.DisplayName,
			); err != nil {
				t.Fatal(err)
			}
			if _, err = r.pool.Exec(
				t.Context(),
				"INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'mentor')",
				r.eventID,
				second.ID,
			); err != nil {
				t.Fatal(err)
			}
			availability, _ := json.Marshal(
				[]map[string]time.Time{
					{"start": page.Configuration.LiveStart, "end": page.Configuration.LiveEnd},
				},
			)
			for _, person := range []dbgen.Person{r.mentor, second} {
				if _, err = r.pool.Exec(
					t.Context(),
					"INSERT INTO mentor_profiles(event_id,person_id,skills,availability)VALUES($1,$2,'{}',$3)",
					r.eventID,
					person.ID,
					availability,
				); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = r.pool.Exec(
				t.Context(),
				"UPDATE mentor_profiles SET maximum_teams=$2 WHERE event_id=$1",
				r.eventID,
				count,
			); err != nil {
				t.Fatal(err)
			}
			for _, team := range teams {
				if _, err = r.pool.Exec(
					t.Context(),
					"INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at)VALUES($1,$2,$3,'second','published',$4,$5)",
					r.eventID,
					team.id,
					second.ID,
					page.Configuration.LiveStart,
					page.Configuration.LiveEnd,
				); err != nil {
					t.Fatal(err)
				}
			}
			deliverable := uuid.New()
			defs := templates.EmptyDefinitions()
			defs.Deliverables = []templates.Deliverable{
				{
					ID:            deliverable,
					Title:         "Formal hosted final",
					Required:      true,
					EvidenceKinds: []string{"url"},
					Audience:      []string{"mentor", "judge", "organiser"},
				},
			}
			raw, _ := json.Marshal(defs)
			if err = dbgen.New(r.pool).
				SaveEventDefinitions(t.Context(), dbgen.SaveEventDefinitionsParams{EventID: r.eventID, Definitions: raw}); err != nil {
				t.Fatal(err)
			}
			for _, team := range teams {
				native, err := surface.deps.Submissions.Read(
					t.Context(),
					team.member.ID,
					r.eventID,
					team.id,
				)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = surface.deps.Submissions.Submit(
					t.Context(),
					team.member.ID,
					r.eventID,
					team.id,
					native.Revision,
					submissions.Command{
						Evidence: []submissions.Evidence{
							{
								DeliverableID: deliverable,
								Kind:          "url",
								Label:         "Actual formal URL",
								URL:           "https://synthetic.invalid/formal-final",
							},
						},
					},
				); err != nil {
					t.Fatal(err)
				}
			}
			page, err = surface.deps.Events.Read(t.Context(), r.owner.ID, r.eventID)
			if err != nil {
				t.Fatal(err)
			}
			zone, err := time.LoadLocation(page.Configuration.Timezone)
			if err != nil {
				t.Fatal(err)
			}
			cp, err = surface.deps.Scorecards.Configure(
				t.Context(),
				r.owner.ID,
				r.eventID,
				page.Revision,
				scorecards.CheckpointCommand{
					Reason: "Actual independent final-review context",
					Definition: scorecards.CheckpointDefinition{
						ID:    uuid.New(),
						Name:  "Frozen final review",
						RunID: run,
						OpensWallTime: page.Configuration.SubmissionCutoff.In(zone).
							Format("2006-01-02T15:04:05"),
						ClosesWallTime: page.Configuration.LiveEnd.Add(time.Hour).
							In(zone).
							Format("2006-01-02T15:04:05"),
					},
				},
			)
			if err != nil {
				t.Fatal(err)
			}
		},
	)
	revision := func() int64 {
		t.Helper()
		p, err := admin.deps.Events.Read(t.Context(), r.owner.ID, r.eventID)
		if err != nil {
			t.Fatal(err)
		}
		return p.Revision
	}
	for _, team := range teams {
		if _, err := admin.deps.Submissions.Read(
			t.Context(),
			r.owner.ID,
			r.eventID,
			team.id,
		); err != nil {
			t.Fatal(err)
		}
	}
	scores := []scorecards.CategoryScore{}
	for _, category := range cp.Criteria.Categories {
		value := 6
		scores = append(
			scores,
			scorecards.CategoryScore{
				CategoryID: category.ID,
				Value:      &value,
				Evidence: []scorecards.EvidenceReference{
					{Kind: "note", Note: "Native independent final review evidence"},
				},
			},
		)
	}
	for _, team := range teams {
		for _, person := range []dbgen.Person{r.mentor, second} {
			if _, err := admin.deps.Scorecards.Change(
				t.Context(),
				person.ID,
				r.eventID,
				team.id,
				revision(),
				scorecards.CardCommand{
					Action:            "submit",
					CheckpointID:      cp.Definition.ID,
					CheckpointVersion: cp.Version,
					Scores:            scores,
				},
			); err != nil {
				t.Fatal(err)
			}
		}
		huddleID := uuid.New()
		h, err := admin.deps.Moderation.Change(
			t.Context(),
			r.owner.ID,
			r.eventID,
			team.id,
			huddleID,
			revision(),
			moderation.Command{
				Action: "configure",
				Reason: "Actual intended independent panel",
				Definition: &moderation.Definition{
					ID:                huddleID,
					CheckpointID:      cp.Definition.ID,
					CheckpointVersion: cp.Version,
					ModeratorID:       r.owner.ID,
					SecondReviewerID:  second.ID,
					ReviewDueAt:       cp.ClosesAt.Add(-time.Minute),
				},
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		for _, step := range []struct {
			actor  uuid.UUID
			action string
		}{{second.ID, "complete_second_review"}, {r.owner.ID, "open"}, {r.owner.ID, "sign_off"}} {
			h, err = admin.deps.Moderation.Change(
				t.Context(),
				step.actor,
				r.eventID,
				team.id,
				huddleID,
				revision(),
				moderation.Command{
					Action:  step.action,
					Version: h.Version,
					Reason:  "Actual current signed final source",
				},
			)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	return qualificationFinalFixture{
		teams:      teams,
		runtime:    r,
		admin:      admin,
		public:     public,
		run:        run,
		checkpoint: cp,
		second:     second,
		scores:     scores,
	}
}

func TestQualificationFiveTeamNativeFinalFixture(t *testing.T) {
	f := newQualificationFinalCohort(t, 5)
	page, err := f.admin.deps.Qualification.Read(
		t.Context(),
		f.runtime.owner.ID,
		f.runtime.eventID,
		f.run,
	)
	if err != nil {
		t.Fatal(err)
	}
	page, err = f.admin.deps.Qualification.Change(
		t.Context(),
		f.runtime.owner.ID,
		f.runtime.eventID,
		f.run,
		page.Revision,
		qualification.Command{
			RequestID: uuid.New(),
			Version:   page.Round.Version,
			Action:    "configure_panel",
			Reason:    "Actual intended five-team verification panel",
			Panel: &qualification.Panel{
				MentorIDs:   []uuid.UUID{f.runtime.mentor.ID, f.second.ID},
				OrganiserID: f.runtime.owner.ID,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Teams) != 5 {
		t.Fatalf("actual frozen cohort: got %d teams", len(page.Teams))
	}
	for _, team := range page.Teams {
		if team.Source == nil {
			t.Fatalf("team %s lacks actual signed independent source: %+v", team.TeamID, team)
		}
	}
}
