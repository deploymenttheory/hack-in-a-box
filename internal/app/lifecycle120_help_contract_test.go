package app

import (
	"encoding/json"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/google/uuid"
)

func TestLifecycleLiveHelpWireUsesCanonicalAPISchema(t *testing.T) {
	swagger, err := contract.GetSwagger()
	if err != nil {
		t.Fatal(err)
	}
	visit := func(schema string, input any) error {
		t.Helper()
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		return swagger.Components.Schemas[schema].Value.VisitJSON(value)
	}
	if err := visit("HelpInput", lifecycleOutputHelpInput(uuid.New())); err != nil {
		t.Fatal("actual help create body violates native schema", err)
	}
	for _, action := range []string{"claim", "resolve"} {
		if err := visit("HelpCommand", lifecycleHelpAction(action)); err != nil {
			t.Fatal("actual help action violates native schema", err)
		}
	}
	missing := lifecycleHelpAction("claim")
	delete(missing, "assignee_id")
	if err := visit("HelpCommand", missing); err == nil {
		t.Fatal("native schema accepted omitted required nullable assignee identity")
	}
	legacy := help.Input{TeamID: uuid.New(), Category: "competition_output", Description: "Synthetic request", Location: "Synthetic desk", MeetingURL: "", CommandID: uuid.New()}
	if err := visit("HelpInput", legacy); err == nil {
		t.Fatal("native schema accepted header-owned command identity inside help body")
	}
}
