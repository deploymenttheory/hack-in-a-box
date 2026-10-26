package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/results"
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
