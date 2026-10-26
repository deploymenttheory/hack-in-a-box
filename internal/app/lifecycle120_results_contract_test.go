package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/qualification"
	"github.com/deploymenttheory/hack-in-a-box/internal/results"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/google/uuid"
)

func TestLifecycleResultBallotUsesCanonicalAPIJSON(t *testing.T) {
	id, teamID := uuid.New(), uuid.New()
	page := results.BallotPage{Version: 7, SourceBinding: strings.Repeat("a", 64)}
	body := lifecycleResultBallotBody(id, page, strings.Repeat("b", 64), teamID)
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	swagger, err := contract.GetSwagger()
	if err != nil {
		t.Fatal(err)
	}
	schema := swagger.Components.Schemas["ResultVoteCommand"].Value
	if err := schema.VisitJSON(wire); err != nil {
		t.Fatalf("actual positive and denied ballot factory violates native API: %v", err)
	}
	// Reproduce the actual native 400 cause independently of role authority.
	evidence := wire["evidence"].([]any)[0].(map[string]any)
	delete(evidence, "deliverable_id")
	if err := schema.VisitJSON(wire); err == nil {
		t.Fatal("schema accepted the omitted required nullable evidence identity")
	}
}

func TestLifecycleResultAwardBodyPreservesModeSpecificCategoryOwner(t *testing.T) {
	swagger, err := contract.GetSwagger()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{rubric.AllTeams, rubric.MentorQualification} {
		t.Run(mode, func(t *testing.T) {
			definition := rubric.Default(mode, uuid.New())
			categoryID := definition.Categories[0].ID
			overall, categoryWinner := uuid.New(), uuid.New()
			definition.Prizes = []rubric.Prize{{ID: uuid.New(), Name: "Overall"}, {ID: uuid.New(), Name: "Category", CategoryID: &categoryID}}
			page := results.Page{Preview: results.Preview{Criteria: definition.PublicDefinition, Binding: strings.Repeat("a", 64)}}
			page.TieStates = []results.TieState{{Tie: results.Tie{Kind: "final"}, WinnerID: &overall}}
			if mode == rubric.AllTeams {
				page.TieStates = append(page.TieStates, results.TieState{Tie: results.Tie{Kind: "category", CategoryID: &categoryID}, WinnerID: &categoryWinner})
			} else {
				// Deliberately distinct: a signed non-finalist category winner must
				// not be replaced by the fresh final judge winner or a fabricated tie.
				page.Preview.Selection = &qualification.Selection{SignedBinding: strings.Repeat("b", 64), Categories: []qualification.CategoryProposal{{CategoryID: categoryID, TeamIDs: []uuid.UUID{categoryWinner}}}}
			}
			awards := lifecycleSyntheticAwards(t, page)
			if awards[0].TeamID != overall || awards[0].CategoryID != nil || awards[1].TeamID != categoryWinner || awards[1].CategoryID == nil || *awards[1].CategoryID != categoryID {
				t.Fatal("award wire factory conflated final judge and native category ownership")
			}
			body := lifecycleResultCommandBody(uuid.New(), page, map[string]any{"action": "confirm", "source_binding": page.Preview.Binding, "awards": awards})
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			var wire any
			if err := json.Unmarshal(raw, &wire); err != nil {
				t.Fatal(err)
			}
			if err := swagger.Components.Schemas["ResultCommand"].Value.VisitJSON(wire); err != nil {
				t.Fatalf("actual mode-specific confirmation violates native schema: %v", err)
			}
		})
	}
}
