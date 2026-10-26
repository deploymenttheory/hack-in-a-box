package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/teamreports"
	"github.com/google/uuid"
)

func TestLifecycleTeamFeedbackCommandsUseCanonicalAPIJSON(t *testing.T) {
	swagger, err := contract.GetSwagger()
	if err != nil {
		t.Fatal(err)
	}
	schema := swagger.Components.Schemas["TeamFeedbackCommand"].Value
	source, scores := strings.Repeat("a", 64), strings.Repeat("b", 64)
	for _, input := range []struct{ action, score string }{{"save", ""}, {"save", scores}, {"release", ""}} {
		body := lifecycleTeamFeedbackCommand(uuid.New(), input.action, source, lifecycleTeamFeedbackBody(false), input.score)
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		var wire any
		if err := json.Unmarshal(raw, &wire); err != nil {
			t.Fatal(err)
		}
		if err := schema.VisitJSON(wire); err != nil {
			t.Fatalf("actual %s feedback factory violates native wire contract: %v", input.action, err)
		}
	}
	// A native domain command serializes save-only zero fields during release.
	// The independent API schema rejects it; the driver must use its actual
	// phase-specific wire factory rather than assume domain DTO equivalence.
	legacy := teamreports.Command{RequestID: uuid.New(), Action: "release", SourceBinding: source, Reason: "Explicit current report release"}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	var wire any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if err := schema.VisitJSON(wire); err == nil {
		t.Fatal("schema accepted save-only empty fields in a domain release DTO")
	}
}
