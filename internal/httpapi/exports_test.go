package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/preparation"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestReadinessDownloadContracts(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	p := testdb.Person(t, pool)
	id := testdb.Event(t, pool, p.ID)
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
	session, err := auth.Sessions.Load(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	auth.Sessions.Put(session, "person_id", p.ID.String())
	auth.Sessions.Put(session, "session_revision", p.SessionRevision)
	token, _, err := auth.Sessions.Commit(session)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Dependencies{Pool: pool, Auth: auth, Preparation: preparation.Service{Pool: pool}},
	)
	for _, v := range []struct{ file, media string }{{"readiness.csv", "text/csv"}, {"run-sheet.html", "text/html"}} {
		path := "/api/v1/events/" + id.String() + "/exports/" + v.file + "?audience=organiser"
		req := httptest.NewRequest("GET", path, nil)
		req.AddCookie(&http.Cookie{Name: "hackbox_session", Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 200 || w.Header().Get("Content-Type") != v.media ||
			!strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment;") ||
			!strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatalf("download contract: %d %v %s", w.Code, w.Header(), w.Body)
		}
		validateResponse(t, req, w)
		req = httptest.NewRequest("GET", path, nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 401 {
			t.Fatalf("anonymous export: %d", w.Code)
		}
		req = httptest.NewRequest("GET", strings.Replace(path, "organiser", "participant", 1), nil)
		req.AddCookie(&http.Cookie{Name: "hackbox_session", Value: token})
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 403 {
			t.Fatalf("audience does not grant authority: %d %s", w.Code, w.Body)
		}
	}
	if _, err = pool.Exec(
		ctx,
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2",
		id,
		p.ID,
	); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(
		"GET",
		"/api/v1/events/"+id.String()+"/exports/readiness.csv?audience=organiser",
		nil,
	)
	req.AddCookie(&http.Cookie{Name: "hackbox_session", Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("revoked export authority: %d %s", w.Code, w.Body)
	}
}
