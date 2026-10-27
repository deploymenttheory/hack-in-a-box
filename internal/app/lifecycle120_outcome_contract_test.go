package app

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

// Validate the actual journey factory against the independently generated API
// contract: a domain-valid nil slice is not a valid non-null wire array.
func TestLifecycleOutcomeEvidenceUsesCanonicalAPIJSON(t *testing.T) {
	swagger, err := contract.GetSwagger()
	if err != nil {
		t.Fatal(err)
	}
	schema := swagger.Components.Schemas["OutcomeEvidence"].Value
	observed := time.Date(2027, 10, 23, 12, 0, 0, 0, time.UTC)
	for _, basis := range []string{"estimate", "self_report", "demonstrated_adoption"} {
		t.Run(basis, func(t *testing.T) {
			var at *time.Time
			source := ""
			if basis == "demonstrated_adoption" {
				at, source = &observed, "https://evidence.example.test/observations/test"
			}
			input := lifecycleOutcomeEvidence(basis, "Synthetic follow-up observation", source, at)
			if err := input.Validate(); err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			var wire any
			if err := json.Unmarshal(raw, &wire); err != nil {
				t.Fatal(err)
			}
			if err := schema.VisitJSON(wire); err != nil {
				t.Fatalf("actual %s evidence factory violates native API schema: %v", basis, err)
			}
		})
	}
}
