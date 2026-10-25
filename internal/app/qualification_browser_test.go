package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func TestQualificationBrowsers(t *testing.T) {
	if os.Getenv("HACKBOX_QUALIFICATION_BROWSER") != "1" {
		t.Skip(
			"set HACKBOX_INTEGRATION=1 HACKBOX_QUALIFICATION_BROWSER=1 after building both portals",
		)
	}
	for _, surface := range []string{"admin", "public"} {
		if chosen := os.Getenv("HACKBOX_QUALIFICATION_SURFACE"); chosen != "" && chosen != surface {
			continue
		}
		for _, viewport := range []string{"desktop", "mobile"} {
			for _, scenario := range []string{"initial", "final"} {
				if selected := os.Getenv(
					"HACKBOX_QUALIFICATION_SCENARIO",
				); selected != "" &&
					selected != scenario {
					continue
				}
				t.Run(
					surface+"/"+viewport+"/"+scenario,
					func(t *testing.T) { runQualificationBrowser(t, surface, viewport, scenario) },
				)
			}
		}
	}
}

func runQualificationBrowser(t *testing.T, surfaceName, viewport, scenario string) {
	t.Helper()
	var r *liveRehearsal
	var admin, public rehearsalSurface
	var run uuid.UUID
	var final qualificationFinalFixture
	if scenario == "final" {
		final = newQualificationFinalCohort(t, 5)
		r, admin, public, run = final.runtime, final.admin, final.public, final.run
	} else {
		r, admin, public, run = qualificationHTTPFixture(t)
	}
	surface := public
	if surfaceName == "admin" {
		surface = admin
	}
	page, err := admin.deps.Qualification.Read(t.Context(), r.owner.ID, r.eventID, run)
	if err != nil || page.Round.Version != 0 {
		t.Fatal("real initial native panel", page, err)
	}
	private, err := os.MkdirTemp("/tmp", "hackbox111-")
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
			var input struct{ Action string }
			if err := json.NewDecoder(http.MaxBytesReader(w, request.Body, 1024)).
				Decode(&input); err != nil {
				http.Error(w, "invalid private control", 400)
				return
			}
			q := dbgen.New(r.pool)
			switch input.Action {
			case "session", "session-primary", "session-second":
				actor := r.owner.ID
				if input.Action == "session-primary" {
					actor = r.mentor.ID
				}
				if input.Action == "session-second" {
					if scenario != "final" {
						http.Error(w, "actor unavailable", 400)
						return
					}
					actor = final.second.ID
				}
				person, err := q.Person(request.Context(), actor)
				if err != nil {
					http.Error(w, "current synthetic identity unavailable", 500)
					return
				}
				cookie, err := surface.sessionCookie(request.Context(), person)
				if err != nil {
					http.Error(w, "synthetic session unavailable", 500)
					return
				}
				_ = json.NewEncoder(w).Encode(cookie)
			case "expire":
				if err := q.RevokeSessions(request.Context(), r.owner.ID); err != nil {
					http.Error(w, "synthetic session expiry failed", 500)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			case "retire":
				if scenario != "final" {
					http.Error(w, "final fixture required", 400)
					return
				}
				current, err := admin.deps.Submissions.Read(
					request.Context(),
					r.owner.ID,
					r.eventID,
					final.teams[0].id,
				)
				if err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				if _, err = admin.deps.Submissions.Outcome(
					request.Context(),
					r.owner.ID,
					r.eventID,
					final.teams[0].id,
					current.Revision,
					"withdrawn",
					"Actual test-owned organiser withdrawal after private panel preparation",
				); err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				// Existing native outcome fixtures own these team lifecycle facts. This
				// bridge tests the UI consuming disbandment, not a team lifecycle workflow.
				tx, err := r.pool.Begin(request.Context())
				if err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				defer func() { _ = tx.Rollback(request.Context()) }()
				if _, err = tx.Exec(
					request.Context(),
					"UPDATE competition_entries SET active=false WHERE event_id=$1 AND team_id=$2",
					r.eventID,
					final.teams[1].id,
				); err == nil {
					_, err = tx.Exec(
						request.Context(),
						"UPDATE hack_teams SET state='disbanded' WHERE event_id=$1 AND id=$2",
						r.eventID,
						final.teams[1].id,
					)
				}
				if err == nil {
					_, err = tx.Exec(
						request.Context(),
						"INSERT INTO team_repositories(event_id,team_id,organization_id,organization,name,app_id,installation_id,template_repository,requested_by,state)VALUES($1,$2,99,'synthetic-org','UNUSED-DISBANDED-REPOSITORY-CANARY',42,55,'template',$3,'pending')",
						r.eventID,
						final.teams[1].id,
						r.owner.ID,
					)
				}
				if err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				if err = tx.Commit(request.Context()); err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			case "evidence":
				current, err := admin.deps.Qualification.Read(
					request.Context(),
					r.owner.ID,
					r.eventID,
					run,
				)
				if err != nil {
					http.Error(w, "native evidence read failed", 500)
					return
				}
				_ = json.NewEncoder(w).
					Encode(map[string]any{"version": current.Round.Version, "cards": len(current.Round.Cards), "current": current.Current, "teams": current.Teams, "signoff": current.Round.Signoff != nil, "page": current})
			default:
				http.Error(w, "unknown private control", 400)
			}
		}),
	}
	go func() { _ = control.Serve(listener) }()
	t.Cleanup(func() { _ = control.Close() })
	manifest := map[string]any{
		"event_id":              r.eventID,
		"run_id":                run,
		"team_id":               r.teamID,
		"run_name":              page.Criteria.Name,
		"owner_id":              r.owner.ID,
		"mentor_name":           r.mentor.DisplayName,
		"origin":                surface.server.URL,
		"socket":                socket,
		"surface":               surfaceName,
		"viewport":              viewport,
		"command_metadata_path": filepath.Join(private, "last-command.json"),
	}
	manifest["scenario"] = scenario
	if scenario == "final" {
		ids := []uuid.UUID{}
		for _, team := range final.teams {
			ids = append(ids, team.id)
		}
		manifest["team_ids"] = ids
		manifest["second_id"] = final.second.ID
		manifest["primary_id"] = r.mentor.ID
		manifest["categories"] = final.checkpoint.Criteria.Categories
	}
	labels, err := dbgen.New(r.pool).HackTeams(t.Context(), r.eventID)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, team := range labels {
		names[team.ID.String()] = team.Name
	}
	manifest["team_names"] = names
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(private, "manifest.json")
	if err = os.WriteFile(manifestPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(
		t.Context(),
		"node",
		"node_modules/@playwright/test/cli.js",
		"test",
		"--config",
		"playwright.qualification-native.config.ts",
	)
	command.Dir = filepath.Join("..", "..", "web")
	command.Env = append(
		os.Environ(),
		"HACKBOX_QUALIFICATION_MANIFEST="+manifestPath,
		"HACKBOX_QUALIFICATION_SCOPE="+surfaceName+"-"+viewport+"-"+scenario,
		"TMPDIR="+private,
	)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	git := func(args ...string) string {
		t.Helper()
		value, err := exec.CommandContext(t.Context(), "git", args...).Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(value))
	}
	head, tree, status := git(
		"rev-parse",
		"HEAD",
	), git(
		"rev-parse",
		"HEAD^{tree}",
	), git(
		"status",
		"--porcelain",
		"--untracked-files=normal",
	)
	runErr := command.Run()
	end, statusEnd := git(
		"rev-parse",
		"HEAD",
	), git(
		"status",
		"--porcelain",
		"--untracked-files=normal",
	)
	proof := map[string]any{
		"source_commit":             head,
		"source_tree":               tree,
		"source_commit_end":         end,
		"source_worktree_clean":     status == "" && statusEnd == "",
		"surface":                   surfaceName,
		"viewport":                  viewport,
		"event_id":                  r.eventID,
		"run_id":                    run,
		"browser_command_succeeded": runErr == nil,
		"scope":                     scenario + " native panel journey; exact browser assertions define exercised acceptance",
	}
	if runErr != nil {
		// Inspect only test-owned safe original identity after a genuine failure.
		// This diagnostic cannot retry, acknowledge in the UI or change an outcome.
		var metadata struct {
			RequestID uuid.UUID `json:"request_id"`
			Action    string    `json:"action"`
		}
		if raw, readErr := os.ReadFile(
			filepath.Join(private, "last-command.json"),
		); readErr == nil && json.Unmarshal(raw, &metadata) == nil &&
			metadata.RequestID != uuid.Nil {
			diagnostic, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			recovered, receiptErr := admin.deps.Qualification.OriginalReceipt(
				diagnostic,
				r.owner.ID,
				r.eventID,
				run,
				metadata.RequestID,
			)
			proof["last_owner_request_action"], proof["last_owner_request_id"] = metadata.Action, metadata.RequestID
			proof["diagnostic_original_ack_available"] = receiptErr == nil &&
				recovered.Receipt != nil
			if receiptErr != nil {
				proof["diagnostic_original_ack_error"] = receiptErr.Error()
			}
			proof["diagnostic_pool_acquired"], proof["diagnostic_pool_idle"] = r.pool.Stat().
				AcquiredConns(),
				r.pool.Stat().
					IdleConns()
			cancel()
		}
	}
	proofJSON, err := json.MarshalIndent(proof, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	proofDir := filepath.Join(
		command.Dir,
		"test-results-qualification-native",
		surfaceName+"-"+viewport+"-"+scenario,
	)
	if err = os.MkdirAll(proofDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(
		filepath.Join(proofDir, "native-fixture.json"),
		proofJSON,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if head != end {
		t.Fatal("source changed during native qualification proof")
	}
	if runErr != nil {
		t.Fatal(fmt.Errorf("native qualification browser: %w", runErr))
	}
}
