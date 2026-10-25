package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/preparation"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

// Authentication is tested through actual RS256 exchange by identity's security
// fixture. Here legitimate native stored sessions exercise the canonical HTTP
// middleware and resource owners; knowing an event/file ID never supplies a grant.
func TestSecurity121CrossEventFilesExportsAndMaliciousUploads(t *testing.T) {
	pool := testdb.New(t, true)
	ctx, q := t.Context(), dbgen.New(pool)
	first, second := testdb.Person(t, pool), testdb.Person(t, pool)
	eventA, eventB := testdb.Event(t, pool, first.ID), testdb.Event(t, pool, second.ID)
	cfg, err := config.Load(func(k string) string {
		if k == "HACKBOX_ENV" {
			return "test"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	storage, err := files.OpenLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := storage.Close(); err != nil {
			t.Error(err)
		}
	})
	auth, err := identity.New(ctx, pool, identity.Settings{Origin: cfg.PublicOrigin})
	if err != nil {
		t.Fatal(err)
	}
	policy := access.EventPolicy{Queries: q}
	fileOwner := files.Service{Pool: pool, Storage: storage, Policy: policy, MaxBytes: 1024, EventQuota: 8192}
	const private = "PRIVATE-SECURITY121-OTHER-EVENT-FILE"
	other, err := fileOwner.Upload(ctx, files.Upload{Actor: second.ID, Event: eventB, Key: uuid.New(), Revision: 1, Filename: "private.txt", MediaType: "text/plain", Content: []byte(private)})
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	router := NewRouter(cfg, slog.New(slog.NewJSONHandler(&logs, nil)), Dependencies{Pool: pool, Auth: auth, Files: fileOwner, Audit: audit.Service{Queries: q, Policy: policy}, Preparation: preparation.Service{Pool: pool}})
	session, err := auth.Sessions.Load(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	const csrf = "synthetic-security121-csrf-at-least-32-bytes"
	auth.Sessions.Put(session, "person_id", first.ID.String())
	auth.Sessions.Put(session, "session_revision", first.SessionRevision)
	auth.Sessions.Put(session, "csrf", csrf)
	token, _, err := auth.Sessions.Commit(session)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(method, path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("X-CSRF-Token", csrf)
		r.Header.Set("Idempotency-Key", uuid.NewString())
		r.Header.Set("If-Match", `"1"`)
		r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") || strings.Contains(w.Body.String()+logs.String(), private) {
			t.Fatal("private object denial lost cache/redaction boundary")
		}
		return w
	}
	for _, path := range []string{
		fmt.Sprintf("/api/v1/events/%s/files/%s", eventB, other.ID),
		fmt.Sprintf("/api/v1/events/%s/files/%s", eventA, other.ID),
		fmt.Sprintf("/api/v1/events/%s/audit", eventB),
		fmt.Sprintf("/api/v1/events/%s/exports/readiness.csv?audience=organiser", eventB),
	} {
		w := request("GET", path, nil)
		if w.Code != http.StatusForbidden && w.Code != http.StatusNotFound {
			t.Fatal("cross-event reference accepted", path, w.Code)
		}
	}
	base := "/api/v1/events/" + eventA.String() + "/files"
	for name, payload := range map[string][]byte{
		"html.txt":     []byte(`<html><script>window.security121=true</script></html>`),
		"svg.png":      []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`),
		"oversize.txt": bytes.Repeat([]byte("a"), 1025),
	} {
		w := request("POST", base, contract.UploadFileJSONRequestBody{Filename: name, MediaType: contract.UploadFileJSONBodyMediaTypeImagepng, Content: payload})
		if w.Code < 400 || w.Code >= 500 {
			t.Fatal("malicious/mismatched upload did not fail safely", name, w.Code)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM files WHERE event_id=$1", eventA).Scan(&count); err != nil || count != 0 {
		t.Fatal("rejected upload left authorised metadata", err)
	}
	good := request("POST", base, contract.UploadFileJSONRequestBody{Filename: "safe.txt", MediaType: contract.UploadFileJSONBodyMediaTypeTextplain, Content: []byte("Synthetic safe positive control")})
	if good.Code != 201 {
		t.Fatal("legitimate upload positive control failed", good.Code)
	}
	var own contract.File
	if err := json.Unmarshal(good.Body.Bytes(), &own); err != nil {
		t.Fatal(err)
	}
	download := request("GET", base+"/"+own.Id.String(), nil)
	if download.Code != 200 || !strings.HasPrefix(download.Header().Get("Content-Disposition"), "attachment;") || download.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("safe download lost attachment/nosniff policy")
	}
	if _, err := pool.Exec(ctx, "UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2", eventA, first.ID); err != nil {
		t.Fatal(err)
	}
	if w := request("GET", base+"/"+own.Id.String(), nil); w.Code != 403 {
		t.Fatal("revoked event organiser retained file download", w.Code)
	}
}
