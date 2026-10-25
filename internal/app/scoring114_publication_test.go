package app

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/deploymenttheory/hack-in-a-box/internal/qualification"
	"github.com/deploymenttheory/hack-in-a-box/internal/results"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestIndependentFiveTeamQualificationToNonFinalistCategoryAwardPublication(t *testing.T) {
	prizes := [3]uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	var demo, technical uuid.UUID
	f := newQualificationFinalCohortWithCriteria(t, 5, func(d *rubric.Definition) {
		d.RequiredJudgeCount = 2
		demo, technical = d.Categories[0].ID, d.Categories[1].ID
		d.Prizes = []rubric.Prize{
			{ID: prizes[0], Name: "Overall winner"},
			{ID: prizes[1], Name: "Demo winner", CategoryID: &demo},
			{ID: prizes[2], Name: "Technical winner", CategoryID: &technical},
		}
	})
	r, ctx := f.runtime, t.Context()
	panel := f.admin.deps.Qualification
	page, err := panel.Read(ctx, r.owner.ID, r.eventID, f.run)
	resultsSourceRequire(t, err)
	change := func(command qualification.Command) {
		t.Helper()
		command.RequestID, command.Version, command.Reason = uuid.New(), page.Round.Version, "Independent complete five-team publication fixture"
		page, err = panel.Change(ctx, r.owner.ID, r.eventID, f.run, page.Revision, command)
		resultsSourceRequire(t, err)
	}
	change(qualification.Command{Action: "configure_panel", Panel: &qualification.Panel{
		MentorIDs: []uuid.UUID{r.mentor.ID, f.second.ID}, OrganiserID: r.owner.ID,
	}})
	// Independently summed totals: 47,42,37,32,14. Team five is neither a
	// finalist nor reserve, but wins technical10; team one wins four categories.
	values := [5][5]int{{10, 9, 10, 9, 9}, {9, 8, 9, 8, 8}, {8, 7, 8, 7, 7}, {7, 6, 7, 6, 6}, {1, 10, 1, 1, 1}}
	for i, team := range f.teams {
		status := page.Teams[slices.IndexFunc(page.Teams, func(s qualification.TeamStatus) bool { return s.TeamID == team.id })]
		scores := make([]scorecards.CategoryScore, 5)
		for category, value := range values[i] {
			scores[category] = scorecards.CategoryScore{CategoryID: page.Criteria.Categories[category].ID, Value: &value,
				Evidence: []scorecards.EvidenceReference{{Kind: "note", Note: "PRIVATE-MODERATED-CATEGORY-REASON"}}}
		}
		change(qualification.Command{Action: "save_card", FreezeBinding: status.FreezeBinding,
			Card: &qualification.CardInput{TeamID: team.id, Scores: scores}})
		status = page.Teams[slices.IndexFunc(page.Teams, func(s qualification.TeamStatus) bool { return s.TeamID == team.id })]
		change(qualification.Command{Action: "sign_card", TeamID: team.id, CardVersion: status.CardVersion, FreezeBinding: status.FreezeBinding})
	}
	change(qualification.Command{Action: "propose"})
	if page.Round.Proposal == nil || len(page.Round.Proposal.Ties) != 0 || len(page.Round.Proposal.Blockers) != 0 {
		t.Fatal("complete independent qualification proposal blocked", page.Round.Proposal)
	}
	expected := []int64{47, 42, 37, 32, 14}
	for i, rank := range page.Round.Proposal.Rankings {
		if rank.TeamID != f.teams[i].id || rank.Total != expected[i] {
			t.Fatal("qualification arithmetic differs from independent sums", page.Round.Proposal.Rankings)
		}
	}
	if !slices.Equal(page.Round.Proposal.FinalistIDs, []uuid.UUID{f.teams[0].id, f.teams[1].id, f.teams[2].id}) || !slices.Equal(page.Round.Proposal.ReserveIDs, []uuid.UUID{f.teams[3].id}) {
		t.Fatal("native three finalists/reserve differ from hand-worked totals", page.Round.Proposal)
	}
	change(qualification.Command{Action: "sign_off"})
	owner := f.admin.deps.Results
	prepared, err := owner.Read(ctx, r.owner.ID, r.eventID)
	resultsSourceRequire(t, err)
	prepared, err = owner.Change(ctx, r.owner.ID, r.eventID, prepared.Revision, results.Command{
		RequestID: uuid.New(), Version: prepared.Round.Version, Action: "publish_finalists", SourceBinding: prepared.Preview.FinalistsBinding, Reason: "Explicit finalist announcement before fresh final",
	})
	resultsSourceRequire(t, err)
	config, err := f.admin.deps.Rubrics.Configuration(ctx, r.owner.ID, r.eventID, 0)
	resultsSourceRequire(t, err)
	config, err = f.admin.deps.Rubrics.Change(ctx, r.owner.ID, r.eventID, config.Revision,
		rubric.Command{Action: "create_run", Purpose: "judge_final", Version: config.Reference.Version, Reason: "Fresh native final after signed five-team qualification"})
	resultsSourceRequire(t, err)
	var run uuid.UUID
	for _, candidate := range config.Runs {
		if candidate.Purpose == "judge_final" {
			run = candidate.ID
		}
	}
	judges := []dbgen.Person{testdb.Person(t, r.pool), testdb.Person(t, r.pool)}
	for _, judge := range judges {
		resultsSourceRequire(t, dbgen.New(r.pool).SetMembership(ctx, dbgen.SetMembershipParams{EventID: r.eventID, PersonID: judge.ID, Role: "judge", Active: true}))
	}
	for i, team := range f.teams[:3] {
		round, err := f.admin.deps.Judging.Round(ctx, r.owner.ID, r.eventID, run)
		resultsSourceRequire(t, err)
		_, err = f.admin.deps.Judging.ChangeRound(ctx, r.owner.ID, r.eventID, run, round.Revision,
			judging.RoundCommand{RequestID: uuid.New(), Action: "assign", TeamID: team.id, JudgeIDs: []uuid.UUID{judges[0].ID, judges[1].ID}, Reason: "Independent judges for actual selected finalist"})
		resultsSourceRequire(t, err)
		for _, judge := range judges {
			brief, err := f.admin.deps.Judging.Brief(ctx, judge.ID, r.eventID, run, team.id)
			resultsSourceRequire(t, err)
			scores := make([]judging.Score, len(brief.Criteria.Categories))
			for c, category := range brief.Criteria.Categories {
				value := 7 - i
				scores[c] = judging.Score{CategoryID: category.ID, Value: &value,
					Evidence: []judging.Evidence{{Note: "PRIVATE-FRESH-FINAL-JUDGE-REASON"}}}
			}
			_, err = f.admin.deps.Judging.ChangeCard(ctx, judge.ID, r.eventID, run, team.id, 0,
				judging.CardCommand{RequestID: uuid.New(), Action: "submit", SubmissionRevision: brief.SubmissionRevision,
					RepositoryDecisionID: brief.Repository.DecisionID, Scores: scores, Reason: "Fresh final has no mentor carry"})
			resultsSourceRequire(t, err)
		}
	}
	round, err := f.admin.deps.Judging.Round(ctx, r.owner.ID, r.eventID, run)
	resultsSourceRequire(t, err)
	_, err = f.admin.deps.Judging.ChangeRound(ctx, r.owner.ID, r.eventID, run, round.Revision,
		judging.RoundCommand{RequestID: uuid.New(), Action: "open_deliberation", Reason: "All three finalists have complete independent cards"})
	resultsSourceRequire(t, err)
	prepared, err = owner.Read(ctx, r.owner.ID, r.eventID)
	resultsSourceRequire(t, err)
	if len(prepared.Preview.Tallies) != 3 || prepared.Preview.Tallies[0].Total != (results.Exact{Numerator: "35", Denominator: "1", Display: "35.00"}) || slices.ContainsFunc(prepared.Preview.Tallies, func(tally results.TeamTally) bool { return tally.TeamID == f.teams[4].id }) {
		t.Fatal("fresh final carried mentor scores or included nonfinalist", prepared.Preview.Tallies)
	}
	for _, action := range []string{"prepare", "confirm", "publish_awards"} {
		command := results.Command{RequestID: uuid.New(), Version: prepared.Round.Version, Action: action,
			SourceBinding: prepared.Preview.Binding, Reason: "Independent deliberately reviewed full publication"}
		if action == "confirm" {
			command.Awards = []results.Award{
				{PrizeID: prizes[0], Name: "Overall winner", TeamID: f.teams[0].id},
				{PrizeID: prizes[1], Name: "Demo winner", CategoryID: &demo, TeamID: f.teams[0].id},
				{PrizeID: prizes[2], Name: "Technical winner", CategoryID: &technical, TeamID: f.teams[4].id},
			}
		}
		prepared, err = owner.Change(ctx, r.owner.ID, r.eventID, prepared.Revision, command)
		resultsSourceRequire(t, err)
		announced, err := owner.Released(ctx, f.teams[4].member.ID, r.eventID)
		resultsSourceRequire(t, err)
		if len(announced) != 1 && action != "publish_awards" {
			t.Fatal("private preparation or confirmation implicitly published awards", action, announced)
		}
	}
	announced, err := owner.Released(ctx, f.teams[4].member.ID, r.eventID)
	resultsSourceRequire(t, err)
	index := slices.IndexFunc(announced, func(a results.Announcement) bool { return a.Kind == "awards" })
	if len(announced) != 2 || index < 0 || len(announced[index].Awards) != 3 {
		t.Fatal("complete explicitly released awards missing", announced)
	}
	awards := announced[index].Awards
	if awards[0].Team.ID != f.teams[0].id || awards[1].Team.ID != f.teams[0].id || awards[2].Team.ID != f.teams[4].id {
		t.Fatal("multi-award winner or nonfinalist category winner replaced", awards)
	}
	raw, err := json.Marshal(announced)
	resultsSourceRequire(t, err)
	for _, secret := range []string{"PRIVATE-MODERATED-CATEGORY-REASON", "PRIVATE-FRESH-FINAL-JUDGE-REASON", "rankings", "tallies", "source_binding", "votes", "evidence"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("private assessment leaked in nonfinalist award announcement", secret)
		}
	}
}
