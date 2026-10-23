package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type appHTTPProvider struct{ code string }

func (p *appHTTPProvider) ValidateKey(_ int64, key []byte) bool { return len(key) > 0 }

func (p *appHTTPProvider) Inspect(
	_ context.Context,
	_ githubapp.Configuration,
	_ []byte,
	_ int64,
) githubapp.Diagnostics {
	return githubapp.Diagnostics{
		Code:      p.code,
		Checks:    []githubapp.PermissionCheck{},
		CheckedAt: time.Now().UTC(),
	}
}

func TestGitHubAppHTTPStrictPrivacyCSRFAndSessionBoundCallback(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	owner, other := testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	cfg, err := config.Load(func(name string) string {
		if name == "HACKBOX_ENV" {
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
	raw, _ := json.Marshal(
		map[string]any{
			"active": "test",
			"keys": map[string]string{
				"test": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32)),
			},
		},
	)
	keys, err := secretbox.Parse(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	provider := &appHTTPProvider{code: "healthy"}
	var logs bytes.Buffer
	router := NewRouter(
		cfg,
		slog.New(slog.NewTextHandler(&logs, nil)),
		Dependencies{
			Pool:      pool,
			Auth:      auth,
			GitHubApp: githubapp.Service{Pool: pool, Keys: keys, Provider: provider},
		},
	)
	tokens := map[uuid.UUID]string{}
	csrf := strings.Repeat("a", 32)
	revision := func() int64 {
		row, err := q.Event(ctx, eventID)
		if err != nil {
			t.Fatal(err)
		}
		return row.Revision
	}
	request := func(person dbgen.Person, method, path string, body any, validCSRF bool) *httptest.ResponseRecorder {
		t.Helper()
		var data []byte
		if body != nil {
			data, _ = json.Marshal(body)
		}
		r := httptest.NewRequest(method, path, bytes.NewReader(data))
		if body != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("If-Match", fmt.Sprintf("\"%d\"", revision()))
		if validCSRF {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		if person.ID != uuid.Nil {
			if tokens[person.ID] == "" {
				session, err := auth.Sessions.Load(ctx, "")
				if err != nil {
					t.Fatal(err)
				}
				auth.Sessions.Put(session, "person_id", person.ID.String())
				auth.Sessions.Put(session, "session_revision", person.SessionRevision)
				auth.Sessions.Put(session, "csrf", csrf)
				token, _, err := auth.Sessions.Commit(session)
				if err != nil {
					t.Fatal(err)
				}
				tokens[person.ID] = token
			}
			r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: tokens[person.ID]})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		return w
	}
	check := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("got %d want %d: %s", w.Code, status, w.Body.String())
		}
		if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("missing no-store")
		}
	}
	base := "/api/v1/events/" + eventID.String() + "/github-app"
	body := map[string]any{
		"action":              "configure",
		"app_id":              42,
		"app_slug":            "synthetic-app",
		"organization_id":     99,
		"organization":        "synthetic-org",
		"template_repository": "synthetic-template",
		"private_key":         "SYNTHETIC-HTTP-APP-PRIVATE-KEY",
	}
	check(request(owner, "POST", base, body, false), 400)
	check(request(other, "POST", base, body, true), 403)
	w := request(owner, "POST", base, body, true)
	check(w, 200)
	if strings.Contains(w.Body.String(), "PRIVATE-KEY") ||
		strings.Contains(w.Body.String(), "encrypted") ||
		strings.Contains(w.Body.String(), "key_id") {
		t.Fatal("configuration response disclosed credentials")
	}
	body["unknown_private_field"] = "SYNTHETIC-UNEXPECTED-SECRET"
	check(request(owner, "POST", base, body, true), 400)
	delete(body, "unknown_private_field")
	begin := func() string {
		t.Helper()
		w := request(owner, "POST", base+"/installation-authorizations", nil, true)
		check(w, 200)
		var result struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		u, err := url.Parse(result.URL)
		if err != nil {
			t.Fatal(err)
		}
		return u.Query().Get("state")
	}
	state := begin()
	callback := "/auth/github-app/callback?installation_id=7&setup_action=install&state=" + url.QueryEscape(
		state,
	)
	check(request(other, "GET", callback, nil, false), 400)
	w = request(owner, "GET", callback, nil, false)
	check(w, 303)
	if w.Header().Get("Location") != "/events/"+eventID.String()+"?github_app=connected" ||
		w.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("unsafe callback redirect: %v", w.Header())
	}
	check(request(owner, "GET", callback, nil, false), 400)
	provider.code = "installation_removed"
	w = request(owner, "GET", base, nil, true)
	check(w, 200)
	if !strings.Contains(w.Body.String(), "installation_removed") {
		t.Fatal("removed installation missing diagnostic")
	}
	state = begin()
	callback = "/auth/github-app/callback?installation_id=7&state=" + url.QueryEscape(state)
	if _, err = pool.Exec(
		ctx,
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='organiser'",
		eventID,
		owner.ID,
	); err != nil {
		t.Fatal(err)
	}
	check(request(owner, "GET", callback, nil, false), 403)
	check(request(owner, "GET", base, nil, true), 403)
	var auditText string
	if err = pool.QueryRow(ctx, "SELECT string_agg(metadata::text,' ') FROM audit_events WHERE event_id=$1", eventID).
		Scan(&auditText); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{logs.String(), auditText} {
		if strings.Contains(value, "PRIVATE-KEY") || strings.Contains(value, "UNEXPECTED-SECRET") ||
			strings.Contains(value, state) {
			t.Fatal("secret/state entered logs or audit")
		}
	}
}
