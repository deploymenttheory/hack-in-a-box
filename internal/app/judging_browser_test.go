package app

import (
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
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/deploymenttheory/hack-in-a-box/internal/observations"
	"github.com/deploymenttheory/hack-in-a-box/internal/presentation"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type judgeBrowserEdition struct {
	runtime       *liveRehearsal
	first, second dbgen.Person
	surface       rehearsalSurface
	runID         uuid.UUID
}

// Each practice/final/portal/viewport edition owns its actual PostgreSQL runtime
// and monotonic domain clock. No completed outcome is erased or clock rewound.
func newJudgeBrowserEdition(t *testing.T, surface string, final bool) judgeBrowserEdition {
	t.Helper()
	ctx := t.Context()
	r := newLiveRehearsal(t)
	f := judgeBrowserEdition{
		runtime: r,
		first:   testdb.Person(t, r.pool),
		second:  testdb.Person(t, r.pool),
	}
	q := dbgen.New(r.pool)
	for _, person := range []dbgen.Person{f.first, f.second} {
		if err := q.SetMembership(
			ctx,
			dbgen.SetMembershipParams{
				EventID:  r.eventID,
				PersonID: person.ID,
				Role:     "judge",
				Active:   true,
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	f.surface = r.surface(t, surface)
	rs := f.surface.deps.Rubrics
	current, err := rs.Configuration(ctx, r.owner.ID, r.eventID, 0)
	if err != nil {
		t.Fatal(err)
	}
	current, err = rs.Change(
		ctx,
		r.owner.ID,
		r.eventID,
		current.Revision,
		rubric.Command{
			Action:  "create_run",
			Purpose: "judge_final",
			Version: current.Reference.Version,
			Reason:  "Synthetic native judge browser source",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range current.Runs {
		if run.Purpose == "judge_final" {
			f.runID = run.ID
		}
	}
	if _, err = f.surface.deps.Judging.ChangeRound(
		ctx,
		r.owner.ID,
		r.eventID,
		f.runID,
		0,
		judging.RoundCommand{
			RequestID: uuid.New(),
			Action:    "assign",
			TeamID:    r.teamID,
			JudgeIDs:  []uuid.UUID{f.first.ID, f.second.ID},
			Reason:    "Two independent synthetic assigned judges",
		},
	); err != nil {
		t.Fatal(err)
	}
	row, err := q.Event(ctx, r.eventID)
	if err != nil {
		t.Fatal(err)
	}
	var dates event.Configuration
	if err = json.Unmarshal(row.Configuration, &dates); err != nil {
		t.Fatal(err)
	}
	if !final {
		r.clock.set(dates.LiveStart.Add(-time.Hour))
		return f
	}
	definitionID := uuid.New()
	defs := templates.EmptyDefinitions()
	defs.Deliverables = []templates.Deliverable{
		{
			ID:            definitionID,
			Title:         "Native frozen demonstration",
			Required:      true,
			EvidenceKinds: []string{"url"},
			Audience:      []string{"organiser", "mentor", "judge"},
		},
	}
	raw, err := json.Marshal(defs)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.pool.Exec(
		ctx,
		"INSERT INTO event_definitions(event_id,definitions)VALUES($1,$2) ON CONFLICT(event_id)DO UPDATE SET definitions=EXCLUDED.definitions",
		r.eventID,
		raw,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = r.pool.Exec(
		ctx,
		"UPDATE events SET stage='live' WHERE id=$1",
		r.eventID,
	); err != nil {
		t.Fatal(err)
	}
	r.clock.set(dates.LiveStart.Add(time.Hour))
	page, err := f.surface.deps.Submissions.Read(ctx, r.competitor.ID, r.eventID, r.teamID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.surface.deps.Submissions.Submit(
		ctx,
		r.competitor.ID,
		r.eventID,
		r.teamID,
		page.Revision,
		submissions.Command{
			Reason: "Actual synthetic formal output",
			Evidence: []submissions.Evidence{
				{
					DeliverableID: definitionID,
					Kind:          "url",
					Label:         "Native frozen demonstration",
					URL:           "https://example.test/native-judge-demonstration",
				},
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	r.clock.set(dates.SubmissionCutoff)
	page, err = f.surface.deps.Submissions.Read(ctx, r.mentor.ID, r.eventID, r.teamID)
	if err != nil || page.Final == nil || page.Outcome != "frozen" {
		t.Fatal("actual native final fixture", err)
	}
	observed, err := f.surface.deps.Observations.Read(ctx, r.mentor.ID, r.eventID, r.teamID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.surface.deps.Observations.Change(
		ctx,
		r.mentor.ID,
		r.eventID,
		r.teamID,
		observed.Revision,
		observations.Command{
			ID:     uuid.New(),
			Action: "observation",
			Evidence: observations.Evidence{
				Category: "Private raw assessment",
				Evidence: "PRIVATE-NATIVE-MENTOR-NOTE-NOT-FOR-JUDGES",
				Outcome:  "Private qualification observation",
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err = f.surface.deps.Judging.ChangeSummary(
		ctx,
		r.mentor.ID,
		r.eventID,
		r.teamID,
		0,
		judging.SummaryCommand{
			RequestID:          uuid.New(),
			Action:             "save",
			SubmissionRevision: page.Final.Revision,
			Reason:             "Dedicated judge-facing factual summary",
			Facts: []judging.SummaryFact{
				{
					DeliverableID: definitionID,
					Fact:          "Mentor observed the frozen demonstration complete its stated synthetic workflow.",
				},
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err = f.surface.deps.Judging.ChangeSummary(
		ctx,
		r.owner.ID,
		r.eventID,
		r.teamID,
		1,
		judging.SummaryCommand{
			RequestID:          uuid.New(),
			Action:             "approve",
			SubmissionRevision: page.Final.Revision,
			Reason:             "Explicit exact-version factual review excluding private notes and qualification scores",
		},
	); err != nil {
		t.Fatal(err)
	}
	room, err := f.surface.deps.Presentation.Change(
		ctx,
		r.mc.ID,
		r.eventID,
		0,
		presentation.Command{
			Action: "save",
			Reason: "Synthetic actual published judging programme",
			Cues: []presentation.Cue{
				{
					ID:              uuid.New(),
					Title:           "Native demonstration slot",
					Text:            "Public room text is not part of the title-only order",
					DurationSeconds: 60,
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	index := 0
	if _, err = f.surface.deps.Presentation.Change(
		ctx,
		r.mc.ID,
		r.eventID,
		room.Revision,
		presentation.Command{
			Action:    "activate",
			FromDraft: true,
			Index:     &index,
			Reason:    "Publish native presentation slot",
		},
	); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestJudgeUIBrowsers(t *testing.T) {
	if os.Getenv("HACKBOX_JUDGE_BROWSER") != "1" {
		t.Skip(
			"set HACKBOX_INTEGRATION=1 HACKBOX_JUDGE_BROWSER=1 after building both production portals",
		)
	}
	for _, surface := range []string{"admin", "public"} {
		for _, viewport := range []string{"desktop", "mobile"} {
			t.Run(
				surface+"/"+viewport,
				func(t *testing.T) { runJudgeUIBrowser(t, surface, viewport) },
			)
		}
	}
}

func runJudgeUIBrowser(t *testing.T, surface, viewport string) {
	t.Helper()
	ctx := t.Context()
	practice, final := newJudgeBrowserEdition(
		t,
		surface,
		false,
	), newJudgeBrowserEdition(
		t,
		surface,
		true,
	)
	private, err := os.MkdirTemp("/tmp", "hackbox109-")
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
			var input struct{ Action, Edition, Actor string }
			if err := json.NewDecoder(http.MaxBytesReader(w, request.Body, 1024)).
				Decode(&input); err != nil {
				http.Error(w, "invalid private control", 400)
				return
			}
			switch input.Action {
			case "session":
				edition := final
				if input.Edition == "practice" {
					edition = practice
				} else if input.Edition != "final" {
					http.Error(w, "unknown edition", 400)
					return
				}
				person := edition.first
				if input.Actor == "owner" {
					person = edition.runtime.owner
				} else if input.Actor != "judge" {
					http.Error(w, "unknown synthetic actor", 400)
					return
				}
				person, err := dbgen.New(edition.runtime.pool).Person(request.Context(), person.ID)
				if err != nil {
					http.Error(w, "current actor unavailable", 500)
					return
				}
				cookie, err := edition.surface.sessionCookie(request.Context(), person)
				if err != nil {
					http.Error(w, "session unavailable", 500)
					return
				}
				_ = json.NewEncoder(w).Encode(cookie)
			case "expire":
				if _, err := final.runtime.pool.Exec(
					request.Context(),
					"UPDATE people SET session_revision=session_revision+1 WHERE id=$1",
					final.first.ID,
				); err != nil {
					http.Error(w, "expire failed", 500)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			case "evidence":
				var version int64
				var state string
				var practiceProduction, practiceCards int
				if err := final.runtime.pool.QueryRow(request.Context(), "SELECT coalesce((SELECT version FROM judge_cards WHERE event_id=$1 AND author_id=$2),0),coalesce((SELECT document->>'state' FROM judge_cards WHERE event_id=$1 AND author_id=$2),'not_started')", final.runtime.eventID, final.first.ID).
					Scan(&version, &state); err != nil {
					http.Error(w, "native card evidence unavailable", 500)
					return
				}
				if err := practice.runtime.pool.QueryRow(request.Context(), "SELECT (SELECT count(*) FROM judge_cards WHERE event_id=$1),(SELECT count(*) FROM judge_practice_cards WHERE event_id=$1)", practice.runtime.eventID).
					Scan(&practiceProduction, &practiceCards); err != nil {
					http.Error(w, "native practice evidence unavailable", 500)
					return
				}
				_ = json.NewEncoder(w).
					Encode(map[string]any{"version": version, "state": state, "practice_production_cards": practiceProduction, "practice_cards": practiceCards})
			default:
				http.Error(w, "unknown private action", 400)
			}
		}),
	}
	go func() { _ = control.Serve(listener) }()
	t.Cleanup(func() { _ = control.Close() })
	manifest := map[string]any{
		"event_id":          final.runtime.eventID,
		"team_id":           final.runtime.teamID,
		"run_id":            final.runID,
		"practice_event_id": practice.runtime.eventID,
		"judge_id":          final.first.ID,
		"final":             final.surface.server.URL,
		"practice":          practice.surface.server.URL,
		"socket":            socket,
		"surface":           surface,
		"viewport":          viewport,
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(private, "manifest.json")
	if err = os.WriteFile(manifestPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(
		ctx,
		"node",
		"node_modules/@playwright/test/cli.js",
		"test",
		"--config",
		"playwright.judging-native.config.ts",
	)
	command.Dir = filepath.Join("..", "..", "web")
	command.Env = append(
		os.Environ(),
		"HACKBOX_JUDGE_MANIFEST="+manifestPath,
		"HACKBOX_JUDGE_VIEWPORT="+viewport,
		"TMPDIR="+private,
	)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err = command.Run(); err != nil {
		t.Fatal("native judge UI journey", err)
	}
	source, err := exec.CommandContext(ctx, "git", "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	proof, err := json.MarshalIndent(
		map[string]any{
			"source":                            strings.TrimSpace(string(source)),
			"surface":                           surface,
			"viewport":                          viewport,
			"native_postgresql":                 true,
			"native_http_and_production_portal": true,
			"private_unix_control":              true,
			"providers":                         "none; URL-only final, no provider workers",
			"result":                            "passed",
		},
		"",
		"  ",
	)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(
		"..",
		"..",
		"docs",
		"verification",
		"judge-ui",
		fmt.Sprintf("native-%s-%s.json", surface, viewport),
	)
	if err = os.WriteFile(output, append(proof, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
