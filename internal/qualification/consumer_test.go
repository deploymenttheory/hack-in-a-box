package qualification

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestActualFreshJudgeConsumerUsesOnlyCurrentSelectedFinalistsAndNoMentorPayload(t *testing.T) {
	f, s, cp, p := signedQualificationCohort(
		t,
		[][]int{
			{10, 9, 10, 9, 9},
			{9, 8, 9, 8, 8},
			{8, 7, 8, 7, 7},
			{7, 6, 7, 6, 6},
			{1, 10, 1, 1, 1},
		},
	)
	p, err := s.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		cp.Definition.RunID,
		f.revision(),
		Command{
			RequestID: uuid.New(),
			Version:   p.Round.Version,
			Action:    "sign_off",
			Reason:    "Exact qualification panel source for fresh final",
		},
	)
	if err != nil || !p.Current {
		t.Fatal(p, err)
	}
	criteria := rubric.Service{
		Pool:      f.pool,
		Clock:     func() time.Time { return f.now },
		Finalists: &s,
	}
	current, err := criteria.Configuration(t.Context(), f.owner.ID, f.event, 0)
	if err != nil {
		t.Fatal(err)
	}
	current, err = criteria.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		current.Revision,
		rubric.Command{
			Action:  "create_run",
			Purpose: "judge_final",
			Version: current.Reference.Version,
			Reason:  "Actual separate fresh judge final",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	var runID uuid.UUID
	for _, run := range current.Runs {
		if run.Purpose == "judge_final" {
			runID = run.ID
		}
	}
	judge := testdb.Person(t, f.pool)
	f.exec(
		"INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'judge')",
		f.event,
		judge.ID,
	)
	native := judging.Service{Pool: f.pool, Now: func() time.Time { return f.now }, Finalists: &s}
	for index, team := range f.teams {
		round, err := native.Round(t.Context(), f.owner.ID, f.event, runID)
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := native.ChangeRound(
			t.Context(),
			f.owner.ID,
			f.event,
			runID,
			round.Revision,
			judging.RoundCommand{
				RequestID: uuid.New(),
				Action:    "assign",
				TeamID:    team.team,
				JudgeIDs:  []uuid.UUID{judge.ID},
				Reason:    "Explicit independent final assignment",
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if receipt.Revision != round.Revision+1 {
			t.Fatal("native assignment revision", receipt)
		}
		brief, err := native.Brief(t.Context(), judge.ID, f.event, runID, team.team)
		if index >= 3 {
			if !errors.Is(err, access.ErrForbidden) {
				t.Fatal("reserve/category-only non-finalist acquired fresh assessment", index, err)
			}
			continue
		}
		if err != nil || brief.OwnCard != nil || len(brief.SubmittedCards) != 0 {
			t.Fatal("fresh judge inherited earlier scores", brief, err)
		}
		raw, _ := json.Marshal(brief)
		for _, hidden := range []string{"PRIVATE-SCORE-EVIDENCE-CANARY", "rankings", "finalist_ids", "moderated frozen-final category"} {
			if strings.Contains(string(raw), hidden) {
				t.Fatal("fresh briefing leaked qualification score/rank/prose", string(raw))
			}
		}
		scores := []judging.Score{}
		for _, category := range brief.Criteria.Categories {
			value := 8
			scores = append(
				scores,
				judging.Score{
					CategoryID: category.ID,
					Value:      &value,
					Evidence: []judging.Evidence{
						{Note: "Fresh independent judge evidence, no mentor score carryover"},
					},
				},
			)
		}
		if _, err = native.ChangeCard(
			t.Context(),
			judge.ID,
			f.event,
			runID,
			team.team,
			0,
			judging.CardCommand{
				RequestID:            uuid.New(),
				Action:               "submit",
				SubmissionRevision:   brief.SubmissionRevision,
				RepositoryDecisionID: brief.Repository.DecisionID,
				Scores:               scores,
				Reason:               "Independent fresh-final assessment",
			},
		); err != nil {
			t.Fatal("actual selected fresh scoring failed", err)
		}
	}
}
