package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissionfreeze"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
	"github.com/google/uuid"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSubmissionFreezeHTTPAuthorityAllowlistsAndOriginalReceipt(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	owner, member, outsider := testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool)
	id, team := testdb.Event(t, pool, owner.ID), uuid.New()
	now := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	c := event.DefaultConfiguration()
	c.LiveStart = now.Add(-time.Hour)
	c.LiveEnd = now.Add(7 * time.Hour)
	c.SubmissionCutoff = now.Add(6 * time.Hour)
	raw, _ := json.Marshal(c)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("UPDATE events SET configuration=$2,published_at=$3,stage='preparation' WHERE id=$1", id, raw, now.Add(-7*24*time.Hour))
	exec("INSERT INTO hack_teams(id,event_id,name,state,frozen) VALUES($1,$2,'Synthetic API team','published',true)", team, id)
	exec("INSERT INTO competition_entries(event_id,team_id) VALUES($1,$2)", id, team)
	exec("INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')", id, team, member.ID)
	if err := q.SetCompetitionState(ctx, dbgen.SetCompetitionStateParams{EventID: id, PersonID: member.ID, State: "confirmed"}); err != nil {
		t.Fatal(err)
	}
	exec("INSERT INTO team_repositories(event_id,team_id,organization_id,organization,name,app_id,installation_id,template_repository,requested_by,github_repository_id,state) VALUES($1,$2,99,'synthetic-org',$3,42,55,'template',$4,700,'ready')", id, team, "synthetic-"+team.String(), owner.ID)
	cfg, err := config.Load(func(k string) string {
		if k == "HACKBOX_ENV" {
			return "test"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	auth, err := identity.New(ctx, pool, identity.Settings{Origin: cfg.PublicOrigin})
	if err != nil {
		t.Fatal(err)
	}
	service := submissionfreeze.Service{Pool: pool, Now: func() time.Time { return now }}
	router := NewRouter(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{Pool: pool, Auth: auth, SubmissionFreeze: service})
	request := func(person dbgen.Person, method, path string, body any, version string, csrfOK bool) *httptest.ResponseRecorder {
		t.Helper()
		sessionCtx, err := auth.Sessions.Load(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		csrf := strings.Repeat("a", 32)
		auth.Sessions.Put(sessionCtx, "person_id", person.ID.String())
		auth.Sessions.Put(sessionCtx, "session_revision", person.SessionRevision)
		auth.Sessions.Put(sessionCtx, "csrf", csrf)
		token, _, err := auth.Sessions.Commit(sessionCtx)
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(encoded))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("If-Match", version)
		if !csrfOK {
			csrf = "incorrect"
		}
		r.Header.Set("X-CSRF-Token", csrf)
		r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		return w
	}
	path := fmt.Sprintf("/api/v1/events/%s/teams/%s/submission-freeze", id, team)
	requestID := uuid.New()
	body := map[string]any{"request_id": requestID, "action": "configure", "branch": "release/build"}
	if w := request(outsider, "GET", path, nil, "\"0\"", true); w.Code != 403 {
		t.Fatal(w.Code, w.Body)
	}
	if w := request(member, "POST", path, body, "\"0\"", false); w.Code != 403 {
		t.Fatal(w.Code, w.Body)
	}
	body["private_key"] = "must-not-be-accepted"
	if w := request(member, "POST", path, body, "\"0\"", true); w.Code != 400 {
		t.Fatal("unknown secret field accepted", w.Code, w.Body)
	}
	delete(body, "private_key")
	if w := request(member, "POST", path, body, "garbage", true); w.Code != 400 {
		t.Fatal("malformed reviewed revision accepted", w.Code, w.Body)
	}
	if response := request(member, "POST", path, body, "\"9223372036854775808\"", true); response.Code != 400 {
		t.Fatal("overflowed reviewed revision accepted as zero", response.Code, response.Body)
	}

	w := request(member, "POST", path, body, "\"0\"", true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	var ack submissionfreeze.Receipt
	if err = json.Unmarshal(w.Body.Bytes(), &ack); err != nil {
		t.Fatal(err)
	}
	if w = request(member, "GET", path+"/commands/"+requestID.String(), nil, "\"0\"", true); w.Code != 200 || !strings.Contains(w.Body.String(), ack.Fingerprint) {
		t.Fatal(w.Code, w.Body)
	}
	if w = request(owner, "GET", path+"/commands/"+requestID.String(), nil, "\"0\"", true); w.Code != 200 || strings.Contains(w.Body.String(), ack.Fingerprint) {
		t.Fatal("other actor received command", w.Code, w.Body)
	}
	body["request_id"] = uuid.New()
	if w = request(member, "POST", path, body, "\"0\"", true); w.Code != 409 {
		t.Fatal("stale revision accepted", w.Code, w.Body)
	}
	w = request(member, "GET", path, nil, "\"1\"", true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	var fields map[string]any
	if err = json.Unmarshal(w.Body.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{}
	for _, key := range []string{"current_reference_status", "native_revision", "native_outcome", "current_submission_revision", "binding_current", "revision", "branch", "repository_id", "repository_url", "cutoff", "can_configure", "can_reconcile", "can_amend", "status", "source", "observations", "decisions", "current"} {
		allowed[key] = true
	}
	for key := range fields {
		if !allowed[key] {
			t.Fatal("unapproved response field", key)
		}
	}
	for _, private := range []string{"private_key", "encrypted", "credential_version", "installation_id", "provider_body", "working_draft"} {
		if strings.Contains(w.Body.String(), private) {
			t.Fatal("private response field", private)
		}
	}
	if w = request(member, "GET", path+"/push-evidence", nil, "\"1\"", true); w.Code != 403 {
		t.Fatal("participant got organiser evidence", w.Code, w.Body)
	}
	// Real accepted native final -> queued capture -> organiser reconciliation.
	// Recovery and identical HTTP replay must re-project current authority.
	deliverable := uuid.New()
	defs := templates.EmptyDefinitions()
	defs.Deliverables = []templates.Deliverable{{ID: deliverable, Title: "Build", Required: true, EvidenceKinds: []string{"repository"}}}
	encodedDefs, _ := json.Marshal(defs)
	if err = q.SaveEventDefinitions(ctx, dbgen.SaveEventDefinitionsParams{EventID: id, Definitions: encodedDefs}); err != nil {
		t.Fatal(err)
	}
	native := submissions.Service{Pool: pool, Clock: service.Now}
	nativePage, err := native.Read(ctx, member.ID, id, team)
	if err != nil {
		t.Fatal(err)
	}
	_, err = native.Submit(ctx, member.ID, id, team, nativePage.Revision, submissions.Command{Evidence: []submissions.Evidence{{DeliverableID: deliverable, Kind: "repository", Label: "Managed build", URL: "https://github.com/synthetic-org/synthetic-" + team.String()}}})
	if err != nil {
		t.Fatal(err)
	}
	exec("INSERT INTO github_app_integrations(event_id,app_id,app_slug,organization_id,organization,template_repository,credential_version,key_id,private_key_encrypted,installation_id) VALUES($1,42,'synthetic-app',99,'synthetic-org','template',1,'synthetic',decode('00','hex'),55)", id)
	service.App = freezeHTTPObserver{}
	queue, err := jobs.New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), map[string]jobs.Handler{submissionfreeze.HandlerKind: &service})
	if err != nil {
		t.Fatal(err)
	}
	service.Jobs = queue
	now = c.SubmissionCutoff
	if err = service.Schedule(ctx, id); err != nil {
		t.Fatal(err)
	}
	var operation uuid.UUID
	if err = pool.QueryRow(ctx, "SELECT id FROM operations WHERE event_id=$1 AND kind=$2", id, submissionfreeze.HandlerKind).Scan(&operation); err != nil {
		t.Fatal(err)
	}
	if err = queue.Execute(ctx, operation); err != nil {
		t.Fatal(err)
	}
	current, err := service.Read(ctx, owner.ID, id, team)
	if err != nil {
		t.Fatal(err)
	}
	decisionID := uuid.New()
	decisionBody := map[string]any{"request_id": decisionID, "action": "reconcile", "source_id": current.Source.ID, "native_revision": current.NativeRevision, "submission_revision": *current.CurrentSubmissionRevision, "sha": strings.Repeat("a", 40), "reason": "PRIVATE-HTTP-ORGANISER-REASON", "external_evidence": "PRIVATE-HTTP-PROVENANCE", "observation_ids": []uuid.UUID{current.Observations[0].ID}}
	reviewedVersion := fmt.Sprintf("\"%d\"", current.Revision)
	w = request(owner, "POST", path, decisionBody, reviewedVersion, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	var originalDecision submissionfreeze.Receipt
	if err = json.Unmarshal(w.Body.Bytes(), &originalDecision); err != nil {
		t.Fatal(err)
	}
	exec("INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mentor')", id, owner.ID)
	exec("INSERT INTO policy_resources(event_id,id,kind) VALUES($1,$2,'team')", id, team)
	exec("INSERT INTO resource_assignments(event_id,resource_id,person_id) VALUES($1,$2,$3)", id, team, owner.ID)
	exec("UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='organiser'", id, owner.ID)
	for _, response := range []*httptest.ResponseRecorder{request(owner, "GET", path+"/commands/"+decisionID.String(), nil, reviewedVersion, true), request(owner, "POST", path, decisionBody, reviewedVersion, true)} {
		if response.Code != 200 || strings.Contains(response.Body.String(), "PRIVATE-HTTP-") {
			t.Fatal("current mentor recovered private original HTTP decision", response.Code, response.Body)
		}
	}
	var count int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM submission_freeze_decisions d JOIN submission_freeze_sources s ON s.id=d.source_id WHERE s.event_id=$1", id).Scan(&count); err != nil || count != 1 {
		t.Fatal("HTTP recovery/replay created a new decision", count, err)
	}
	var retained []byte
	if err = pool.QueryRow(ctx, "SELECT result FROM submission_freeze_commands WHERE event_id=$1 AND request_id=$2", id, decisionID).Scan(&retained); err != nil || !strings.Contains(string(retained), "PRIVATE-HTTP-ORGANISER-REASON") {
		t.Fatal("projection erased immutable original command", err)
	}

	exec("UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2", id, member.ID)
	if w = request(member, "GET", path+"/commands/"+requestID.String(), nil, "\"1\"", true); w.Code != 403 {
		t.Fatal("lost authority recovered private ACK", w.Code, w.Body)
	}
}

type freezeHTTPObserver struct{}

func (freezeHTTPObserver) ObserveBranch(context.Context, *dbgen.Queries, uuid.UUID, githubapp.RepositoryRequest, int64, string) (githubapp.BranchObservation, error) {
	return githubapp.BranchObservation{SHA: strings.Repeat("a", 40), Status: "current_ref"}, nil
}
