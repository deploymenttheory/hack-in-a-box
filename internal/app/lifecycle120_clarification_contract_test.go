package app

import (
	"encoding/json"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/google/uuid"
)

func TestLifecycleClarificationWireUsesCanonicalAPISchema(t *testing.T) {
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
		var wire any
		if err := json.Unmarshal(raw, &wire); err != nil {
			t.Fatal(err)
		}
		return swagger.Components.Schemas[schema].Value.VisitJSON(wire)
	}
	question := lifecyclePrivateQuestion(uuid.New(), uuid.New())
	if err := visit("PrivateQuestionInput", question); err != nil {
		t.Fatal("actual private question violates native wire schema", err)
	}
	for _, corrected := range []bool{false, true} {
		if err := visit("ClarificationDocument", lifecycleCommonAnswer(corrected)); err != nil {
			t.Fatal("actual common answer violates native wire schema", err)
		}
	}
	for _, action := range []string{"approve", "publish"} {
		if err := visit("ClarificationCommand", clarification.AnswerCommand{Action: action, Version: 1}); err != nil {
			t.Fatal("actual review action violates native wire schema", err)
		}
	}
	raw, err := json.Marshal(question)
	if err != nil {
		t.Fatal(err)
	}
	var missing map[string]any
	if err := json.Unmarshal(raw, &missing); err != nil {
		t.Fatal(err)
	}
	delete(missing, "attachments")
	if err := visit("PrivateQuestionInput", missing); err == nil {
		t.Fatal("schema accepted missing required attachments array")
	}
	wrong := map[string]any{"title": "Common clarification", "body": "Actual reviewed clarification", "kind": "clarification", "question_id": uuid.New(), "source": map[string]any{}}
	if err := visit("ClarificationDocument", wrong); err == nil {
		t.Fatal("common answer accepted private question identity or caller-owned source override")
	}
}
