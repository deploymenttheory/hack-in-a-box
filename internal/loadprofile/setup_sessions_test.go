package loadprofile

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

// Real HTTPS transport accounting only: synthetic handler responses do not
// prove application authentication or logout. A lost original cleanup response
// must not become acknowledged merely because its handler already ran.
func TestSetupSessionCleanupRetainsUnknownResponseWithoutReplay(t *testing.T) {
	var accepted, unknown atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/logout" {
			t.Error("cleanup left normal native logout path")
		}
		if r.Header.Get("X-CSRF-Token") == "synthetic-known" {
			accepted.Add(1)
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		unknown.Add(1)
		connection, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = connection.Close()
	}))
	defer server.Close()
	actors := []CohortActor{
		{
			Actor: Actor{
				ID: "known",
				Session: &Session{
					origin: server.URL,
					client: server.Client(),
					person: uuid.New(),
					csrf:   "synthetic-known",
				},
			},
		},
		{
			Actor: Actor{
				ID: "unknown",
				Session: &Session{
					origin: server.URL,
					client: server.Client(),
					person: uuid.New(),
					csrf:   "synthetic-unknown",
				},
			},
		},
	}
	receipt, calls, err := closeSetupSessions(t.Context(), actors)
	if err == nil || accepted.Load() != 1 || unknown.Load() != 1 || len(calls) != 2 ||
		receipt.Attempted != 2 || receipt.OriginalACKs != 1 || receipt.Unresolved != 1 ||
		receipt.Statuses[http.StatusNoContent] != 1 || receipt.Statuses[0] != 1 ||
		calls[1].OriginalComplete {
		t.Fatal("cleanup uncertainty was hidden or replayed", receipt, err)
	}
}
