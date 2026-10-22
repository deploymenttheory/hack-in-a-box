package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/presentation"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestCurrentMCUsesIndependentAdminSessionAndLosesConsoleOnRevocation(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	owner, mc, admin := testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	q := dbgen.New(pool)
	if _, err := q.BootstrapAdmin(ctx, admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mc'),($1,$2,'photographer')",
		eventID,
		mc.ID,
	); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(func(k string) string {
		if k == "HACKBOX_ENV" {
			return "test"
		}
		if k == "HACKBOX_SURFACE" {
			return "admin"
		}
		if k == "HACKBOX_PUBLIC_ORIGIN" {
			return "http://admin.example.test"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	auth, err := identity.New(
		ctx,
		pool,
		identity.Settings{Origin: cfg.PublicOrigin, CookiePrefix: "hackbox_admin"},
	)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Dependencies{
			Pool:         pool,
			Auth:         auth,
			Events:       event.Service{Pool: pool},
			Presentation: presentation.Service{Pool: pool},
		},
	)
	cookie := func(person dbgen.Person) *http.Cookie {
		t.Helper()
		session, loadErr := auth.Sessions.Load(ctx, "")
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		auth.Sessions.Put(session, "person_id", person.ID.String())
		auth.Sessions.Put(session, "session_revision", person.SessionRevision)
		value, _, commitErr := auth.Sessions.Commit(session)
		if commitErr != nil {
			t.Fatal(commitErr)
		}
		return &http.Cookie{Name: "hackbox_admin_session", Value: value}
	}
	request := func(cookie *http.Cookie, path string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("GET", cfg.PublicOrigin+path, nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		return w
	}
	mcCookie := cookie(mc)
	if w := request(mcCookie, "/api/v1/session"); w.Code != 200 {
		t.Fatalf("ordinary MC cannot enter admin session: %d %s", w.Code, w.Body)
	}
	if w := request(mcCookie, "/api/v1/events"); w.Code != 200 {
		t.Fatalf("ordinary MC cannot discover assigned event: %d %s", w.Code, w.Body)
	}
	base := "/api/v1/events/" + eventID.String()
	readCapability := func(c *http.Cookie, want bool) {
		t.Helper()
		w := request(c, base)
		if w.Code != 200 {
			t.Fatalf("event capability: %d %s", w.Code, w.Body)
		}
		var view event.View
		if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		if view.CanPresent != want || view.CanEdit || view.CanReadStaff {
			t.Fatalf(
				"unexpected MC/admin-only capabilities: present=%t edit=%t staff=%t",
				view.CanPresent,
				view.CanEdit,
				view.CanReadStaff,
			)
		}
	}
	readCapability(mcCookie, true)
	if w := request(mcCookie, base+"/presentation"); w.Code != 200 {
		t.Fatalf("current MC cannot open console: %d %s", w.Code, w.Body)
	}
	foreignCookie := *mcCookie
	foreignCookie.Name = "hackbox_public_session"
	if w := request(&foreignCookie, base+"/presentation"); w.Code != 401 {
		t.Fatalf("public cookie crossed admin session boundary: %d", w.Code)
	}
	admin, err = q.Person(ctx, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	adminCookie := cookie(admin)
	readCapability(adminCookie, false)
	if w := request(adminCookie, base+"/presentation"); w.Code != 403 {
		t.Fatalf("platform administrator acquired implicit console authority: %d", w.Code)
	}
	if _, err := pool.Exec(
		ctx,
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mc'",
		eventID,
		mc.ID,
	); err != nil {
		t.Fatal(err)
	}
	readCapability(mcCookie, false)
	if w := request(mcCookie, base+"/presentation"); w.Code != 403 {
		t.Fatalf("revoked MC retained console through existing session: %d", w.Code)
	}
}
