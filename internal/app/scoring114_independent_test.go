package app

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/deploymenttheory/hack-in-a-box/internal/results"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type independentScoringFixture struct {
	Weights           []int           `json:"weights"`
	JudgeCards        [][]int         `json:"judge_cards"`
	CategoryMeans     []results.Exact `json:"category_means"`
	WeightedTotal     results.Exact   `json:"weighted_total"`
	MentorFinalTotal  int64           `json:"mentor_final_total"`
	ProvisionalValues []int           `json:"provisional_values"`
}

func TestIndependentHandWorkedScoringUsesNativeSourcesInBothModes(t *testing.T) {
	raw, err := os.ReadFile("testdata/scoring114-independent.json")
	resultsSourceRequire(t, err)
	var golden independentScoringFixture
	resultsSourceRequire(t, json.Unmarshal(raw, &golden))
	if len(golden.JudgeCards) != 3 || len(golden.Weights) != 5 || len(golden.CategoryMeans) != 5 {
		t.Fatal("independent fixture must retain its three-judge, five-category scope")
	}
	for _, mode := range []string{rubric.AllTeams, rubric.MentorQualification} {
		t.Run(mode, func(t *testing.T) {
			f := newResultsSourceFixtureWithSetup(t, mode, func(d *rubric.Definition) {
				d.RequiredJudgeCount = 3
				for i := range d.Categories {
					d.Categories[i].Weight = golden.Weights[i]
				}
			}, func(f *resultsSourceFixture, surface rehearsalSurface) {
				independentLiveCheckpoints(t, f, surface, golden.ProvisionalValues)
			})
			r := f.runtime
			third := testdb.Person(t, r.pool)
			resultsSourceRequire(t, dbgen.New(r.pool).SetMembership(t.Context(), dbgen.SetMembershipParams{
				EventID: r.eventID, PersonID: third.ID, Role: "judge", Active: true,
			}))
			judges := []dbgen.Person{f.judges[0], f.judges[1], third}
			f.round(t, judging.RoundCommand{Action: "assign", TeamID: r.teamID,
				JudgeIDs: []uuid.UUID{judges[0].ID, judges[1].ID, judges[2].ID}})
			blocked := func(want string) {
				t.Helper()
				page, err := f.service.Read(t.Context(), r.owner.ID, r.eventID)
				resultsSourceRequire(t, err)
				if !slices.Contains(page.Preview.Blockers, want) || len(page.Preview.Tallies) != 0 || page.Preview.Binding != "" {
					t.Fatalf("required native source was replaced by partial/zero scores: %+v", page.Preview)
				}
			}
			blocked("current_required_judge_assessment_needed")
			independentJudgeCard(t, f, judges[0], golden.JudgeCards[0], "submit", false)
			independentJudgeCard(t, f, judges[1], golden.JudgeCards[1], "save", true)
			blocked("current_required_judge_assessment_needed")
			independentJudgeCard(t, f, judges[2], golden.JudgeCards[2], "submit", false)
			blocked("current_required_judge_assessment_needed")
			independentJudgeCard(t, f, judges[1], golden.JudgeCards[1], "submit", false)
			blocked("judge_deliberation_required")
			f.round(t, judging.RoundCommand{Action: "open_deliberation"})
			page, err := f.service.Read(t.Context(), r.owner.ID, r.eventID)
			resultsSourceRequire(t, err)
			if len(page.Preview.Blockers) != 0 || len(page.Preview.Tallies) != 1 {
				t.Fatalf("complete actual source unavailable: %+v", page.Preview)
			}
			tally := page.Preview.Tallies[0]
			if tally.TeamID != r.teamID || tally.RequiredJudges != 3 || tally.Total != golden.WeightedTotal || len(tally.Categories) != len(golden.CategoryMeans) {
				t.Fatalf("native total differs from hand-worked fixture: %+v", tally)
			}
			for i, expected := range golden.CategoryMeans {
				if tally.Categories[i].Average != expected {
					t.Fatalf("category %d mean: got %+v, independently expected %+v", i, tally.Categories[i].Average, expected)
				}
			}
			if mode == rubric.MentorQualification {
				mentor, err := f.admin.deps.Qualification.Read(t.Context(), r.owner.ID, r.eventID, f.mentorRun)
				resultsSourceRequire(t, err)
				if !mentor.Current || mentor.Round.Proposal == nil || len(mentor.Round.Proposal.Rankings) != 1 || mentor.Round.Proposal.Rankings[0].Total != golden.MentorFinalTotal {
					t.Fatalf("provisional checkpoints accumulated into mentor final: %+v", mentor.Round.Proposal)
				}
			}
			// Assessment is deliberately on the same hack day as cutoff, not a
			// calendar-day workaround for the post-cutoff native gate.
			event, err := f.admin.deps.Events.Read(t.Context(), r.owner.ID, r.eventID)
			resultsSourceRequire(t, err)
			if r.clock.read().UTC().Format("2006-01-02") != event.Configuration.SubmissionCutoff.UTC().Format("2006-01-02") {
				t.Fatal("test left the hack date rather than exercising same-day assessment")
			}
			_, err = f.admin.deps.Events.Transition(t.Context(), r.owner.ID, r.eventID, event.Revision, "assessment", "Same-day independent scoring verification", false)
			resultsSourceRequire(t, err)
			page, err = f.service.Read(t.Context(), r.owner.ID, r.eventID)
			resultsSourceRequire(t, err)
			if len(page.Preview.Tallies) != 1 || page.Preview.Tallies[0].Total != golden.WeightedTotal {
				t.Fatal("same-day assessment changed the exact native result")
			}
		})
	}
}

func independentJudgeCard(t *testing.T, f *resultsSourceFixture, judge dbgen.Person, values []int, action string, pending bool) {
	t.Helper()
	r := f.runtime
	brief, err := f.admin.deps.Judging.Brief(t.Context(), judge.ID, r.eventID, f.judgeRun, r.teamID)
	resultsSourceRequire(t, err)
	if len(values) != len(brief.Criteria.Categories) {
		t.Fatal("golden judge input does not cover the published categories")
	}
	version := int64(0)
	if brief.OwnCard != nil {
		version = brief.OwnCard.Version
	}
	scores := make([]judging.Score, len(values))
	for i, value := range values {
		scores[i] = judging.Score{CategoryID: brief.Criteria.Categories[i].ID, Value: &value,
			Evidence: []judging.Evidence{{DeliverableID: &f.deliverable, Note: "PRIVATE-INDEPENDENT-GOLDEN-EVIDENCE"}}}
	}
	if pending {
		scores[1].Value = nil
	}
	_, err = f.admin.deps.Judging.ChangeCard(t.Context(), judge.ID, r.eventID, f.judgeRun, r.teamID, version,
		judging.CardCommand{RequestID: uuid.New(), Action: action, SubmissionRevision: brief.SubmissionRevision,
			RepositoryDecisionID: brief.Repository.DecisionID, Scores: scores, Reason: "Independent hand-worked native fixture"})
	resultsSourceRequire(t, err)
}

func independentLiveCheckpoints(t *testing.T, f *resultsSourceFixture, surface rehearsalSurface, values []int) {
	t.Helper()
	r := f.runtime
	config, err := surface.deps.Rubrics.Configuration(t.Context(), r.owner.ID, r.eventID, 0)
	resultsSourceRequire(t, err)
	config, err = surface.deps.Rubrics.Change(t.Context(), r.owner.ID, r.eventID, config.Revision,
		rubric.Command{Action: "create_run", Purpose: "checkpoint", Version: config.Reference.Version, Reason: "Independent live provisional inputs"})
	resultsSourceRequire(t, err)
	var run uuid.UUID
	for _, candidate := range config.Runs {
		if candidate.Purpose == "checkpoint" {
			run = candidate.ID
		}
	}
	event, err := surface.deps.Events.Read(t.Context(), r.owner.ID, r.eventID)
	resultsSourceRequire(t, err)
	opens, choice := testdb.WallTimeForInstant(t, event.Configuration.LiveStart, event.Configuration.Timezone)
	closes, endChoice := testdb.WallTimeForInstant(t, event.Configuration.SubmissionCutoff, event.Configuration.Timezone)
	if choice == "" {
		choice = endChoice
	}
	for _, value := range values {
		event, err = surface.deps.Events.Read(t.Context(), r.owner.ID, r.eventID)
		resultsSourceRequire(t, err)
		checkpoint, err := surface.deps.Scorecards.Configure(t.Context(), r.owner.ID, r.eventID, event.Revision,
			scorecards.CheckpointCommand{Definition: scorecards.CheckpointDefinition{ID: uuid.New(), Name: "Independent provisional checkpoint",
				RunID: run, OpensWallTime: opens, ClosesWallTime: closes, DSTChoice: choice}, Reason: "Independent non-accumulating checkpoint"})
		resultsSourceRequire(t, err)
		scores := make([]scorecards.CategoryScore, len(checkpoint.Criteria.Categories))
		for i, category := range checkpoint.Criteria.Categories {
			v := value
			scores[i] = scorecards.CategoryScore{CategoryID: category.ID, Value: &v,
				Evidence: []scorecards.EvidenceReference{{Kind: "note", Note: "PRIVATE-PROVISIONAL-GOLDEN-EVIDENCE"}}}
		}
		event, err = surface.deps.Events.Read(t.Context(), r.owner.ID, r.eventID)
		resultsSourceRequire(t, err)
		_, err = surface.deps.Scorecards.Change(t.Context(), r.mentor.ID, r.eventID, r.teamID, event.Revision,
			scorecards.CardCommand{Action: "submit", CheckpointID: checkpoint.Definition.ID,
				CheckpointVersion: checkpoint.Version, Scores: scores})
		resultsSourceRequire(t, err)
	}
}
