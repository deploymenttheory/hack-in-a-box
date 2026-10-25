package app

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
	"github.com/google/uuid"
)

// This reuses the reviewed native runtime/router/static portal surface. Control
// IPC has a private Unix socket only; no fixture route exists in either portal.
func TestSessionBookingBrowsers(t *testing.T) {
	if os.Getenv("HACKBOX_SESSION_BROWSER") != "1" {
		t.Skip("set HACKBOX_INTEGRATION=1 HACKBOX_SESSION_BROWSER=1 after building both portals")
	}
	for _, viewport := range []string{"desktop", "mobile"} {
		t.Run(viewport, func(t *testing.T) { runSessionBookingBrowser(t, viewport) })
	}
}
func runSessionBookingBrowser(t *testing.T, viewport string) {
	t.Helper()
	ctx := t.Context()
	f := newSessionNotificationFixture(t)
	second, facilitator := testdb.Person(t, f.pool), testdb.Person(t, f.pool)
	q := dbgen.New(f.pool)
	if err := q.SetMembership(ctx, dbgen.SetMembershipParams{EventID: f.eventID, PersonID: facilitator.ID, Role: "mentor", Active: true}); err != nil {
		t.Fatal(err)
	}
	registrationRow, err := q.Registration(ctx, dbgen.RegistrationParams{EventID: f.eventID, ID: f.registrationID})
	if err != nil {
		t.Fatal(err)
	}
	applied, err := f.registration.Apply(ctx, second.ID, f.eventID, registration.Application{ID: uuid.New(), FormID: registrationRow.FormID, FormVersion: registrationRow.FormVersion, Answers: forms.Answers{}, TeamPreference: "solo"})
	if err != nil || applied.State != "confirmed" {
		t.Fatal("native second admission fixture", err)
	}
	current, err := f.events.Read(ctx, f.owner.ID, f.eventID)
	if err != nil {
		t.Fatal(err)
	}
	newID := uuid.New()
	configuration := current.Configuration
	configuration.Schedule[0].Title = "Synthetic native workshop"
	configuration.Schedule[0].Audience = []string{"all"}
	configuration.Schedule = append(configuration.Schedule, event.AgendaItem{ID: newID, Title: "Synthetic native office hour", Audience: []string{"all"}, Anchor: "live_start", Minutes: 30})
	if _, err = f.events.Update(ctx, f.owner.ID, f.eventID, current.Revision, event.Input{ID: f.eventID, Name: "Synthetic session browser edition", Slug: current.Slug, Configuration: configuration, Reason: "Synthetic native session browser fixture"}); err != nil {
		t.Fatal(err)
	}
	r := &liveRehearsal{pool: f.pool, clock: &rehearsalClock{now: f.now}, owner: f.owner, competitor: f.person, mentor: facilitator, eventID: f.eventID}
	admin, public := r.surface(t, "admin"), r.surface(t, "public")
	initial, err := admin.deps.SessionBookings.Read(ctx, f.owner.ID, f.eventID, f.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	definition := *initial.Definition
	definition.FacilitatorIDs = []uuid.UUID{facilitator.ID}
	definition.Room = "Synthetic native lab"
	definition.MeetingURL = "https://meeting.example.test/synthetic-native-workshop"
	if _, err = admin.deps.SessionBookings.Configure(ctx, f.owner.ID, f.eventID, f.sessionID, initial.Revision, definition); err != nil {
		t.Fatal(err)
	}
	people := map[string]dbgen.Person{"first": f.person, "second": second, "facilitator": facilitator, "owner": f.owner}
	registrations := map[string]uuid.UUID{"first": f.registrationID, "second": applied.ID}
	private, err := os.MkdirTemp("/tmp", "hackbox167-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(private) })
	private, err = filepath.EvalSymlinks(private)
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(private, "control.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	control := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(w, "POST required", 405)
			return
		}
		var input struct {
			Action, Actor, Surface string
			Capacity               int
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, request.Body, 1024)).Decode(&input); err != nil {
			http.Error(w, "invalid control", 400)
			return
		}
		var actionErr error
		switch input.Action {
		case "session":
			person, ok := people[input.Actor]
			if !ok {
				http.Error(w, "unknown synthetic actor", 400)
				return
			}
			surface := public
			if input.Surface == "admin" {
				surface = admin
			} else if input.Surface != "public" {
				http.Error(w, "unknown native surface", 400)
				return
			}
			// Bootstrap and current identity changes advance the native session revision.
			// Issue the synthetic session from the current repository row, exactly as
			// production authentication requires; stale fixture snapshots stay invalid.
			person, err := q.Person(request.Context(), person.ID)
			if err != nil {
				http.Error(w, "current synthetic actor unavailable", 500)
				return
			}
			cookie, err := surface.sessionCookie(request.Context(), person)
			if err != nil {
				http.Error(w, "synthetic session unavailable", 500)
				return
			}
			_ = json.NewEncoder(w).Encode(cookie)
			return
		case "start":
			page, err := admin.deps.SessionBookings.Read(request.Context(), f.owner.ID, f.eventID, f.sessionID)
			if err != nil {
				actionErr = err
				break
			}
			r.clock.set(page.Start.Add(time.Minute))
		case "withdraw":
			person, ok := people[input.Actor]
			id, registered := registrations[input.Actor]
			if !ok || !registered {
				http.Error(w, "unknown admitted fixture", 400)
				return
			}
			row, err := q.Registration(request.Context(), dbgen.RegistrationParams{EventID: f.eventID, ID: id})
			if err != nil {
				actionErr = err
				break
			}
			_, actionErr = admin.deps.Registration.Respond(request.Context(), person.ID, f.eventID, id, row.Revision, "withdraw")
		case "remove-facilitator", "capacity":
			page, err := admin.deps.SessionBookings.Read(request.Context(), f.owner.ID, f.eventID, f.sessionID)
			if err != nil {
				actionErr = err
				break
			}
			d := *page.Definition
			if input.Action == "remove-facilitator" {
				d.FacilitatorIDs = []uuid.UUID{}
			} else {
				d.Capacity = input.Capacity
			}
			_, actionErr = admin.deps.SessionBookings.Configure(request.Context(), f.owner.ID, f.eventID, f.sessionID, page.Revision, d)
		case "move-session":
			page, err := admin.deps.Events.Read(request.Context(), f.owner.ID, f.eventID)
			if err != nil {
				actionErr = err
				break
			}
			c := page.Configuration
			c.Schedule[0].Minutes += 30
			_, actionErr = admin.deps.Events.Update(request.Context(), f.owner.ID, f.eventID, page.Revision, event.Input{ID: f.eventID, Name: page.Name, Slug: page.Slug, Configuration: c, Reason: "Synthetic reviewed date correction"})
		case "evidence":
			var booked, confirmed, members, commands int64
			for _, query := range []struct {
				sql    string
				target *int64
			}{{"SELECT count(*) FROM session_bookings WHERE event_id=$1 AND session_id=$2 AND state='booked'", &booked}, {"SELECT count(*) FROM competition_claims WHERE event_id=$1 AND $2::uuid IS NOT NULL AND state='confirmed'", &confirmed}, {"SELECT count(*) FROM hack_team_members WHERE event_id=$1 AND $2::uuid IS NOT NULL", &members}, {"SELECT count(*) FROM session_commands WHERE event_id=$1 AND session_id=$2", &commands}} {
				if err := f.pool.QueryRow(request.Context(), query.sql, f.eventID, f.sessionID).Scan(query.target); err != nil {
					http.Error(w, "fixture evidence unavailable", 500)
					return
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"booked": booked, "confirmed": confirmed, "team_members": members, "commands": commands})
			return
		default:
			http.Error(w, "unknown private control", 400)
			return
		}
		if actionErr != nil {
			http.Error(w, fmt.Sprintf("native fixture action rejected: %v", actionErr), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})}
	go func() { _ = control.Serve(listener) }()
	t.Cleanup(func() { _ = control.Close() })
	manifest := map[string]any{"event_id": f.eventID, "session_id": f.sessionID, "new_session_id": newID, "admin": admin.server.URL, "public": public.server.URL, "socket": socket, "run_id": uuid.NewString(), "viewport": viewport, "opens": definition.BookingOpens.Wall, "closes": definition.BookingCloses.Wall, "facilitator": facilitator.DisplayName}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(private, "manifest.json")
	if err = os.WriteFile(manifestPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, "node", "node_modules/@playwright/test/cli.js", "test", "--config", "playwright.sessions-native.config.ts")
	command.Dir = filepath.Join("..", "..", "web")
	command.Env = append(os.Environ(), "HACKBOX_SESSION_MANIFEST="+manifestPath, "TMPDIR="+private)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err = command.Run(); err != nil {
		t.Fatal(err)
	}
}
