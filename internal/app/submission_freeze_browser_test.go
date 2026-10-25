package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissionfreeze"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
	"github.com/google/uuid"
)

type freezeBrowserApp struct {
	mu     sync.Mutex
	branch string
	calls  int
}

func (a *freezeBrowserApp) ObserveBranch(_ context.Context, _ *dbgen.Queries, _ uuid.UUID, _ githubapp.RepositoryRequest, _ int64, branch string) (githubapp.BranchObservation, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.branch = branch
	a.calls++
	return githubapp.BranchObservation{Status: "branch_missing"}, nil
}

// Own PostgreSQL/runtime and private test IPC. No public fixture endpoint,
// provider mutation, participant-code execution or shared event-clock rewind.
func TestSubmissionFreezeBrowsers(t *testing.T) {
	if os.Getenv("HACKBOX_FREEZE_BROWSER") != "1" {
		t.Skip("set HACKBOX_INTEGRATION=1 HACKBOX_FREEZE_BROWSER=1 after production builds")
	}
	for _, viewport := range []string{"desktop", "mobile"} {
		t.Run(viewport, func(t *testing.T) { runSubmissionFreezeBrowser(t, viewport) })
	}
}
func runSubmissionFreezeBrowser(t *testing.T, viewport string) {
	t.Helper()
	ctx := t.Context()
	r := newFreezeBrowserFixture(t)
	q := dbgen.New(r.pool)
	definitionID, demoID, provenance := uuid.New(), uuid.New(), uuid.New()
	defs := templates.EmptyDefinitions()
	defs.Deliverables = []templates.Deliverable{{ID: definitionID, Title: "Synthetic native code", Required: true, EvidenceKinds: []string{"repository"}, Audience: []string{"mentor", "judge"}}, {ID: demoID, Title: "Synthetic native demo", Required: true, EvidenceKinds: []string{"url"}, Audience: []string{"mentor", "judge"}}}
	raw, err := json.Marshal(defs)
	if err != nil {
		t.Fatal(err)
	}
	if err = q.SaveEventDefinitions(ctx, dbgen.SaveEventDefinitionsParams{EventID: r.eventID, Definitions: raw}); err != nil {
		t.Fatal(err)
	}
	repo := "freeze-ui-" + r.teamID.String()
	for _, command := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO github_app_integrations(event_id,app_id,app_slug,organization_id,organization,template_repository,credential_version,key_id,private_key_encrypted,installation_id) VALUES($1,42,'synthetic-app',99,'synthetic-org','template',1,'synthetic',decode('00','hex'),55)", []any{r.eventID}},
		{"INSERT INTO team_repositories(event_id,team_id,organization_id,organization,name,app_id,installation_id,template_repository,requested_by,provenance,github_repository_id,state) VALUES($1,$2,99,'synthetic-org',$3,42,55,'template',$4,$5,123456,'ready')", []any{r.eventID, r.teamID, repo, r.owner.ID, provenance}},
		{"INSERT INTO policy_resources(event_id,id,kind) VALUES($1,$2,'team') ON CONFLICT DO NOTHING", []any{r.eventID, r.teamID}},
		{"INSERT INTO resource_assignments(event_id,resource_id,person_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING", []any{r.eventID, r.teamID, r.mentor.ID}},
	} {
		if _, err = r.pool.Exec(ctx, command.sql, command.args...); err != nil {
			t.Fatal(err)
		}
	}
	admin, public := r.surface(t, "admin"), r.surface(t, "public")
	eventPage, err := admin.deps.Events.Read(ctx, r.owner.ID, r.eventID)
	if err != nil {
		t.Fatal(err)
	}
	// Advance from actual common publication to the live window, never backwards.
	r.clock.set(eventPage.Configuration.LiveStart)
	if _, err = admin.deps.Events.Transition(ctx, r.owner.ID, r.eventID, eventPage.Revision, "live", "Synthetic organiser starts the live hack", false); err != nil {
		t.Fatal(err)
	}
	rounds, err := admin.deps.Judging.Rounds(ctx, r.owner.ID, r.eventID)
	if err != nil || len(rounds.Items) != 1 {
		t.Fatalf("current native judge round: %+v %v", rounds, err)
	}
	runID := rounds.Items[0].RunID
	if _, err = admin.deps.Judging.ChangeRound(ctx, r.owner.ID, r.eventID, runID, rounds.Items[0].Revision, judging.RoundCommand{RequestID: uuid.New(), Action: "assign", TeamID: r.teamID, JudgeIDs: []uuid.UUID{r.mc.ID}, Reason: "Synthetic current judge assignment"}); err != nil {
		t.Fatal(err)
	}
	app := &freezeBrowserApp{}
	freeze := &submissionfreeze.Service{Pool: r.pool, App: app, Now: r.clock.read}
	queue, err := jobs.New(r.pool, slog.New(slog.NewTextHandler(io.Discard, nil)), map[string]jobs.Handler{submissionfreeze.HandlerKind: freeze})
	if err != nil {
		t.Fatal(err)
	}
	freeze.Jobs = queue
	private, err := os.MkdirTemp("/tmp", "hackbox97-")
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
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var in struct{ Action, Actor, Surface string }
		if request.Method != "POST" || json.NewDecoder(http.MaxBytesReader(w, request.Body, 1024)).Decode(&in) != nil {
			http.Error(w, "invalid fixture control", 400)
			return
		}
		var actionErr error
		switch in.Action {
		case "session":
			actor := r.competitor
			switch in.Actor {
			case "owner":
				actor = r.owner
			case "mentor":
				actor = r.mentor
			case "judge":
				actor = r.mc
			case "competitor":
			default:
				http.Error(w, "unknown synthetic actor", 400)
				return
			}
			surface := public
			if in.Surface == "admin" {
				surface = admin
			} else if in.Surface != "public" {
				http.Error(w, "unknown surface", 400)
				return
			}
			currentActor, loadErr := q.Person(request.Context(), actor.ID)
			if loadErr != nil {
				http.Error(w, "current actor unavailable", 500)
				return
			}
			cookie, cookieErr := surface.sessionCookie(request.Context(), currentActor)
			if cookieErr != nil {
				http.Error(w, "fixture authentication failed", 500)
				return
			}
			_ = json.NewEncoder(w).Encode(cookie)
			return
		case "submit":
			s := submissions.Service{Pool: r.pool, Clock: r.clock.read}
			p, e := s.Read(request.Context(), r.competitor.ID, r.eventID, r.teamID)
			if e != nil {
				actionErr = e
				break
			}
			_, actionErr = s.Submit(request.Context(), r.competitor.ID, r.eventID, r.teamID, p.Revision, submissions.Command{Evidence: []submissions.Evidence{{DeliverableID: definitionID, Kind: "repository", Label: "Synthetic native repository", URL: "https://github.com/synthetic-org/" + repo}, {DeliverableID: demoID, Kind: "url", Label: "Synthetic native live demo", URL: "https://demo.example.test/current"}}})
		case "cutoff":
			r.clock.set(eventPage.Configuration.SubmissionCutoff)
			if actionErr = freeze.Schedule(request.Context(), r.eventID); actionErr != nil {
				break
			}
			var id uuid.UUID
			actionErr = r.pool.QueryRow(request.Context(), "SELECT id FROM operations WHERE event_id=$1 AND target_id=$2 AND kind=$3", r.eventID, r.teamID, submissionfreeze.HandlerKind).Scan(&id)
			if actionErr == nil {
				actionErr = queue.Execute(request.Context(), id)
			}
		case "remove-mentor":
			_, actionErr = r.pool.Exec(request.Context(), "DELETE FROM resource_assignments WHERE event_id=$1 AND resource_id=$2 AND person_id=$3", r.eventID, r.teamID, r.mentor.ID)
		case "judge-conflict":
			current, readErr := admin.deps.Judging.Round(request.Context(), r.owner.ID, r.eventID, runID)
			if readErr != nil {
				actionErr = readErr
				break
			}
			_, actionErr = admin.deps.Judging.ChangeRound(request.Context(), r.owner.ID, r.eventID, runID, current.Revision, judging.RoundCommand{RequestID: uuid.New(), Action: "declare_conflict", JudgeID: r.mc.ID, CompetitorID: r.competitor.ID, ConflictKind: "reporting", Enabled: true, Reason: "Synthetic explicitly declared reporting conflict"})
		case "evidence":
			app.mu.Lock()
			defer app.mu.Unlock()
			var commands, decisions int
			actionErr = r.pool.QueryRow(request.Context(), "SELECT count(*) FROM submission_freeze_commands WHERE event_id=$1 AND team_id=$2", r.eventID, r.teamID).Scan(&commands)
			if actionErr != nil {
				break
			}
			actionErr = r.pool.QueryRow(request.Context(), "SELECT count(*) FROM submission_freeze_decisions d JOIN submission_freeze_sources s ON s.id=d.source_id WHERE s.event_id=$1 AND s.team_id=$2", r.eventID, r.teamID).Scan(&decisions)
			if actionErr != nil {
				break
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"commands": commands, "decisions": decisions, "provider_calls": app.calls, "observed_branch": app.branch})
			return
		default:
			http.Error(w, "unknown fixture control", 400)
			return
		}
		if actionErr != nil {
			http.Error(w, actionErr.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	manifest := map[string]any{"event_id": r.eventID, "team_id": r.teamID, "judge_run": runID, "admin": admin.server.URL, "public": public.server.URL, "socket": socket, "viewport": viewport}
	raw, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(private, "manifest.json")
	if err = os.WriteFile(manifestPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, "node", "node_modules/@playwright/test/cli.js", "test", "--config", "playwright.freeze-native.config.ts")
	command.Dir = filepath.Join("..", "..", "web")
	command.Env = append(os.Environ(), "HACKBOX_FREEZE_MANIFEST="+manifestPath, "TMPDIR="+private)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	source, err := exec.CommandContext(ctx, "git", "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	status, err := exec.CommandContext(ctx, "git", "status", "--porcelain", "--untracked-files=normal").Output()
	if err != nil {
		t.Fatal(err)
	}
	runErr := command.Run()
	end, err := exec.CommandContext(ctx, "git", "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	proof := map[string]any{"source_commit": strings.TrimSpace(string(source)), "source_commit_end": strings.TrimSpace(string(end)), "source_worktree_clean_start": len(strings.TrimSpace(string(status))) == 0, "viewport": viewport, "browser_command_succeeded": runErr == nil, "test_selection": "unfiltered"}
	raw, _ = json.MarshalIndent(proof, "", "  ")
	proofDir := filepath.Join(command.Dir, "test-results-freeze-native", viewport)
	if err = os.MkdirAll(proofDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(proofDir, "native-fixture.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if string(source) != string(end) {
		t.Fatal("source changed during native proof")
	}
	if runErr != nil {
		t.Fatal(runErr)
	}
}

// Start with a separate historical fixture: durable jobs deliberately use real
// lease/due time. Never rewind the existing future-dated live rehearsal or
// rewrite an operation due date to make a cutoff job claimable.
func newFreezeBrowserFixture(t *testing.T) *liveRehearsal {
	t.Helper()
	live := time.Date(2026, 6, 20, 9, 0, 0, 0, time.UTC)
	publication := live.AddDate(0, 0, -7)
	r := &liveRehearsal{pool: testdb.New(t, true), clock: &rehearsalClock{now: publication.Add(-time.Minute)}}
	r.owner = testdb.Person(t, r.pool)
	r.mentor = testdb.Person(t, r.pool)
	r.competitor = testdb.Person(t, r.pool)
	r.mc = testdb.Person(t, r.pool) // Separate synthetic judge; no MC role is granted.
	r.eventID = testdb.Event(t, r.pool, r.owner.ID)
	r.teamID = uuid.New()
	c := event.DefaultConfiguration()
	c.MinimumAttendance = 1
	c.LiveStart = live
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	c.SubmissionCutoff = c.LiveStart.Add(7 * time.Hour)
	c.TeamFreeze = c.LiveStart.Add(-8 * 24 * time.Hour)
	c.AdmissionCutoff = c.TeamFreeze.Add(-time.Hour)
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []struct {
		sql  string
		args []any
	}{
		{"UPDATE events SET stage='preparation',configuration=$2,team_frozen_at=$3 WHERE id=$1", []any{r.eventID, raw, c.TeamFreeze}},
		{"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mentor')", []any{r.eventID, r.mentor.ID}},
		{"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'judge')", []any{r.eventID, r.mc.ID}},
		{"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')", []any{r.eventID, r.competitor.ID}},
		{"INSERT INTO hack_teams(event_id,id,name,state,frozen,published_captain_id) VALUES($1,$2,'Synthetic native build team','published',true,$3)", []any{r.eventID, r.teamID, r.competitor.ID}},
		{"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')", []any{r.eventID, r.teamID, r.competitor.ID}},
		{"INSERT INTO competition_entries(event_id,team_id) VALUES($1,$2)", []any{r.eventID, r.teamID}},
		{"INSERT INTO event_gate_facts(event_id,attendance_ready,teams_ready,total_teams,frozen_teams,assigned_attendees)VALUES($1,true,true,1,1,1)", []any{r.eventID}},
	} {
		if _, err = r.pool.Exec(t.Context(), cmd.sql, cmd.args...); err != nil {
			t.Fatal(err)
		}
	}
	criteria := rubric.Service{Pool: r.pool, Clock: r.clock.read}
	definition := rubric.Default(rubric.AllTeams, r.eventID)
	definition.RequiredJudgeCount = 1
	definition.PrivateJudgeGuidance = "PRIVATE-SYNTHETIC-JUDGE-GUIDANCE"
	version, err := criteria.Change(t.Context(), r.owner.ID, r.eventID, 0, rubric.Command{Action: "save", Definition: &definition, Reason: "Synthetic configured criteria"})
	if err != nil {
		t.Fatal(err)
	}
	version, err = criteria.Change(t.Context(), r.owner.ID, r.eventID, version.Revision, rubric.Command{Action: "approve", Version: version.Reference.Version, Reason: "Synthetic criteria approval"})
	if err != nil {
		t.Fatal(err)
	}
	common := content.Service{Pool: r.pool, Clock: r.clock.read}
	challenge, err := common.Save(t.Context(), r.owner.ID, r.eventID, uuid.New(), 0, content.Input{Kind: "challenge", Document: content.Document{Title: "Synthetic native challenge", Markdown: "Build a fictional prototype", Audience: access.ContentAudience{Confirmed: true}, Rubric: &version.Reference}})
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"review", "approve"} {
		challenge, err = common.Action(t.Context(), r.owner.ID, r.eventID, challenge.ID, challenge.Revision, action, "Synthetic reviewed common challenge")
		if err != nil {
			t.Fatal(err)
		}
	}
	r.clock.set(publication)
	if err = common.Dispatch(t.Context(), r.eventID, challenge.ID, challenge.Revision); err != nil {
		t.Fatal(err)
	}
	version, err = criteria.Configuration(t.Context(), r.owner.ID, r.eventID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = criteria.Change(t.Context(), r.owner.ID, r.eventID, version.Revision, rubric.Command{Action: "create_run", Purpose: "judge_final", Version: version.Reference.Version, Reason: "Synthetic independent final judging"}); err != nil {
		t.Fatal(err)
	}
	return r
}
