package results

import (
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/qualification"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

func TestCategoryAwardsKeepIndependentAllTeamWinners(t *testing.T) {
	s := source()
	addTeam(&s, teamA, [][2]int{{8, 2}, {8, 2}, {8, 2}})
	addTeam(&s, teamB, [][2]int{{2, 10}, {2, 10}, {2, 10}})
	tallies, err := Calculate(s)
	if err != nil {
		t.Fatal(err)
	}
	preview := Preview{Binding: "synthetic-source", Criteria: s.Criteria, Tallies: tallies}
	categories, err := categoryContenders(preview)
	if err != nil {
		t.Fatal(err)
	}
	if len(categories) != 2 || len(categories[0].TeamIDs) != 1 ||
		categories[0].TeamIDs[0] != teamA ||
		categories[1].TeamIDs[0] != teamB {
		t.Fatalf("category winners replaced by overall rank: %+v", categories)
	}
}

func TestQualificationCategorySourceRetainsANonFinalistWinner(t *testing.T) {
	p := Preview{
		Criteria: rubric.PublicDefinition{Mode: rubric.MentorQualification},
		Selection: &qualification.Selection{
			FinalistIDs: []uuid.UUID{teamA},
			Categories: []qualification.CategoryProposal{
				{CategoryID: categoryB, TeamIDs: []uuid.UUID{teamB}},
			},
		},
	}
	got, err := categoryContenders(p)
	if err != nil || len(got) != 1 || got[0].TeamIDs[0] != teamB {
		t.Fatalf("nonfinalist category winner lost: %+v %v", got, err)
	}
	p.Selection = nil
	if _, err = categoryContenders(p); err == nil {
		t.Fatal("missing native signed qualification source accepted")
	}
}

func TestTieIdentityBindsExactSourceAndKeepsNoImplicitWinner(t *testing.T) {
	a := makeTie("first-source", "final", nil, []uuid.UUID{teamB, teamA})
	b := makeTie("first-source", "final", nil, []uuid.UUID{teamA, teamB})
	c := makeTie("changed-source", "final", nil, []uuid.UUID{teamA, teamB})
	if a.ID != b.ID || a.ID == c.ID || len(a.TeamIDs) != 2 {
		t.Fatalf("unstable or source-free tie identity: %+v %+v %+v", a, b, c)
	}
}
