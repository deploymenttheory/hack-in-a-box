package httpapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubidentity"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestGitHubHTTPCurrentAccountCSRFAndSafeCallback(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	person, other := testdb.Person(t, pool), testdb.Person(t, pool)
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
	github, err := githubidentity.New(
		pool,
		githubidentity.Settings{
			ClientID:     "synthetic",
			ClientSecret: "never-return-secret",
			TokenKey:     base64.StdEncoding.EncodeToString(make([]byte, 32)),
			Origin:       cfg.PublicOrigin,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Dependencies{Pool: pool, Auth: auth, GitHub: github},
	)
	csrf := strings.Repeat("x", 32)
	login := func(person dbgen.Person) string {
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
		return token
	}
	browser, otherBrowser := login(person), login(other)
	request := func(token, method, path string, body any, revision int64, csrfToken string) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("If-Match", fmt.Sprintf("\"%d\"", revision))
		r.Header.Set("X-CSRF-Token", csrfToken)
		if token != "" {
			r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: token})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if strings.HasPrefix(path, "/api/") {
			validateResponse(t, r, w)
		}
		return w
	}
	path := "/api/v1/account/github"
	ack := map[string]bool{"acknowledge_invitation_impact": true}
	if w := request(
		browser,
		"GET",
		path,
		nil,
		0,
		"",
	); w.Code != 200 ||
		!strings.Contains(w.Body.String(), `"account":null`) {
		t.Fatalf("unlinked account: %d %s", w.Code, w.Body)
	}
	for _, token := range []string{"", browser} {
		w := request(token, "POST", path+"/authorize", ack, 0, strings.Repeat("z", 32))
		if w.Code != 401 && w.Code != 403 {
			t.Fatalf("authorization ignored session/CSRF: %d %s", w.Code, w.Body)
		}
	}
	if w := request(
		browser,
		"POST",
		path+"/authorize",
		map[string]any{"acknowledge_invitation_impact": true, "login": "typed-name"},
		0,
		csrf,
	); w.Code != 400 {
		t.Fatal("typed username accepted")
	}
	w := request(browser, "POST", path+"/authorize", ack, 0, csrf)
	if w.Code != 200 || strings.Contains(w.Body.String(), "never-return-secret") {
		t.Fatalf("authorize: %d %s", w.Code, w.Body)
	}
	var body struct {
		URL string `json:"url"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	address, _ := url.Parse(body.URL)
	if address.Query().Get("redirect_uri") != cfg.PublicOrigin+"/auth/github/callback" ||
		address.Query().Get("code_challenge_method") != "S256" {
		t.Fatal("callback or PKCE not fixed")
	}
	callback := "/auth/github/callback?state=" + url.QueryEscape(
		address.Query().Get("state"),
	) + "&code=forged&return_to=https://evil.example.test"
	for _, token := range []string{"", otherBrowser} {
		w := request(token, "GET", callback, nil, 0, "")
		if w.Code != 303 || w.Header().Get("Location") != "/account?github=failed" {
			t.Fatal("unsafe callback redirect")
		}
	}
	if _, err := q.SaveGitHubAccount(
		ctx,
		dbgen.SaveGitHubAccountParams{
			PersonID:       person.ID,
			GithubID:       123,
			Login:          "verified-name",
			TokenEncrypted: []byte("secret-ciphertext-marker"),
		},
	); err != nil {
		t.Fatal(err)
	}
	if w := request(
		browser,
		"GET",
		path,
		nil,
		0,
		"",
	); w.Code != 200 || strings.Contains(w.Body.String(), "secret") ||
		!strings.Contains(w.Body.String(), "verified-name") {
		t.Fatalf("identity status: %d %s", w.Code, w.Body)
	}
	if w := request(
		otherBrowser,
		"GET",
		path,
		nil,
		0,
		"",
	); strings.Contains(
		w.Body.String(),
		"verified-name",
	) {
		t.Fatal("GitHub identity leaked across people")
	}
	if w := request(
		browser,
		"DELETE",
		path,
		ack,
		0,
		csrf,
	); w.Code != 200 ||
		!strings.Contains(w.Body.String(), `"account":null`) {
		t.Fatalf("unlink: %d %s", w.Code, w.Body)
	}
	if w := request(browser, "DELETE", path, ack, 0, csrf); w.Code != 409 {
		t.Fatal("stale unlink accepted")
	}
}
