package automation

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestNativeRubricPublicationQueuesApprovedLocalMessageOnce(t *testing.T) {
	f := newFixture(t)
	_, relay := installProviders(t, f)
	rubrics, service, challenge, _ := prepareNativeCommonChallenge(t, f)
	var err error
	f.s.Sources = map[string]SourcePolicy{"rubric.published": RubricSource{Rubric: rubrics}}
	f.campaign("challenge_release", "Safe reviewed common rubric update", false, "in_app")
	page, _ := f.change(
		Command{
			Action: "save",
			Definition: &Definition{
				ID:                     uuid.New(),
				CampaignID:             f.campaignID,
				CampaignVersion:        f.campaignVersion,
				Trigger:                "rubric.published",
				SourceID:               &challenge.ID,
				SourceVersion:          challenge.Version,
				MaximumLatenessSeconds: 3600,
			},
			Reason: "Review native rubric journey",
		},
	)
	binding := page.Bindings[0]
	_, preview := f.change(
		Command{
			Action:    "preview_publication",
			BindingID: binding.Definition.ID,
			Version:   binding.Version,
			Reason:    "Review exact challenge and rubric release transition",
		},
	)
	if preview == nil || preview.GenericCampaignApproved {
		t.Fatal("typed plan misrepresented generic approval")
	}

	owners := f.s.Sources
	f.s.Sources = nil
	if _, _, err = f.s.Change(
		t.Context(),
		f.owner.ID,
		f.id,
		f.revision(),
		Command{
			Action:        "approve_publication",
			BindingID:     binding.Definition.ID,
			Version:       binding.Version,
			PreviewPlanID: preview.PlanID,
			Reason:        "A saved preview cannot substitute for a missing native owner",
		},
	); !errors.Is(
		err,
		communications.ErrUnresolved,
	) {
		t.Fatalf("missing native rubric owner approved typed plan: %v", err)
	}
	f.s.Sources = owners
	f.change(
		Command{
			Action:        "approve_publication",
			BindingID:     binding.Definition.ID,
			Version:       binding.Version,
			PreviewPlanID: preview.PlanID,
			Reason:        "Approve exact native common publication",
		},
	)
	if page = f.maintain(); len(page.Occurrences) != 0 {
		t.Fatal("unreleased rubric invented occurrence")
	}
	f.now = f.now.Add(time.Minute)
	if err = service.Dispatch(t.Context(), f.id, challenge.ID, challenge.Revision); err != nil {
		t.Fatal(err)
	}
	page = f.maintain()
	if len(page.Occurrences) != 1 || page.Occurrences[0].SourceRevision != challenge.Version {
		t.Fatalf("native publication not retained: %+v", page)
	}
	f.execute(page.Occurrences[0].ID)
	f.maintain()
	f.execute(page.Occurrences[0].ID)
	var count int
	var body string
	if err = f.pool.QueryRow(t.Context(), `SELECT count(*),coalesce(min(body),'') FROM inbox_notifications WHERE event_id=$1 AND source_kind='campaign'`, f.id).
		Scan(&count, &body); err != nil {
		t.Fatal(err)
	}
	if count != 2 || strings.Contains(body, "PRIVATE") || relay.calls.Load() != 0 {
		t.Fatalf(
			"rubric delivery omitted/duplicated/leaked: count%d body%q calls%d state%+v",
			count,
			body,
			relay.calls.Load(),
			f.maintain().Occurrences,
		)
	}
}

func prepareNativeCommonChallenge(
	t *testing.T,
	f *fixture,
) (rubric.Service, content.Service, content.Page, uuid.UUID) {
	t.Helper()
	rubrics := rubric.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	cfg := event.DefaultConfiguration()
	zone, _ := time.LoadLocation(cfg.Timezone)
	cfg.LiveStart = f.now.In(zone).AddDate(0, 0, 7).Add(time.Minute)
	cfg.LiveEnd = cfg.LiveStart.Add(8 * time.Hour)
	cfg.SubmissionCutoff = cfg.LiveEnd.Add(-time.Hour)
	cfg.TeamFreeze = f.now.Add(-time.Hour)
	cfg.AdmissionCutoff = cfg.TeamFreeze.Add(-time.Hour)
	cfg.MinimumAttendance = 2
	raw, _ := json.Marshal(cfg)
	f.exec(
		`UPDATE events SET stage='preparation',configuration=$2,team_frozen_at=$3 WHERE id=$1`,
		f.id,
		raw,
		cfg.TeamFreeze,
	)
	f.exec(
		`INSERT INTO event_gate_facts(event_id,attendance_ready,teams_ready,total_teams,frozen_teams,assigned_attendees)VALUES($1,true,true,1,1,2)`,
		f.id,
	)
	team := uuid.New()
	f.exec(
		`INSERT INTO hack_teams(event_id,id,name,state,frozen)VALUES($1,$2,'Synthetic team','published',true)`,
		f.id,
		team,
	)
	f.exec(`INSERT INTO competition_entries(event_id,team_id)VALUES($1,$2)`, f.id, team)
	second := testdb.Person(t, f.pool)
	f.exec(
		`INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')`,
		f.id,
		second.ID,
	)
	for _, person := range []uuid.UUID{f.person.ID, second.ID} {
		f.exec(
			`INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot)VALUES($1,$2,$3,'published')`,
			f.id,
			team,
			person,
		)
	}
	definition := rubric.Default(rubric.AllTeams, f.id)
	definition.PrivateJudgeGuidance = "PRIVATE-JUDGE-GUIDANCE-CANARY"
	saved, err := rubrics.Change(
		t.Context(),
		f.owner.ID,
		f.id,
		0,
		rubric.Command{
			Action:     "save",
			Definition: &definition,
			Reason:     "Configure common criteria",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := rubrics.Change(
		t.Context(),
		f.owner.ID,
		f.id,
		saved.Revision,
		rubric.Command{
			Action:  "approve",
			Version: saved.Reference.Version,
			Reason:  "Approve common criteria",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	service := content.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	challenge, err := service.Save(
		t.Context(),
		f.owner.ID,
		f.id,
		uuid.New(),
		0,
		content.Input{
			Kind: "challenge",
			Document: content.Document{
				Title:    "Common challenge",
				Markdown: "Public requirements",
				Audience: access.ContentAudience{Confirmed: true},
				Rubric:   &approved.Reference,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"review", "approve"} {
		challenge, err = service.Action(
			t.Context(),
			f.owner.ID,
			f.id,
			challenge.ID,
			challenge.Revision,
			action,
			"Review exact common source",
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	return rubrics, service, challenge, team
}
