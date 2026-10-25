package app

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/presentation"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/google/uuid"
)

// Browser controls exist only on an owner-private Unix socket. Production HTTP
// responses, sessions, membership, report versions and SSE remain the real app.
func TestTeamReportsUIBrowsers(t *testing.T) {
	if os.Getenv("HACKBOX_REPORT_BROWSER") != "1" {
		t.Skip("set HACKBOX_INTEGRATION=1 HACKBOX_REPORT_BROWSER=1 after building both portals")
	}
	for _, viewport := range []string{"desktop", "mobile"} {
		t.Run(viewport, func(t *testing.T) { runTeamReportsBrowser(t, viewport) })
	}
}
func runTeamReportsBrowser(t *testing.T, viewport string) {
	ctx := t.Context()
	f := newResultsSourceFixtureWithSetup(t, rubric.AllTeams, func(d *rubric.Definition) {
		d.Prizes = []rubric.Prize{{ID: uuid.New(), Name: "Native outcome award", Description: "Explicit overall winner award"}}
	}, nil)
	f.complete(t)
	r := f.runtime
	display, err := f.admin.deps.Presentation.Read(ctx, r.owner.ID, r.eventID)
	resultsSourceRequire(t, err)
	paired, err := f.admin.deps.Presentation.Pair(ctx, r.owner.ID, r.eventID, display.Revision, presentation.PairInput{Label: "Native results browser display", Minutes: 60, Reason: "Actual scoped native room proof"})
	resultsSourceRequire(t, err)
	private, err := os.MkdirTemp("/tmp", "hackbox113-")
	resultsSourceRequire(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(private) })
	private, err = filepath.EvalSymlinks(private)
	resultsSourceRequire(t, err)
	socket := filepath.Join(private, "control.sock")
	listener, err := net.Listen("unix", socket)
	resultsSourceRequire(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	control := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "POST required", 405)
			return
		}
		var input struct{ Action, Actor, Surface string }
		if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1024)).Decode(&input); err != nil {
			http.Error(w, "invalid private control", 400)
			return
		}
		switch input.Action {
		case "session":
			surface := f.public
			if input.Surface == "admin" {
				surface = f.admin
			} else if input.Surface != "public" {
				http.Error(w, "unknown surface", 400)
				return
			}
			person := r.competitor
			if input.Actor == "owner" {
				person = r.owner
			} else if input.Actor == "judge" {
				person = f.judges[0]
			} else if input.Actor != "competitor" {
				http.Error(w, "unknown actor", 400)
				return
			}
			person, err := dbgen.New(r.pool).Person(req.Context(), person.ID)
			if err != nil {
				http.Error(w, "actor unavailable", 500)
				return
			}
			cookie, err := surface.sessionCookie(req.Context(), person)
			if err != nil {
				http.Error(w, "session unavailable", 500)
				return
			}
			_ = json.NewEncoder(w).Encode(cookie)
		case "room":
			_ = json.NewEncoder(w).Encode(map[string]string{"token": paired.Token})
		case "complete":
			completeReportEvent(t, f)
			w.WriteHeader(204)
		case "evidence":
			var versions, releases int
			if err := r.pool.QueryRow(req.Context(), "SELECT count(*),count(*) FILTER(WHERE document->>'state'='released') FROM team_feedback_versions WHERE event_id=$1 AND team_id=$2", r.eventID, r.teamID).Scan(&versions, &releases); err != nil {
				http.Error(w, "report evidence unavailable", 500)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]int{"versions": versions, "releases": releases})
		default:
			http.Error(w, "unknown private action", 400)
		}
	})}
	go func() { _ = control.Serve(listener) }()
	t.Cleanup(func() { _ = control.Close() })
	manifest, err := json.Marshal(map[string]any{"event_id": r.eventID, "team_id": r.teamID, "admin": f.admin.server.URL, "public": f.public.server.URL, "socket": socket, "viewport": viewport})
	resultsSourceRequire(t, err)
	path := filepath.Join(private, "manifest.json")
	resultsSourceRequire(t, os.WriteFile(path, manifest, 0600))
	command := exec.CommandContext(ctx, "node", "node_modules/@playwright/test/cli.js", "test", "--config", "playwright.reports-native.config.ts")
	command.Dir = filepath.Join("..", "..", "web")
	command.Env = append(os.Environ(), "HACKBOX_REPORT_MANIFEST="+path, "HACKBOX_REPORT_VIEWPORT="+viewport, "TMPDIR="+private)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err = command.Run(); err != nil {
		t.Fatal("native results/report browser journey", err)
	}
}
