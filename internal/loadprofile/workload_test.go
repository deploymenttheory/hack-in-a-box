package loadprofile

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Synthetic HTTPS checks driver dispatch only. It cannot establish native
// authority, original acknowledgement, persistence or a passing load profile.
func TestCurrentReadCannotUnblockUnknownOriginalRoleWrite(t *testing.T) {
	person, event, team := uuid.New(), uuid.New(), uuid.New()
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/session" {
			t.Error("unknown actor wrote or retargeted", r.Method, r.URL.Path)
		}
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"person": map[string]any{"id": person}, "csrf_token": strings.Repeat("c", 32),
		})
	}))
	defer server.Close()
	actor := Actor{ID: "synthetic-mentor", Phase: SustainedPhase,
		Session: &Session{person: person, csrf: strings.Repeat("c", 32), origin: server.URL,
			client: server.Client(), surface: PublicSurface}}
	var attempts AttemptLedger
	spec := syntheticWrite()
	spec.Actor, spec.Event, spec.Team = person, event, team
	id, err := attempts.Begin(spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := attempts.Finish(id, 0, false); err != nil {
		t.Fatal(err)
	}
	var metrics Metrics
	workload, err := NewNativeWorkload(map[string]Journey{actor.ID: {
		Read:        ReadBinding{Event: event, Team: team, Operations: []Operation{SessionRead}},
		Observation: uuid.New(), RequestHelp: true, WorkHelpQueue: true,
	}}, NativeWrites{Attempts: &attempts, Metrics: &metrics}, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	for _, sequence := range []uint64{2, 7, 11} {
		measurement, accepted, err := workload.Action(t.Context(), actor, sequence)
		if err != nil || accepted != nil || measurement.Operation != SessionRead || measurement.Outcome != Accepted {
			t.Fatal("ordinary read did not remain separate", measurement, accepted, err)
		}
		if !attempts.ActorBlocked(person) || len(attempts.Specs()) != 1 {
			t.Fatal("current read inferred original ACK or permitted later write")
		}
	}
	if requests != 3 {
		t.Fatal("read requests retried or hidden", requests)
	}
}
