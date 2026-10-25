package submissionfreeze

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The frontend corpus is checked by the actual native codec. Copying a field
// layout into a second Go struct would fail to detect native order changes.
func TestFrontendIntentFingerprintCorpus(t *testing.T) {
	raw, err := os.ReadFile("../../web/src/submission-freeze-fingerprint-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Command  Command `json:"command"`
		Revision int64   `json:"revision"`
		Expected string  `json:"expected"`
	}
	if err = json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 3 {
		t.Fatal("complete nil/empty/text codec corpus required")
	}
	for _, v := range vectors {
		v.Command.ExpectedRevision = v.Revision
		v.Command.Reason = strings.TrimSpace(v.Command.Reason)
		v.Command.ExternalEvidence = strings.TrimSpace(v.Command.ExternalEvidence)
		actual, err := fingerprint(v.Command)
		if err != nil || actual != v.Expected {
			t.Fatalf("native %s fingerprint mismatch: got %s, expected %s: %v", v.Command.Action, actual, v.Expected, err)
		}
	}
}
