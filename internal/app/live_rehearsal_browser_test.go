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
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

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
	proxy := newRehearsalProxy(t, public.server.URL)
	// A Unix socket is private test IPC, never a route registered on either
	// application listener. The browser cannot call it through the product API.
	// Keep below Unix sockaddr path limits on macOS as well as Linux.
	private, err := os.MkdirTemp("/tmp", "hackbox90-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(private) })
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
				Instant time.Time `json:"instant"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, request.Body, 1024)).
				Decode(&input); err != nil {
				http.Error(w, "invalid control", 400)
				return
			}
			switch input.Action {
			case "reset-room":
				service := admin.deps.Presentation
				index := 0
				for _, command := range []presentation.Command{
					{Action: "save", Cues: []presentation.Cue{{ID: uuid.New(), Title: "Rehearsal opening", Text: "Approved public opening", DurationSeconds: 120}, {ID: uuid.New(), Title: "Rehearsal building", Text: "Approved public building", DurationSeconds: 60}}},
					{Action: "publish"},
					{Action: "activate", Index: &index},
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
				var revision, jobs int64
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
				_ = json.NewEncoder(w).
					Encode(map[string]any{"event_revision": revision, "provider_jobs": jobs})
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
		"event_id": r.eventID,
		"admin":    admin.server.URL,
		"public":   public.server.URL,
		"display":  proxy.server.URL,
		"socket":   socket,
		"mc_cookie": admin.cookie(
			t,
			r.mc,
		),
		"mentor_cookie":     public.cookie(t, r.mentor),
		"competitor_cookie": public.cookie(t, r.competitor),
		"run_id":            uuid.NewString(),
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
	command.Dir = filepath.Join("..", "..", "web")
	command.Env = append(os.Environ(), "HACKBOX_REHEARSAL_MANIFEST="+manifestPath)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		t.Fatal(fmt.Errorf("production browser rehearsal: %w", err))
	}
}
