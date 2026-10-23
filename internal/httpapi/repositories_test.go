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

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/repositories"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestRepositoryHTTPCurrentAuthorityContractAndPreliveAccess(t *testing.T) {
	pool := testdb.New(t, true)
	owner, member, other := testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	team := uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(t.Context(), query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(
		"INSERT INTO hack_teams(id,event_id,name,state) VALUES($1,$2,'Synthetic team','published')",
		team,
		eventID,
	)
	exec("INSERT INTO competition_entries(event_id,team_id) VALUES($1,$2)", eventID, team)
	exec(
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
		eventID,
		team,
		member.ID,
	)
	exec(
		"INSERT INTO github_app_integrations(event_id,app_id,app_slug,organization_id,organization,template_repository,credential_version,key_id,private_key_encrypted,installation_id) VALUES($1,42,'synthetic-app',99,'synthetic-org','template',1,'test',decode('00','hex'),55)",
		eventID,
	)
	cfg, err := config.Load(func(key string) string {
		if key == "HACKBOX_ENV" {
			return "test"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	auth, err := identity.New(t.Context(), pool, identity.Settings{Origin: cfg.PublicOrigin})
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Dependencies{Pool: pool, Auth: auth, Repositories: repositories.Service{Pool: pool}},
	)
	csrf := strings.Repeat("b", 32)
	tokens := map[uuid.UUID]string{}
	base := "/api/v1/events/" + eventID.String() + "/repositories"
	request := func(person dbgen.Person, method string, body any, validCSRF bool, revision int64) *httptest.ResponseRecorder {
		t.Helper()
		var data []byte
		if body != nil {
			data, _ = json.Marshal(body)
		}
		r := httptest.NewRequest(method, base, bytes.NewReader(data))
		if body != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("If-Match", fmt.Sprintf("\"%d\"", revision))
		if validCSRF {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		if tokens[person.ID] == "" {
			ctx, err := auth.Sessions.Load(t.Context(), "")
			if err != nil {
				t.Fatal(err)
			}
			auth.Sessions.Put(ctx, "person_id", person.ID.String())
			auth.Sessions.Put(ctx, "session_revision", person.SessionRevision)
			auth.Sessions.Put(ctx, "csrf", csrf)
			token, _, err := auth.Sessions.Commit(ctx)
			if err != nil {
				t.Fatal(err)
			}
			tokens[person.ID] = token
		}
		r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: tokens[person.ID]})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		return w
	}
	check := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("got%d want%d: %s", w.Code, status, w.Body.String())
		}
		if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("missing no-store")
		}
	}
	e, _ := dbgen.New(pool).Event(t.Context(), eventID)
	body := map[string]any{"action": "queue"}
	check(request(owner, "POST", body, false, e.Revision), 400)
	check(request(other, "POST", body, true, e.Revision), 403)
	check(request(owner, "POST", body, true, e.Revision+1), 409)
	check(
		request(
			owner,
			"POST",
			map[string]any{"action": "queue", "private": false},
			true,
			e.Revision,
		),
		400,
	)
	w := request(owner, "POST", body, true, e.Revision)
	check(w, 200)
	if strings.Contains(w.Body.String(), "provenance") ||
		strings.Contains(w.Body.String(), "key_id") ||
		strings.Contains(w.Body.String(), "installation_id") {
		t.Fatal("private provider state exposed")
	}
	check(request(member, "GET", nil, false, e.Revision), 403)
	exec("UPDATE events SET published_at=now(),stage='preparation' WHERE id=$1", eventID)
	w = request(member, "GET", nil, false, e.Revision)
	check(w, 200)
	if !strings.Contains(w.Body.String(), team.String()) {
		t.Fatal("current prelive member cannot read own repo")
	}
	check(request(other, "GET", nil, false, e.Revision), 403)
	check(request(member, "POST", body, true, e.Revision), 403)
	check(request(owner, "POST", map[string]any{"action": "reconcile"}, true, e.Revision), 400)
	exec("UPDATE events SET writes_allowed=false WHERE id=$1", eventID)
	check(request(owner, "POST", body, true, e.Revision), 403)
	check(request(owner, "GET", nil, false, e.Revision), 200)
}
