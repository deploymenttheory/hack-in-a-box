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
)

func TestShowcaseBrowsers(t *testing.T) {
	if os.Getenv("HACKBOX_SHOWCASE_BROWSER") != "1" {
		t.Skip("set HACKBOX_INTEGRATION=1 HACKBOX_SHOWCASE_BROWSER=1 after building both portals")
	}
	for _, surface := range []string{"admin", "public"} {
		for _, viewport := range []string{"desktop", "mobile"} {
			scope := surface + "-" + viewport
			if selected := os.Getenv(
				"HACKBOX_SHOWCASE_SCOPE",
			); selected != "" &&
				selected != scope {
				continue
			}
			t.Run(scope, func(t *testing.T) { runShowcaseBrowser(t, surface, scope) })
		}
	}
}

func runShowcaseBrowser(t *testing.T, surfaceName, scope string) {
	s := newShowcaseFixture(t, true)
	f, r := s.results, s.results.runtime
	surface := f.public
	if surfaceName == "admin" {
		surface = f.admin
	}
	private, err := os.MkdirTemp("/tmp", "hackbox163-")
	resultsSourceRequire(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(private) })
	private, err = filepath.EvalSymlinks(private)
	resultsSourceRequire(t, err)
	socket := filepath.Join(private, "control.sock")
	listener, err := net.Listen("unix", socket)
	resultsSourceRequire(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	server := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.Method != http.MethodPost {
				http.Error(w, "POST required", 405)
				return
			}
			var input struct{ Action, Actor string }
			decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1024))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil {
				http.Error(w, "invalid owner-private control", 400)
				return
			}
			var actionErr error
			switch input.Action {
			case "session":
				person := r.competitor
				if input.Actor == "outsider" {
					person = s.outsider
				} else if input.Actor == "owner" {
					person = r.owner
				} else if input.Actor != "competitor" {
					http.Error(w, "unknown synthetic actor", 400)
					return
				}
				cookie, e := surface.sessionCookie(req.Context(), person)
				if e != nil {
					http.Error(w, "native session unavailable", 500)
					return
				}
				_ = json.NewEncoder(w).Encode(cookie)
				return
			case "announce":
				actionErr = s.announce(req.Context())
			case "correct":
				actionErr = s.correct(req.Context())
			case "withdraw-nonwinner":
				actionErr = s.withdraw(req.Context(), s.nonWinnerProject)
			case "withdraw-cache-project":
				actionErr = s.withdraw(req.Context(), s.secondProject)
			case "archive":
				actionErr = s.archive(req.Context())
			case "decline-consent":
				actionErr = s.declineConsent(req.Context())
			case "withdraw-audience":
				// Synthetic authority loss is an input fact, not proof of the registration
				// withdrawal workflow. Every subsequent result/project/media read is native.
				_, actionErr = r.pool.Exec(
					req.Context(),
					"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
					r.eventID,
					r.competitor.ID,
				)
			case "evidence":
				var versions int
				actionErr = r.pool.QueryRow(req.Context(), "SELECT count(*) FROM outcome_project_versions WHERE outcome_id=$1", s.nonWinnerProject).
					Scan(&versions)
				if actionErr == nil {
					_ = json.NewEncoder(w).Encode(map[string]int{"nonwinner_versions": versions})
					return
				}
			default:
				http.Error(w, "unknown owner-private action", 400)
				return
			}
			if actionErr != nil {
				http.Error(w, fmt.Sprintf("native %s: %v", input.Action, actionErr), 500)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}),
	}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	manifest, err := json.Marshal(
		map[string]any{
			"origin":             surface.server.URL,
			"socket":             socket,
			"scope":              scope,
			"event_id":           r.eventID,
			"second_event_id":    s.secondEvent,
			"winner_project":     s.winnerProject,
			"nonwinner_project":  s.nonWinnerProject,
			"second_project":     s.secondProject,
			"draft_project":      s.draftProject,
			"restricted_project": s.restrictedProject,
			"media_id":           s.mediaID,
		},
	)
	resultsSourceRequire(t, err)
	path := filepath.Join(private, "manifest.json")
	resultsSourceRequire(t, os.WriteFile(path, manifest, 0o600))
	command := exec.CommandContext(
		t.Context(),
		"node",
		"node_modules/@playwright/test/cli.js",
		"test",
		"--config",
		"playwright.showcase-native.config.ts",
	)
	command.Dir = filepath.Join("..", "..", "web")
	command.Env = append(
		os.Environ(),
		"HACKBOX_SHOWCASE_MANIFEST="+path,
		"HACKBOX_SHOWCASE_SCOPE="+scope,
		"TMPDIR="+private,
	)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err = command.Run(); err != nil {
		t.Fatal("actual native showcase browser journey", err)
	}
}
