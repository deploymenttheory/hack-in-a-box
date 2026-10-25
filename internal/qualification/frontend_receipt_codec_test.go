package qualification

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
)

// Generate the frontend corpus from real accepted native commands and their
// immutable receipts, rather than a second Go Command/fingerprint implementation.
// Optional output is an explicitly named test artifact; ordinary runs write none.
func TestFrontendQualificationCodecUsesActualAcceptedOriginalReceipts(t *testing.T) {
	f, service, run := receiptFixture(t)
	type vector struct {
		Command  Command `json:"command"`
		Revision int64   `json:"revision"`
		Receipt  Receipt `json:"receipt"`
	}
	vectors := []vector{}
	for index, reason := range []string{
		"  Native <panel> & reason\u2028\u2029😀\u0085\ufeff  ",
		"Explicit empty evidence is a distinct captured intent",
		"Native nil evidence remains a distinct captured intent",
	} {
		command := panelCommand(f)
		command.Reason = reason
		command.RequestID = uuid.New()
		command.Version = int64(index)
		if index == 1 {
			command.Evidence = []scorecards.EvidenceReference{}
		}
		revision := f.revision()
		page, err := service.Change(t.Context(), f.owner.ID, f.event, run, revision, command)
		if err != nil || page.Receipt == nil {
			t.Fatal("actual native command rejected", index, page, err)
		}
		receipt := *page.Receipt
		if receipt.RequestID != command.RequestID || receipt.ExpectedVersion != int64(index) ||
			receipt.ExpectedRevision != revision || receipt.Version != int64(index+1) ||
			receipt.RunID != run || len(receipt.Fingerprint) != 64 {
			t.Fatal("native original identity mismatch", receipt)
		}
		original, err := service.OriginalReceipt(t.Context(), f.owner.ID, f.event, run, command.RequestID)
		if err != nil || original.Receipt == nil || *original.Receipt != receipt {
			t.Fatal("actual original recovery changed receipt", original, err)
		}
		vectors = append(vectors, vector{Command: command, Revision: revision, Receipt: receipt})
	}
	if path := os.Getenv("HACKBOX_QUALIFICATION_CODEC_OUTPUT"); path != "" {
		raw, err := json.MarshalIndent(vectors, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
