package httpapi

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubwebhooks"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestGitHubWebhookRawMachineAuthenticationAndStatusPolicy(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	owner := testdb.Person(t, pool)
	outsider := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	teamID := uuid.New()
	q := dbgen.New(pool)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	keys, err := secretbox.Parse(
		`{"active":"test","keys":{"test":"` + base64.StdEncoding.EncodeToString(
			bytes.Repeat([]byte{8}, 32),
		) + `"}}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	const secret = "SYNTHETIC-WEBHOOK-SECRET-PRIVATE-CANARY"
	app := githubapp.Service{Pool: pool, Keys: keys, Provider: &appHTTPProvider{code: "healthy"}}
	event, err := q.Event(ctx, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if err = app.Configure(
		ctx,
		owner.ID,
		eventID,
		event.Revision,
		githubapp.Command{
			Action: "configure",
			Configuration: githubapp.Configuration{
				AppID:              42,
				AppSlug:            "synthetic-app",
				OrganizationID:     99,
				Organization:       "synthetic-org",
				TemplateRepository: "synthetic-template",
			},
			PrivateKey:    "SYNTHETIC-PRIVATE-KEY",
			WebhookSecret: secret,
		},
	); err != nil {
		t.Fatal(err)
	}
	exec("UPDATE github_app_integrations SET installation_id=7 WHERE event_id=$1", eventID)
	exec("UPDATE events SET published_at=now()-interval '1 hour' WHERE id=$1", eventID)
	exec(
		"INSERT INTO hack_teams(event_id,id,name,state) VALUES($1,$2,'Synthetic team','published')",
		eventID,
		teamID,
	)
	exec(
		"INSERT INTO team_repositories(event_id,team_id,organization_id,organization,name,app_id,installation_id,template_repository,github_repository_id,state,requested_by) VALUES($1,$2,99,'synthetic-org','synthetic-repo',42,7,'synthetic-template',123,'ready',$3)",
		eventID,
		teamID,
		owner.ID,
	)
	webhooks := &githubwebhooks.Service{Pool: pool, Keys: keys}
	outbox, err := jobs.New(
		pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{githubwebhooks.HandlerKind: webhooks},
	)
	if err != nil {
		t.Fatal(err)
	}
	webhooks.Jobs = outbox
	cfg, err := config.Load(func(name string) string {
		if name == "HACKBOX_ENV" {
			return "test"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxBodyBytes = 2 * githubwebhooks.MaxBodyBytes
	auth, err := identity.New(ctx, pool, identity.Settings{Origin: cfg.PublicOrigin})
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	router := NewRouter(
		cfg,
		slog.New(slog.NewTextHandler(&logs, nil)),
		Dependencies{Pool: pool, Auth: auth, GitHubApp: app, GitHubWebhooks: webhooks},
	)
	path := "/api/v1/integrations/github/webhook"
	raw := []byte(
		fmt.Sprintf(
			"{\n  \"installation\":{\"id\":7}, \"repository\":{\"id\":123,\"owner\":{\"id\":99},\"pushed_at\":%d},\n \"ref\":\"refs/heads/main\",\"after\":\"%s\",\"private_prose\":\"PRIVATE-UNICODE-CANARY-🌍\"\n}",
			time.Now().Add(-time.Minute).Unix(),
			strings.Repeat("a", 40),
		),
	)
	signature := func(raw []byte) string {
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(raw)
		return "sha256=" + hex.EncodeToString(mac.Sum(nil))
	}
	request := func(body []byte, sig, media string, duplicate bool) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("POST", path, bytes.NewReader(body))
		r.Header.Set("Content-Type", media)
		r.Header.Set("X-GitHub-Delivery", uuid.NewString())
		r.Header.Set("X-GitHub-Event", "push")
		r.Header.Set("X-Hub-Signature-256", sig)
		if duplicate {
			r.Header.Add("X-GitHub-Event", "member")
		}
		r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: "INVALID-BROWSER-SESSION"})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		return w
	}
	if w := request(raw, signature(raw), "application/json", false); w.Code != 202 {
		t.Fatalf(
			"signed raw machine delivery required browser auth/normalised JSON: %d %s",
			w.Code,
			w.Body,
		)
	}
	if w := request(
		append(append([]byte{}, raw...), '\n'),
		signature(raw),
		"application/json",
		false,
	); w.Code != 403 {
		t.Fatalf("changed raw bytes: %d %s", w.Code, w.Body)
	}
	if w := request(raw, "sha1=unsafe", "application/json", false); w.Code != 403 {
		t.Fatalf("legacy signature: %d", w.Code)
	}
	if w := request(
		raw,
		signature(raw),
		"application/x-www-form-urlencoded",
		false,
	); w.Code != 415 {
		t.Fatalf("unsupported encoding: %d", w.Code)
	}
	if w := request(raw, signature(raw), "application/json", true); w.Code != 400 {
		t.Fatalf("ambiguous headers: %d", w.Code)
	}
	if w := request(
		bytes.Repeat([]byte(" "), int(githubwebhooks.MaxBodyBytes)+1),
		signature(raw),
		"application/json",
		false,
	); w.Code != 413 {
		t.Fatalf("unbounded body: %d", w.Code)
	}
	statusPath := "/api/v1/events/" + eventID.String() + "/github-activity"
	read := func(person *dbgen.Person) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("GET", statusPath, nil)
		if person != nil {
			session, e := auth.Sessions.Load(ctx, "")
			if e != nil {
				t.Fatal(e)
			}
			auth.Sessions.Put(session, "person_id", person.ID.String())
			auth.Sessions.Put(session, "session_revision", person.SessionRevision)
			token, _, e := auth.Sessions.Commit(session)
			if e != nil {
				t.Fatal(e)
			}
			r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: token})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		return w
	}
	if w := read(nil); w.Code != 401 {
		t.Fatalf("anonymous activity: %d", w.Code)
	}
	if w := read(&outsider); w.Code != 403 {
		t.Fatalf("unrelated activity: %d", w.Code)
	}
	w := read(&owner)
	if w.Code != 200 {
		t.Fatalf("organiser activity: %d %s", w.Code, w.Body)
	}
	var status githubwebhooks.Status
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Deliveries) != 1 || status.Deliveries[0].State != "pending" {
		t.Fatalf("missing bounded queue status: %+v", status)
	}
	op := status.Deliveries[0].OperationID
	if op == nil {
		t.Fatal("accepted without durable work")
	}
	if err := outbox.Execute(ctx, *op); err != nil {
		t.Fatal(err)
	}
	if w := read(
		&owner,
	); w.Code != 200 ||
		!strings.Contains(w.Body.String(), `"state":"delivered"`) {
		t.Fatalf("retry status did not advance: %d %s", w.Code, w.Body)
	}
	for _, private := range []string{secret, "PRIVATE-UNICODE-CANARY", "X-Hub-Signature", signature(raw)} {
		if strings.Contains(logs.String(), private) || strings.Contains(w.Body.String(), private) {
			t.Fatal("private webhook data escaped")
		}
	}
}
