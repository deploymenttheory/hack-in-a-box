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

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/deploymenttheory/hack-in-a-box/internal/results"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestMentorAdvisoryBrowsers(t *testing.T) {
	if os.Getenv("HACKBOX_ADVISORY_BROWSER") != "1" {
		t.Skip("set HACKBOX_INTEGRATION=1 HACKBOX_ADVISORY_BROWSER=1 after building both portals")
	}
	for _, portal := range []string{"admin", "public"} {
		for _, viewport := range []string{"desktop", "mobile"} {
			scope := portal + "-" + viewport
			if selected := os.Getenv(
				"HACKBOX_ADVISORY_SCOPE",
			); selected != "" &&
				selected != scope {
				continue
			}
			t.Run(
				scope,
				func(t *testing.T) { runMentorAdvisoryBrowser(t, portal, viewport, scope) },
			)
		}
	}
}

func runMentorAdvisoryBrowser(t *testing.T, portal, viewport, scope string) {
	f, _ := newTiedResultsFixture(t, func(d *rubric.Definition) {
		category := d.Categories[0].ID
		d.Prizes = []rubric.Prize{
			{ID: uuid.New(), Name: "Native category advisory award", CategoryID: &category},
		}
	})
	r, ctx := f.runtime, t.Context()
	surface := f.public
	if portal == "admin" {
		surface = f.admin
	}
	p, err := f.service.Read(ctx, r.owner.ID, r.eventID)
	resultsSourceRequire(t, err)
	_, err = f.service.Change(
		ctx,
		r.owner.ID,
		r.eventID,
		p.Revision,
		results.Command{
			RequestID:     uuid.New(),
			Version:       p.Round.Version,
			Action:        "prepare",
			SourceBinding: p.Preview.Binding,
			Reason:        "Actual native browser category deliberation",
		},
	)
	resultsSourceRequire(t, err)
	own, err := f.service.MentorAdvisories(ctx, r.mentor.ID, r.eventID)
	resultsSourceRequire(t, err)
	if len(own.Targets) == 0 {
		t.Fatal("actual advisory targets absent")
	}
	target := own.Targets[0]
	// Keep independent event access when mentor capability is withdrawn, so
	// the browser proves the narrower resource owner clears its own private view.
	resultsSourceRequire(
		t,
		dbgen.New(r.pool).
			SetMembership(ctx, dbgen.SetMembershipParams{EventID: r.eventID, PersonID: r.mentor.ID, Role: "facilitator", Active: true}),
	)
	outsider := testdb.Person(t, r.pool)
	resultsSourceRequire(
		t,
		dbgen.New(r.pool).
			SetMembership(ctx, dbgen.SetMembershipParams{EventID: r.eventID, PersonID: outsider.ID, Role: "mentor", Active: true}),
	)
	private, err := os.MkdirTemp("/tmp", "hackbox112-")
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
				var person dbgen.Person
				switch input.Actor {
				case "mentor":
					person = r.mentor
				case "judge":
					person = f.judges[0]
				case "other-judge":
					person = f.judges[1]
				case "outsider":
					person = outsider
				case "competitor":
					person = r.competitor
				default:
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
			case "revoke-mentor", "restore-mentor":
				actionErr = dbgen.New(r.pool).
					SetMembership(req.Context(), dbgen.SetMembershipParams{EventID: r.eventID, PersonID: r.mentor.ID, Role: "mentor", Active: input.Action == "restore-mentor"})
			case "reopen-source":
				states, e := surface.deps.Judging.CardStates(
					req.Context(),
					r.owner.ID,
					r.eventID,
					f.judgeRun,
					r.teamID,
				)
				actionErr = e
				if actionErr == nil {
					found := false
					for _, state := range states.Items {
						if state.AuthorID == f.judges[0].ID {
							found = true
							_, actionErr = surface.deps.Judging.ChangeCard(
								req.Context(),
								r.owner.ID,
								r.eventID,
								f.judgeRun,
								r.teamID,
								state.Version,
								judging.CardCommand{
									RequestID:            uuid.New(),
									Action:               "reopen",
									AuthorID:             state.AuthorID,
									SubmissionRevision:   state.SubmissionRevision,
									RepositoryDecisionID: state.RepositoryDecisionID,
									Reason:               "Native browser source revision invalidates held advisory",
								},
							)
							break
						}
					}
					if !found {
						actionErr = fmt.Errorf("native submitted judge card absent")
					}
				}
			default:
				http.Error(w, "unknown control", 400)
				return
			}
			if actionErr != nil {
				http.Error(w, "native owner rejected control: "+actionErr.Error(), 409)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}),
	}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	manifest, err := json.Marshal(
		map[string]any{
			"origin":        surface.server.URL,
			"socket":        socket,
			"scope":         scope,
			"viewport":      viewport,
			"event_id":      r.eventID,
			"team_id":       r.teamID,
			"tie_id":        target.TieID,
			"category_name": target.CategoryName,
			"team_name":     target.TeamName,
			"run_id":        own.RunID,
		},
	)
	resultsSourceRequire(t, err)
	path := filepath.Join(private, "manifest.json")
	resultsSourceRequire(t, os.WriteFile(path, manifest, 0o600))
	command := exec.CommandContext(
		ctx,
		"node",
		"node_modules/@playwright/test/cli.js",
		"test",
		"--config",
		"playwright.advisory-native.config.ts",
	)
	command.Dir = filepath.Join("..", "..", "web")
	command.Env = append(
		os.Environ(),
		"HACKBOX_ADVISORY_MANIFEST="+path,
		"HACKBOX_ADVISORY_SCOPE="+scope,
		"TMPDIR="+private,
	)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err = command.Run(); err != nil {
		t.Fatal("actual native mentor advisory browser journey", err)
	}
	var retained int
	resultsSourceRequire(
		t,
		r.pool.QueryRow(ctx, "SELECT count(*) FROM result_mentor_advisory_versions WHERE event_id=$1", r.eventID).
			Scan(&retained),
	)
	if retained != 3 {
		t.Fatal("browser replayed or wrote after current authority/source ended", retained)
	}
	var versions, votes int
	resultsSourceRequire(
		t,
		r.pool.QueryRow(ctx, "SELECT count(*) FROM result_versions WHERE event_id=$1", r.eventID).
			Scan(&versions),
	)
	resultsSourceRequire(
		t,
		r.pool.QueryRow(ctx, "SELECT jsonb_array_length(document->'votes') FROM result_versions WHERE event_id=$1 ORDER BY version DESC LIMIT 1", r.eventID).
			Scan(&votes),
	)
	if versions != 2 || votes != 1 {
		t.Fatal(
			"advisory changed native result decisions or counted as another vote",
			versions,
			votes,
		)
	}
}
