package app

import (
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Diagnostic only: normal OIDC and one actual native read against a separately
// retained synthetic run. No setup SQL, new draft or replayed command.
func TestRetainedScoringQueryProbe(t *testing.T) {
	if os.Getenv("HACKBOX_LIFECYCLE_ISOLATED") != "synthetic-only" ||
		os.Getenv("HACKBOX_SCORING_PROBE") != "read-only" {
		t.Skip("explicit retained synthetic read-only probe only")
	}
	eventID, err := uuid.Parse(os.Getenv("HACKBOX_SCORING_PROBE_EVENT"))
	if err != nil {
		t.Fatal("explicit retained event required")
	}
	teamID, err := uuid.Parse(os.Getenv("HACKBOX_SCORING_PROBE_TEAM"))
	if err != nil {
		t.Fatal("explicit retained team required")
	}
	owner, err := signInLifecycle(t.Context(), "http://admin:8080", "http://provider:8080", "owner")
	if err != nil {
		t.Fatal(err)
	}
	began := time.Now()
	if err := owner.call(
		t.Context(),
		http.MethodGet,
		"/api/v1/events/"+eventID.String()+"/teams/"+teamID.String()+"/feedback-report",
		-1,
		nil,
		nil,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	t.Logf("actual native private feedback-source read elapsed=%s", time.Since(began))
}
