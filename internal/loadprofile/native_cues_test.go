package loadprofile

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/presentation"
)

// Synthetic HTTP controls verify original-response parsing only. A native
// presentation owner/audit/SSE run remains a separate acceptance requirement.
func TestCueOriginalResponseMustMatchExactReviewedCueAndRevision(t *testing.T) {
	for _, wrong := range []bool{false, true} {
		t.Run(map[bool]string{false: "exact", true: "other-cue"}[wrong], func(t *testing.T) {
			person, event := uuid.New(), uuid.New()
			cue := presentation.Cue{ID: uuid.New(), Title: "Synthetic public cue", Text: "Public fixture text", DurationSeconds: 60}
			writes := 0
			var marker string
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", "no-store")
				if r.URL.Path != "/api/v1/events/"+event.String()+"/presentation" {
					t.Error("cue retargeted another source", r.URL.Path)
				}
				page := presentation.Page{Revision: 1, CanControl: true}
				if r.Method == http.MethodPost {
					writes++
					if r.Header.Get("If-Match") != `"1"` || r.Header.Get("Idempotency-Key") != "" {
						t.Error("cue used another base or invented native idempotency")
					}
					var command presentation.Command
					if json.NewDecoder(r.Body).Decode(&command) != nil || command.Action != "override" ||
						command.Override == nil || *command.Override != cue {
						t.Error("reviewed public cue changed")
					}
					marker = command.Reason
					page.Revision, page.OverrideMode = 2, "running"
					copyOfCue := cue
					if wrong {
						copyOfCue.ID = uuid.New()
					}
					page.Override = &copyOfCue
				} else if r.Method != http.MethodGet {
					t.Error("unexpected cue method", r.Method)
				}
				_ = json.NewEncoder(w).Encode(page)
			}))
			defer server.Close()
			actor := Actor{ID: "synthetic-mc", Phase: SustainedPhase,
				Session: &Session{person: person, csrf: "synthetic-csrf", origin: server.URL, client: server.Client(), surface: AdminSurface}}
			var attempts AttemptLedger
			var metrics Metrics
			writer := NativeWrites{Attempts: &attempts, Metrics: &metrics}
			measurement, accepted, receipt, err := writer.Cue(t.Context(), actor, event, cue)
			if writes != 1 || measurement.Operation != CueWrite || measurement.Status != 200 {
				t.Fatal("cue write disappeared from accounting", writes, measurement)
			}
			if wrong {
				if err == nil || accepted != nil || receipt != nil || !attempts.ActorBlocked(person) {
					t.Fatal("current other cue became original ACK")
				}
				_, _, _, _ = writer.Cue(t.Context(), actor, event, cue)
				if writes != 1 {
					t.Fatal("unknown original cue was repeated")
				}
			} else if err != nil || accepted == nil || receipt == nil || receipt.Revision != 2 || receipt.Current.ID != cue.ID {
				t.Fatal("exact original cue response lost", accepted, receipt, err)
			}
			for _, spec := range attempts.Specs() {
				if marker != "load-cue:"+spec.Command.String()+":"+spec.Fingerprint || spec.Kind != CueEffect || spec.Version != 2 {
					t.Fatal("native immutable decision marker does not bind submitted cue", spec, marker)
				}
			}
		})
	}
}

func TestCommittedCueWithLostOriginalResponseCannotDisappearFromSSEOracle(t *testing.T) {
	spec := WriteSpec{Kind: CueEffect, Event: uuid.New(), Actor: uuid.New(), Command: uuid.New(), Version: 3, Fingerprint: strings.Repeat("a", 64)}
	spec.Resource = spec.Event
	id := uuid.New()
	specs := map[uuid.UUID]WriteSpec{id: spec}
	facts := map[uuid.UUID]EffectFact{id: {Spec: spec, Present: true, Effects: 1, Fingerprint: spec.Fingerprint}}
	if report := CueSources(specs, facts, DisplayReport{}); report.Complete() || report.CommittedSources != 1 {
		t.Fatal("committed unknown-ACK publication disappeared", report)
	}
	matched := DisplayReport{Publications: 1, ExpectedReceipts: 2, MatchedReceipts: 2}
	if report := CueSources(specs, facts, matched); !report.Complete() {
		t.Fatal("exact independently reconciled source/receipts missing", report)
	}
	matched.MismatchedReceipts = 1
	if report := CueSources(specs, facts, matched); report.Complete() {
		t.Fatal("wrong-body receipt became complete source proof", report)
	}
}
