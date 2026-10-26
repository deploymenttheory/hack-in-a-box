package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissionfreeze"
)

// Check the driver's actual wire-body factory against the independent native
// API schema. Domain command structs do not automatically have its JSON contract.
func TestLifecycleCutoffReviewUsesCanonicalAPIJSON(t *testing.T) {
	revision := int64(1)
	page := submissionfreeze.Page{NativeRevision: 2, CurrentSubmissionRevision: &revision, Source: &submissionfreeze.Source{ID: uuid.New()}}
	work := lifecycleWorkReceipt{lifecycleExternalWorkReceipt: lifecycleExternalWorkReceipt{ID: uuid.New(), SHA: strings.Repeat("a", 40)}, DomainObservedAt: time.Date(2026, 3, 23, 9, 0, 0, 0, time.UTC)}
	command := lifecycleCutoffReviewCommand(page, work)
	if command.SourceID != page.Source.ID || command.NativeRevision != 2 || command.SubmissionRevision != 1 || command.SHA != work.SHA || command.RequestID == uuid.Nil {
		t.Fatal("wire command lost the actual current native source or reviewed work identity")
	}
	raw, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	var body any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	swagger, err := contract.GetSwagger()
	if err != nil {
		t.Fatal(err)
	}
	if err := swagger.Components.Schemas["SubmissionFreezeCommand"].Value.VisitJSON(body); err != nil {
		t.Fatalf("actual cutoff review command violates native wire schema: %v", err)
	}
}
