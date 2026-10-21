package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestAuthenticatedContractFilesAuditAndOperations(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	cfg, err := config.Load(func(key string) string {
		if key == "HACKBOX_ENV" {
			return "test"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.PublicOrigin = "http://hack.example.test"
	cfg.MetricsToken = strings.Repeat("m", 32)
	storage, err := files.OpenLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	auth, err := identity.New(ctx, pool, identity.Settings{Origin: cfg.PublicOrigin})
	if err != nil {
		t.Fatal(err)
	}
	policy := access.EventPolicy{Queries: q}
	logs := new(bytes.Buffer)
	router := NewRouter(
		cfg,
		slog.New(slog.NewJSONHandler(logs, nil)),
		Dependencies{
			Pool:  pool,
			Auth:  auth,
			Audit: audit.Service{Queries: q, Policy: policy},
			Files: files.Service{
				Pool:       pool,
				Storage:    storage,
				Policy:     policy,
				MaxBytes:   1024,
				EventQuota: 4096,
			},
		},
	)
	person, err := q.UpsertIdentity(
		ctx,
		dbgen.UpsertIdentityParams{Issuer: "test", Subject: "admin", DisplayName: "Synthetic"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.BootstrapAdmin(ctx, person.ID); err != nil {
		t.Fatal(err)
	}
	person, err = q.Person(ctx, person.ID)
	if err != nil {
		t.Fatal(err)
	}
	event := uuid.New()
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO events(id,name) VALUES ($1,'Synthetic')",
		event,
	); err != nil {
		t.Fatal(err)
	}
	if err := q.SetMembership(
		ctx,
		dbgen.SetMembershipParams{
			EventID:  event,
			PersonID: person.ID,
			Role:     "organiser",
			Active:   true,
		},
	); err != nil {
		t.Fatal(err)
	}
	sessionCtx, err := auth.Sessions.Load(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	csrf := strings.Repeat("c", 32)
	auth.Sessions.Put(sessionCtx, "person_id", person.ID.String())
	auth.Sessions.Put(sessionCtx, "session_revision", person.SessionRevision)
	auth.Sessions.Put(sessionCtx, "csrf", csrf)
	token, _, err := auth.Sessions.Commit(sessionCtx)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, body []byte, authenticated bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("X-CSRF-Token", csrf)
		r.Header.Set("Idempotency-Key", uuid.NewString())
		r.Header.Set("If-Match", "\"1\"")
		if authenticated {
			r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: token})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		return w
	}
	if result := request("GET", "/api/v1/session", nil, false); result.Code != 401 {
		t.Fatalf("anonymous session: %d %s", result.Code, result.Body)
	}
	if result := request("GET", "/api/v1/session", nil, true); result.Code != 200 {
		t.Fatalf("session: %d %s", result.Code, result.Body)
	}
	base := "/api/v1/events/" + event.String()
	payload, _ := json.Marshal(
		contract.UploadFileJSONRequestBody{
			Filename:  "notes.txt",
			MediaType: contract.UploadFileJSONBodyMediaTypeTextplain,
			Content:   []byte("safe synthetic file"),
		},
	)
	uploaded := request("POST", base+"/files", payload, true)
	if uploaded.Code != 201 {
		t.Fatalf("upload: %d %s", uploaded.Code, uploaded.Body)
	}
	var file contract.File
	if err := json.Unmarshal(uploaded.Body.Bytes(), &file); err != nil {
		t.Fatal(err)
	}
	download := request("GET", base+"/files/"+file.Id.String(), nil, true)
	if download.Code != 200 ||
		!strings.HasPrefix(download.Header().Get("Content-Disposition"), "attachment;") ||
		download.Header().Get("X-Content-Type-Options") != "nosniff" ||
		download.Body.String() != "safe synthetic file" {
		t.Fatalf("unsafe download: %d %v %s", download.Code, download.Header(), download.Body)
	}
	if result := request(
		"GET",
		base+"/audit",
		nil,
		true,
	); result.Code != 200 ||
		!strings.Contains(result.Body.String(), "file.uploaded") {
		t.Fatalf("audit: %d %s", result.Code, result.Body)
	}
	if result := request(
		"GET",
		base+"/audit/export",
		nil,
		true,
	); result.Code != 200 ||
		result.Header().Get("Content-Disposition") == "" {
		t.Fatalf("export: %d %s", result.Code, result.Body)
	}
	if result := request("GET", base+"/audit?limit=500", nil, true); result.Code != 400 {
		t.Fatalf("unbounded page accepted: %d", result.Code)
	}
	// Current authorization is checked after login, not cached in the session.
	if _, err := pool.Exec(
		ctx,
		"UPDATE event_memberships SET active=false WHERE person_id=$1",
		person.ID,
	); err != nil {
		t.Fatal(err)
	}
	if result := request("GET", base+"/audit", nil, true); result.Code != 403 {
		t.Fatalf("revoked authority: %d %s", result.Code, result.Body)
	}
	if _, err := pool.Exec(
		ctx,
		"UPDATE people SET active=false WHERE id=$1",
		person.ID,
	); err != nil {
		t.Fatal(err)
	}
	if result := request("GET", "/api/v1/session", nil, true); result.Code != 401 {
		t.Fatal("deactivated account retained access")
	}
	// Stalled, approved work is visible without exposing event/person identifiers.
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO operations(event_id,operation_key,kind,target_id,due_at,approved) VALUES ($1,'stalled','test',$1,now()-interval '10 minutes',true)",
		event,
	); err != nil {
		t.Fatal(err)
	}
	metrics := httptest.NewRequest("GET", "/ops/metrics", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, metrics)
	if response.Code != 403 {
		t.Fatal("metrics exposed without operational authorization")
	}
	metrics.Header.Set("Authorization", "Bearer "+cfg.MetricsToken)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, metrics)
	if response.Code != 200 ||
		!strings.Contains(response.Body.String(), "hackbox_due_job_lag_seconds 6") ||
		strings.Contains(response.Body.String(), event.String()) {
		t.Fatalf("metrics: %d %s", response.Code, response.Body)
	}
	secret := "super-secret-marker"
	r := httptest.NewRequest("POST", "/missing?secret="+secret, strings.NewReader(secret))
	r.Header.Set("Authorization", "Bearer "+secret)
	r.Header.Set("Cookie", secret)
	router.ServeHTTP(httptest.NewRecorder(), r)
	if strings.Contains(logs.String(), secret) || strings.Contains(logs.String(), token) {
		t.Fatal("request credentials/payload leaked into logs")
	}
	pool.Close()
	live := request("GET", "/healthz", nil, true)
	if live.Code != 200 {
		t.Fatal("liveness depended on database/session store")
	}
	ready := request("GET", "/readyz", nil, false)
	if ready.Code != 503 {
		t.Fatalf("readiness ignored database outage: %d", ready.Code)
	}
}

func validateResponse(t *testing.T, r *http.Request, w *httptest.ResponseRecorder) {
	t.Helper()
	spec, err := contract.GetSpec()
	if err != nil {
		t.Fatal(err)
	}
	routes, err := gorillamux.NewRouter(spec)
	if err != nil {
		t.Fatal(err)
	}
	route, params, err := routes.FindRoute(r)
	if err != nil {
		t.Fatal(err)
	}
	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request:    r,
			PathParams: params,
			Route:      route,
		},
		Status: w.Code,
		Header: w.Header(),
		Body:   io.NopCloser(bytes.NewReader(w.Body.Bytes())),
	}
	if err := openapi3filter.ValidateResponse(t.Context(), input); err != nil {
		t.Fatalf("response violates contract: %v; %s", err, w.Body)
	}
}

func TestAssetsDeepLinksAndAPIBoundary(t *testing.T) {
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(directory, "index.html"),
		[]byte("<html>application</html>"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(directory, "assets", "app.js"),
		[]byte("export {}"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{WebDir: directory, MaxBodyBytes: 1024, RequestTimeout: time.Second}
	router := NewRouter(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, tc := range []struct {
		path   string
		status int
	}{{"/", 200}, {"/events/example", 200}, {"/unknown-route", 200}, {"/assets/app.js", 200}, {"/assets/missing.js", 404}, {"/api", 404}, {"/api/v1/unknown", 404}, {"/auth/unknown", 404}, {"/ops/unknown", 404}, {"/../../.env", 404}} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", tc.path, nil))
		if response.Code != tc.status {
			t.Errorf("%s = %d", tc.path, response.Code)
		}
	}
}
