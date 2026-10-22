package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestSubmissionHTTPContractsAndProtectedFiles(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
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
	owner, member, stranger := testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	)
	id := testdb.Event(t, pool, owner.ID)
	team, repo, notes := uuid.New(), uuid.New(), uuid.New()
	c := event.DefaultConfiguration()
	c.LiveStart = time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	c.SubmissionCutoff = c.LiveEnd.Add(-time.Hour)
	now := c.LiveStart.Add(-time.Hour)
	data, _ := json.Marshal(c)
	record, err := q.SaveEventConfiguration(
		ctx,
		dbgen.SaveEventConfigurationParams{
			ID:            id,
			Name:          "Synthetic submissions",
			Slug:          uuid.NewString(),
			Configuration: data,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(
		"UPDATE events SET published_at=$2,stage='preparation' WHERE id=$1",
		id,
		c.LiveStart.Add(-7*24*time.Hour),
	)
	exec(
		"INSERT INTO hack_teams(id,event_id,name,state,frozen) VALUES($1,$2,'Synthetic team','published',true)",
		team,
		id,
	)
	if err = q.CreateCompetitionEntry(
		ctx,
		dbgen.CreateCompetitionEntryParams{EventID: id, TeamID: team},
	); err != nil {
		t.Fatal(err)
	}
	exec(
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
		id,
		team,
		member.ID,
	)
	if err = q.SetCompetitionState(
		ctx,
		dbgen.SetCompetitionStateParams{EventID: id, PersonID: member.ID, State: "confirmed"},
	); err != nil {
		t.Fatal(err)
	}
	if err = q.InitializeEventFacts(ctx, id); err != nil {
		t.Fatal(err)
	}
	defs := templates.EmptyDefinitions()
	defs.Deliverables = []templates.Deliverable{
		{ID: repo, Title: "Repository", Required: true, EvidenceKinds: []string{"repository"}},
		{ID: notes, Title: "Notes", EvidenceKinds: []string{"file"}},
	}
	data, _ = json.Marshal(defs)
	if err = q.SaveEventDefinitions(
		ctx,
		dbgen.SaveEventDefinitionsParams{EventID: id, Definitions: data},
	); err != nil {
		t.Fatal(err)
	}
	storage, err := files.OpenLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	service := submissions.Service{Pool: pool, Clock: func() time.Time { return now }}
	router := NewRouter(
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Dependencies{
			Pool:        pool,
			Auth:        auth,
			Submissions: service,
			Files: files.Service{
				Pool:       pool,
				Storage:    storage,
				Policy:     access.EventPolicy{Queries: q, Clock: func() time.Time { return now }},
				MaxBytes:   1000,
				EventQuota: 10000,
			},
		},
	)
	request := func(person dbgen.Person, method, path string, body any, revision int64, csrfOK bool) *httptest.ResponseRecorder {
		t.Helper()
		session, err := auth.Sessions.Load(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		csrf := strings.Repeat("a", 32)
		auth.Sessions.Put(session, "person_id", person.ID.String())
		auth.Sessions.Put(session, "session_revision", person.SessionRevision)
		auth.Sessions.Put(session, "csrf", csrf)
		token, _, err := auth.Sessions.Commit(session)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("If-Match", fmt.Sprintf("\"%d\"", revision))
		r.Header.Set("Idempotency-Key", uuid.NewString())
		if !csrfOK {
			csrf = strings.Repeat("b", 32)
		}
		r.Header.Set("X-CSRF-Token", csrf)
		r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		return w
	}
	check := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("want %d, got %d: %s", status, w.Code, w.Body)
		}
	}
	base := "/api/v1/events/" + id.String()
	path := base + "/teams/" + team.String() + "/submission"
	evidence := []submissions.Evidence{
		{
			DeliverableID: repo,
			Kind:          "repository",
			Label:         "Repository",
			URL:           "https://example.test/synthetic/repo",
		},
	}
	command := submissions.Command{Evidence: evidence}
	check(request(member, "GET", path, nil, 0, true), 200)
	check(request(member, "GET", path+"/draft", nil, 0, true), 200)
	check(
		request(member, "PUT", path+"/draft", map[string]any{"evidence": evidence}, 0, false),
		403,
	)
	check(request(member, "PUT", path+"/draft", map[string]any{"evidence": evidence}, 0, true), 200)
	check(request(owner, "GET", path+"/draft", nil, 0, true), 403)
	check(request(stranger, "GET", path, nil, 0, true), 403)
	check(request(member, "PUT", path, command, 0, true), 409)
	check(
		request(
			owner,
			"POST",
			path+"/review",
			map[string]any{"submission_revision": 1, "status": "accepted", "reason": "Too early"},
			0,
			true,
		),
		409,
	)
	upload := map[string]any{
		"team_id":    team,
		"filename":   "notes.txt",
		"media_type": "text/plain",
		"content":    []byte("synthetic notes"),
	}
	uploaded := request(member, "POST", base+"/files", upload, record.Revision, true)
	check(uploaded, 201)
	var file struct {
		ID uuid.UUID `json:"id"`
	}
	if err = json.Unmarshal(uploaded.Body.Bytes(), &file); err != nil {
		t.Fatal(err)
	}
	check(request(owner, "GET", base+"/files/"+file.ID.String(), nil, 0, true), 403)
	now = c.LiveStart
	command.Evidence = append(
		command.Evidence,
		submissions.Evidence{DeliverableID: notes, Kind: "file", Label: "Notes", FileID: &file.ID},
	)
	submitted := request(member, "PUT", path, command, 0, true)
	check(submitted, 200)
	var current submissions.Page
	if err = json.Unmarshal(submitted.Body.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	if current.Final == nil || current.Final.Revision != 1 {
		t.Fatalf("missing formal revision: %+v", current)
	}
	check(request(owner, "GET", base+"/files/"+file.ID.String(), nil, 0, true), 200)
	check(request(stranger, "GET", base+"/files/"+file.ID.String(), nil, 0, true), 403)
	check(request(member, "PUT", path, command, 0, true), 409)
	check(
		request(
			owner,
			"POST",
			path+"/review",
			map[string]any{
				"submission_revision": 1,
				"status":              "accepted",
				"reason":              "Evidence checked",
			},
			current.Revision,
			true,
		),
		200,
	)
	check(request(member, "GET", path+"/history?limit=1", nil, 0, true), 200)
	now = c.SubmissionCutoff
	check(request(member, "PUT", path, command, 2, true), 409)
	frozen := request(owner, "GET", path, nil, 0, true)
	check(frozen, 200)
	if err = json.Unmarshal(frozen.Body.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	if current.Outcome != "frozen" || current.FrozenAt == nil {
		t.Fatalf("cutoff not enforced: %+v", current)
	}
	check(
		request(
			owner,
			"POST",
			path+"/outcome",
			map[string]any{"outcome": "withdrawn", "reason": "Team withdrew"},
			current.Revision,
			true,
		),
		200,
	)
	check(request(member, "GET", path+"/history", nil, 0, true), 200)
	exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		id,
		member.ID,
	)
	check(request(member, "GET", path+"/draft", nil, 0, true), 403)
	check(request(member, "GET", base+"/files/"+file.ID.String(), nil, 0, true), 403)
}
