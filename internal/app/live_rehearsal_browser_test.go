package app

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/observations"
	"github.com/deploymenttheory/hack-in-a-box/internal/presentation"
)

// rehearsalProxy changes availability, never domain responses. All available
// requests and SSE frames pass through the actual production router unchanged.
type rehearsalProxy struct {
	server *httptest.Server
	outage atomic.Bool
}

func newRehearsalProxy(t *testing.T, upstream string) *rehearsalProxy {
	t.Helper()
	target, err := url.Parse(upstream)
	if err != nil {
		t.Fatal(err)
	}
	p := &rehearsalProxy{}
	forward := httputil.NewSingleHostReverseProxy(target)
	forward.FlushInterval = -1
	p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p.outage.Load() && len(r.URL.Path) >= 5 && r.URL.Path[:5] == "/api/" {
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "Synthetic backend outage", http.StatusServiceUnavailable)
			return
		}
		forward.ServeHTTP(w, r)
	}))
	t.Cleanup(p.server.Close)
	return p
}

func TestLiveRehearsalBrowsers(t *testing.T) {
	if os.Getenv("HACKBOX_LIVE_REHEARSAL") != "1" {
		t.Skip(
			"set HACKBOX_INTEGRATION=1 HACKBOX_LIVE_REHEARSAL=1 after building both production portals",
		)
	}
	r := newLiveRehearsal(t)
	admin, public := r.surface(t, "admin"), r.surface(t, "public")
	for _, surface := range []rehearsalSurface{admin, public} {
		// Native router owns SPA fallback; missing built assets must fail visibly.
		response, err := http.Get(
			surface.server.URL,
		) // #nosec G704 -- isolated httptest loopback URL.
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatal("build both production portals before the integrated rehearsal")
		}
	}
	// Seed actual native private evidence before any display is connected.
	previousClock := r.clock.read()
	r.clock.set(time.Date(2027, 3, 29, 9, 0, 0, 0, time.UTC))
	current, err := admin.deps.Observations.Read(
		t.Context(),
		r.mentor.ID,
		r.eventID,
		r.teamID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.deps.Observations.Change(
		t.Context(),
		r.mentor.ID,
		r.eventID,
		r.teamID,
		current.Revision,
		observations.Command{
			Action: "observation",
			ID:     uuid.New(),
			Evidence: observations.Evidence{
				Category:          "PRIVATE-MENTOR-CATEGORY",
				Evidence:          "PRIVATE-MENTOR-EVIDENCE",
				Outcome:           "PRIVATE-MENTOR-OUTCOME",
				FeedbackReference: "PRIVATE-FEEDBACK-REFERENCE",
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	r.clock.set(previousClock)
	support := seedRehearsalSupport(t, r, admin.deps)
	proxy := newRehearsalProxy(t, public.server.URL)
	// A Unix socket is private test IPC, never a route registered on either
	// application listener. The browser cannot call it through the product API.
	// Keep below Unix sockaddr path limits on macOS as well as Linux.
	private, err := os.MkdirTemp("/tmp", "hackbox90-")
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
	control := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			if request.Method != http.MethodPost {
				http.Error(w, "POST required", 405)
				return
			}
			var input struct {
				Action  string    `json:"action"`
				Actor   string    `json:"actor"`
				Surface string    `json:"surface"`
				Instant time.Time `json:"instant"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, request.Body, 1024)).
				Decode(&input); err != nil {
				http.Error(w, "invalid control", 400)
				return
			}
			var err error
			switch input.Action {
			case "session":
				person := r.mc
				switch input.Actor {
				case "mc":
					person = r.mc
				case "mentor":
					person = r.mentor
				case "competitor":
					person = r.competitor
				case "owner":
					person = r.owner
				default:
					http.Error(w, "unknown synthetic actor", 400)
					return
				}
				surface := public
				if input.Surface == "admin" {
					surface = admin
				} else if input.Surface != "public" {
					http.Error(w, "unknown surface", 400)
					return
				}
				cookie, err := surface.sessionCookie(request.Context(), person)
				if err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				_ = json.NewEncoder(w).Encode(cookie)
				return
			case "correct-clarification":
				if err := support.correct(request.Context(), r, admin.deps); err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
			case "stage-lag", "stage-live":
				stage := "preparation"
				if input.Action == "stage-live" {
					stage = "live"
				}
				if _, err := r.pool.Exec(
					request.Context(),
					"UPDATE events SET stage=$2 WHERE id=$1",
					r.eventID,
					stage,
				); err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
			case "mentor-role-remove", "mentor-role-restore":
				if input.Action == "mentor-role-remove" {
					_, err = r.pool.Exec(
						request.Context(),
						"DELETE FROM event_memberships WHERE event_id=$1 AND person_id=$2 AND role='mentor'",
						r.eventID,
						r.mentor.ID,
					)
				} else {
					_, err = r.pool.Exec(
						request.Context(),
						"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mentor') ON CONFLICT DO NOTHING",
						r.eventID,
						r.mentor.ID,
					)
				}
				if err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
			case "reset-room":
				service := admin.deps.Presentation
				index := 0
				for _, command := range []presentation.Command{
					{Action: "save", Cues: []presentation.Cue{{ID: uuid.New(), Title: "Rehearsal opening", Text: "Approved public opening", DurationSeconds: 120}, {ID: uuid.New(), Title: "Rehearsal building", Text: "Approved public building", DurationSeconds: 60}}},
					{Action: "publish"},
					{Action: "activate", Index: &index},
					{Action: "save", Cues: []presentation.Cue{{ID: uuid.New(), Title: "PRIVATE-UNRELEASED-CUE", Text: "PRIVATE-REHEARSAL-ONLY", DurationSeconds: 120}}},
				} {
					page, err := service.Read(request.Context(), r.owner.ID, r.eventID)
					if err != nil {
						http.Error(w, err.Error(), 500)
						return
					}
					command.Reason = "Synthetic native rehearsal fixture preparation"
					if _, err := service.Change(
						request.Context(),
						r.owner.ID,
						r.eventID,
						page.Revision,
						command,
					); err != nil {
						http.Error(w, err.Error(), 500)
						return
					}
				}
			case "clock":
				if input.Instant.IsZero() {
					http.Error(w, "instant required", 400)
					return
				}
				r.clock.set(input.Instant)
			case "outage":
				proxy.outage.Store(true)
				proxy.server.CloseClientConnections()
			case "restore":
				proxy.outage.Store(false)
			case "evidence":
				var revision, presentationRevision, jobs int64
				if err := r.pool.QueryRow(request.Context(), "SELECT revision FROM events WHERE id=$1", r.eventID).
					Scan(&revision); err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				if err := r.pool.QueryRow(request.Context(), "SELECT count(*) FROM river_job").
					Scan(&jobs); err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				if err := r.pool.QueryRow(request.Context(), "SELECT revision FROM presentation_states WHERE event_id=$1", r.eventID).
					Scan(&presentationRevision); err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				_ = json.NewEncoder(w).
					Encode(map[string]any{"event_revision": revision, "presentation_revision": presentationRevision, "provider_jobs": jobs})
				return
			default:
				http.Error(w, "unknown control", 400)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}),
	}
	go func() { _ = control.Serve(listener) }()
	t.Cleanup(func() { _ = control.Close() })
	// Only synthetic short-lived test sessions are passed to the node owner.
	manifest := map[string]any{
		"answer_id":           support.answerID,
		"help_id":             support.helpID,
		"private_question_id": support.questionID,
		"event_id":            r.eventID,
		"team_id":             r.teamID,
		"other_event_id":      r.otherEventID,
		"admin":               admin.server.URL,
		"public":              public.server.URL,
		"display":             proxy.server.URL,
		"socket":              socket,
		"run_id":              uuid.NewString(),
	}
	manifestPath := filepath.Join(private, "manifest.json")
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(
		t.Context(),
		"node",
		"node_modules/@playwright/test/cli.js",
		"test",
		"--config",
		"playwright.live.config.ts",
	)
	if project := os.Getenv("HACKBOX_REHEARSAL_PROJECT"); project != "" {
		if project != "chromium" && project != "firefox-display" {
			t.Fatal("unknown rehearsal project")
		}
		command.Args = append(command.Args, "--project", project)
	}
	if pattern := os.Getenv("HACKBOX_REHEARSAL_GREP"); pattern != "" {
		command.Args = append(command.Args, "--grep", pattern)
	}
	command.Dir = filepath.Join("..", "..", "web")
	command.Env = append(
		os.Environ(),
		"HACKBOX_REHEARSAL_MANIFEST="+manifestPath,
		"TMPDIR="+private,
	)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	result := command.Run()
	source, err := exec.CommandContext(t.Context(), "git", "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	proof := map[string]any{
		"source_commit":             strings.TrimSpace(string(source)),
		"fixture_run_id":            manifest["run_id"],
		"event_id":                  r.eventID,
		"team_id":                   r.teamID,
		"publication_at":            previousClock,
		"project_selection":         os.Getenv("HACKBOX_REHEARSAL_PROJECT"),
		"test_selection":            os.Getenv("HACKBOX_REHEARSAL_GREP"),
		"browser_command_succeeded": result == nil,
	}
	proofJSON, err := json.MarshalIndent(proof, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(command.Dir, "test-results-live", "native-fixture.json"),
		proofJSON,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Fatal(fmt.Errorf("production browser rehearsal: %w", result))
	}
}
