package loadprofile

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/help"
)

// This is a synthetic HTTP contract control, not native application, session,
// transaction or load proof. It checks that the driver cannot conceal its own
// create/resolve traffic or recover a lost original ACK by repeating a write.
func TestHelpRuntimeWritesKeepOriginalCommandAndExactEffect(t *testing.T) {
	for _, action := range []string{"create", "resolve"} {
		for _, malformed := range []bool{false, true} {
			t.Run(action+map[bool]string{false: "-exact", true: "-other-effect"}[malformed], func(t *testing.T) {
				person, event, team, request := uuid.New(), uuid.New(), uuid.New(), uuid.New()
				input := help.Input{TeamID: team, Category: "setup", Description: "Synthetic component request", Location: "Fixture desk"}
				calls := 0
				var command uuid.UUID
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					w.Header().Set("Cache-Control", "no-store")
					var err error
					command, err = uuid.Parse(r.Header.Get("Idempotency-Key"))
					if err != nil || command == uuid.Nil || r.Header.Get("X-CSRF-Token") != "synthetic-csrf" {
						t.Error("native command identity/CSRF missing")
					}
					page := help.Page{Request: help.Request{ID: request, EventID: event, TeamID: team, RequesterID: person, Revision: 1,
						Category: input.Category, Description: input.Description, Location: input.Location, Status: "waiting"}}
					path := "/api/v1/events/" + event.String() + "/help/" + request.String()
					if action == "resolve" {
						path += "/actions"
						if r.Method != http.MethodPost || r.Header.Get("If-Match") != `"2"` {
							t.Error("resolution retargeted captured revision", r.Method, r.Header.Get("If-Match"))
						}
						now := time.Now()
						page.Request.Revision, page.Request.Status = 3, "resolved"
						page.Request.AssigneeID, page.Request.ResolvedAt = &person, &now
					} else if r.Method != http.MethodPut || r.Header.Get("If-Match") != `"0"` {
						t.Error("creation did not use native zero revision", r.Method, r.Header.Get("If-Match"))
					}
					if r.URL.Path != path {
						t.Error("wrong native path", r.URL.Path)
					}
					if malformed {
						page.Request.ID = uuid.New()
					}
					_ = json.NewEncoder(w).Encode(page)
				}))
				defer server.Close()
				actor := Actor{ID: person.String(), Phase: SustainedPhase, Session: &Session{origin: server.URL, client: server.Client(), person: person, csrf: "synthetic-csrf", surface: PublicSurface}}
				var attempts AttemptLedger
				writer := NativeWrites{Attempts: &attempts}
				write := func() (Measurement, *AcceptedWrite, error) {
					if action == "create" {
						return writer.HelpCreate(t.Context(), actor, event, request, input)
					}
					return writer.HelpResolve(t.Context(), actor, event, request, 2)
				}
				measurement, accepted, err := write()
				if calls != 1 || measurement.Operation != HelpWrite || measurement.Status != 200 {
					t.Fatal("runtime help write disappeared", measurement, calls)
				}
				if malformed {
					if err == nil || accepted != nil || !attempts.ActorBlocked(person) {
						t.Fatal("other current effect inferred original ACK")
					}
					_, _, _ = write()
					if calls != 1 {
						t.Fatal("uncertain command replayed or retargeted")
					}
				} else if err != nil || accepted == nil || measurement.Outcome != Accepted {
					t.Fatal("exact original help response not retained", err, measurement)
				}
				for _, spec := range attempts.Specs() {
					if spec.Command != command || spec.Action != action || spec.Resource != request {
						t.Fatal("immutable command/action identity lost", spec)
					}
				}
			})
		}
	}
}
