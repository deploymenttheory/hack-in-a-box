package results

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

var (
	categoryA = uuid.MustParse("00000000-0000-4000-8000-000000000001")
	categoryB = uuid.MustParse("00000000-0000-4000-8000-000000000002")
	teamA     = uuid.MustParse("00000000-0000-4000-8000-000000000010")
	teamB     = uuid.MustParse("00000000-0000-4000-8000-000000000020")
)

func source() JudgeSource {
	return JudgeSource{
		RunID: uuid.New(),
		Reference: rubric.Reference{
			ID:      uuid.New(),
			Version: 1,
			Digest:  "synthetic-public-reference",
		},
		Criteria: rubric.PublicDefinition{
			Name:               "Synthetic final",
			Mode:               rubric.AllTeams,
			RequiredJudgeCount: 3,
			Categories: []rubric.Category{
				{ID: categoryA, Label: "Demo", MinScore: 1, MaxScore: 10, Weight: 2},
				{ID: categoryB, Label: "Technical", MinScore: 1, MaxScore: 10, Weight: 1},
			},
		},
	}
}

func addTeam(s *JudgeSource, team uuid.UUID, values [][2]int) {
	assignment := judging.Assignment{TeamID: team, JudgeIDs: []uuid.UUID{}}
	for _, pair := range values {
		author := uuid.New()
		assignment.JudgeIDs = append(assignment.JudgeIDs, author)
		a, b := pair[0], pair[1]
		scores := []judging.Score{{CategoryID: categoryA, Value: &a}}
		if len(s.Criteria.Categories) > 1 {
			scores = append(scores, judging.Score{CategoryID: categoryB, Value: &b})
		}
		s.Cards = append(
			s.Cards,
			judging.Card{
				RunID:              s.RunID,
				TeamID:             team,
				AuthorID:           author,
				State:              "submitted",
				Version:            1,
				SubmissionRevision: 1,
				Rubric:             s.Reference,
				Scores:             scores,
			},
		)
	}
	s.Assignments = append(s.Assignments, assignment)
}

func TestCategoryAveragesAndWeightsRetainExactRecurringValues(t *testing.T) {
	s := source()
	addTeam(&s, teamA, [][2]int{{1, 2}, {2, 3}, {2, 4}})
	got, err := Calculate(s)
	if err != nil || len(got) != 1 {
		t.Fatalf("calculate: %+v %v", got, err)
	}
	if got[0].Categories[0].Average != (Exact{"5", "3", "1.67"}) ||
		got[0].Total != (Exact{"19", "3", "6.33"}) ||
		got[0].RequiredJudges != 3 {
		t.Fatalf("exact weighted means lost: %+v", got[0])
	}
}

func TestRoundedNearTiesRemainDistinct(t *testing.T) {
	s := source()
	s.Criteria.RequiredJudgeCount = 1
	s.Criteria.Categories = s.Criteria.Categories[:1]
	s.Criteria.Categories[0].Weight = 1
	for _, item := range []struct {
		id     uuid.UUID
		judges int
	}{{teamA, 49}, {teamB, 50}} {
		values := make([][2]int, item.judges)
		for i := range values {
			values[i][0] = 5
		}
		values[0][0] = 6
		addTeam(&s, item.id, values)
	}
	got, err := Calculate(s)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].TeamID != teamA || got[0].Total.Display != "5.02" || got[1].Total.Display != "5.02" ||
		got[0].Rank != 1 ||
		got[1].Rank != 2 {
		t.Fatalf("display rounding affected rank: %+v", got)
	}
	if got[0].Total.Numerator != "246" || got[0].Total.Denominator != "49" ||
		got[1].Total.Numerator != "251" ||
		got[1].Total.Denominator != "50" {
		t.Fatalf("source precision lost: %+v", got)
	}
}

func TestExactTiesRemainTiesRegardlessOfUUIDOrder(t *testing.T) {
	s := source()
	addTeam(&s, teamB, [][2]int{{5, 5}, {5, 5}, {5, 5}})
	addTeam(&s, teamA, [][2]int{{5, 5}, {5, 5}, {5, 5}})
	got, err := Calculate(s)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].TeamID != teamA || got[0].Rank != 1 || got[1].Rank != 1 ||
		got[0].Total != got[1].Total {
		t.Fatalf("unrecorded tie resolution: %+v", got)
	}
}

func TestIncompleteSourcesNeverReduceTheRequiredDenominator(t *testing.T) {
	cases := map[string]func(*JudgeSource){
		"missing assigned author despite minimum count": func(s *JudgeSource) { s.Criteria.RequiredJudgeCount = 2; s.Cards = s.Cards[:2] },
		"missing category":       func(s *JudgeSource) { s.Cards[0].Scores = s.Cards[0].Scores[:1] },
		"null category":          func(s *JudgeSource) { s.Cards[0].Scores[0].Value = nil },
		"duplicate category":     func(s *JudgeSource) { s.Cards[0].Scores[1] = s.Cards[0].Scores[0] },
		"out of range":           func(s *JudgeSource) { v := 11; s.Cards[0].Scores[0].Value = &v },
		"duplicate author":       func(s *JudgeSource) { s.Cards[1].AuthorID = s.Cards[0].AuthorID },
		"extra author":           func(s *JudgeSource) { c := s.Cards[0]; c.AuthorID = uuid.New(); s.Cards = append(s.Cards, c) },
		"duplicate assignment":   func(s *JudgeSource) { s.Assignments = append(s.Assignments, s.Assignments[0]) },
		"wrong rubric":           func(s *JudgeSource) { s.Cards[0].Rubric.Version++ },
		"wrong run":              func(s *JudgeSource) { s.Cards[0].RunID = uuid.New() },
		"saved draft":            func(s *JudgeSource) { s.Cards[0].State = "draft" },
		"declared missing judge": func(s *JudgeSource) { s.Assignments[0].Missing = 1 },
		"unassigned team":        func(s *JudgeSource) { c := s.Cards[0]; c.TeamID = uuid.New(); s.Cards = append(s.Cards, c) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := source()
			addTeam(&s, teamA, [][2]int{{5, 5}, {5, 5}, {5, 5}})
			mutate(&s)
			got, err := Calculate(s)
			if !errors.Is(err, ErrIncomplete) || got != nil {
				t.Fatalf("accepted incomplete source: %+v %v", got, err)
			}
		})
	}
}
