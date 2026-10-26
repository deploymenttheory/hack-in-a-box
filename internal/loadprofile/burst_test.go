package loadprofile

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
)

// Synthetic HTTPS verifies the burst barrier, original response accounting
// and lack of hidden retries. This is not the native registration owner or a
// burst-capacity result, and its inputs never become a release report.
func TestBurstReleasesConcurrentActorsAndRetainsAcceptedWaitlistWrites(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	event, form := uuid.New(), uuid.New()
	var tasks []RegistrationTask
	people := make(map[uuid.UUID]uuid.UUID)
	for index := range 3 {
		id, person := uuid.New(), uuid.New()
		people[id] = person
		tasks = append(tasks, RegistrationTask{
			Actor: Actor{ID: string(rune('a' + index)), Session: &Session{person: person, csrf: "synthetic-csrf", surface: PublicSurface}},
			Input: registration.Application{ID: id, FormID: form, FormVersion: 1, TeamPreference: "join"},
		})
	}
	var received atomic.Int32
	allStarted := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/events/"+event.String()+"/registrations" {
			t.Error("burst retargeted native operation", r.Method, r.URL.Path)
		}
		var input registration.Application
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		sequence := received.Add(1)
		if sequence == 3 {
			close(allStarted)
		}
		select {
		case <-allStarted:
		case <-ctx.Done():
			return
		}
		_ = json.NewEncoder(w).Encode(registration.View{
			ID: input.ID, EventID: event, PersonID: people[input.ID], FormID: input.FormID,
			FormVersion: input.FormVersion, Kind: "attendee", Audience: "attendee",
			Sequence: int64(sequence), Revision: 1, State: "waitlisted", TeamPreference: input.TeamPreference,
		})
	}))
	defer server.Close()
	for index := range tasks {
		tasks[index].Actor.Session.origin = server.URL
		tasks[index].Actor.Session.client = server.Client()
	}
	var metrics Metrics
	var attempts AttemptLedger
	var ledger WriteLedger
	result, err := RunRegistrationBurst(ctx, BurstConfig{Profile: "smoke", Event: event, Form: form, FormVersion: 1},
		tasks, NativeWrites{Attempts: &attempts, Metrics: &metrics}, &ledger)
	if err != nil || !result.Completed || result.Interrupted || result.Accepted != 3 ||
		result.FailedActions != 0 || result.MaximumInflight != 3 || received.Load() != 3 {
		t.Fatal("burst barrier or waitlist ACK accounting failed", result, err, received.Load())
	}
	reports := metrics.Reports()
	if len(reports) != 1 || reports[0].Phase != BurstPhase || reports[0].Count != 3 || reports[0].Outcomes[Accepted] != 3 {
		t.Fatal("accepted waitlist writes became capacity rejections", reports)
	}
	if integrity := ledger.Reconcile(nil); integrity.Accepted != 3 || integrity.Missing != 3 {
		t.Fatal("unreconciled accepted writes disappeared", integrity)
	}
}

func TestReleaseBurstCannotShrinkOrDuplicateItsNativePeople(t *testing.T) {
	event, form, person := uuid.New(), uuid.New(), uuid.New()
	task := RegistrationTask{Actor: Actor{ID: "synthetic", Session: &Session{person: person}},
		Input: registration.Application{ID: uuid.New(), FormID: form, FormVersion: 1}}
	if err := (BurstConfig{Profile: "release", Event: event, Form: form, FormVersion: 1}).validate([]RegistrationTask{task}); err == nil {
		t.Fatal("one request became the declared 200-person release burst")
	}
	other := task
	other.Input.ID = uuid.New()
	other.Actor.ID = "second-label-same-person"
	if err := (BurstConfig{Profile: "smoke", Event: event, Form: form, FormVersion: 1}).validate([]RegistrationTask{task, other}); err == nil {
		t.Fatal("duplicate native person became concurrent actors")
	}
}
