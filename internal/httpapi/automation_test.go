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

	"github.com/deploymenttheory/hack-in-a-box/internal/automation"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestAutomationHTTPCurrentAuthorityAndReviewControls(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	owner, outsider, admin := testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	if _, err := pool.Exec(
		ctx,
		"UPDATE people SET platform_admin=true WHERE id=$1",
		admin.ID,
	); err != nil {
		t.Fatal(err)
	}
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
	service := &automation.Service{Pool: pool, Composer: &communications.Service{Pool: pool}}
	router := NewRouter(
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Dependencies{Pool: pool, Auth: auth, Automation: service},
	)
	csrf := strings.Repeat("a", 32)
	request := func(person dbgen.Person, method, path string, body any, rev int64, goodCSRF bool) *httptest.ResponseRecorder {
		t.Helper()
		var reader io.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			reader = bytes.NewReader(raw)
		}
		r := httptest.NewRequest(method, path, reader)
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("If-Match", fmt.Sprintf("\"%d\"", rev))
		if goodCSRF {
			r.Header.Set("X-CSRF-Token", csrf)
		} else {
			r.Header.Set("X-CSRF-Token", strings.Repeat("b", 32))
		}
		if person.ID != uuid.Nil {
			session, e := auth.Sessions.Load(ctx, "")
			if e != nil {
				t.Fatal(e)
			}
			auth.Sessions.Put(session, "person_id", person.ID.String())
			auth.Sessions.Put(session, "session_revision", person.SessionRevision)
			auth.Sessions.Put(session, "csrf", csrf)
			token, _, e := auth.Sessions.Commit(session)
			if e != nil {
				t.Fatal(e)
			}
			r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: token})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		if strings.Contains(w.Body.String(), "PRIVATE-") ||
			strings.Contains(w.Body.String(), "ciphertext") ||
			strings.Contains(w.Body.String(), "key_id") ||
			strings.Contains(w.Body.String(), "powerplatform.com") {
			t.Fatalf("secret escaped HTTP projection: %s", w.Body)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("secret administration response must not be cached")
		}
		return w
	}
	check := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("want %d got %d: %s", status, w.Code, w.Body)
		}
	}
	base := "/api/v1/events/" + eventID.String() + "/communications/automation"
	rev := func() int64 {
		t.Helper()
		e, err := dbgen.New(pool).Event(ctx, eventID)
		if err != nil {
			t.Fatal(err)
		}
		return e.Revision
	}
	check(request(owner, "GET", base, nil, rev(), true), 200)
	for _, person := range []dbgen.Person{outsider, admin} {
		check(request(person, "GET", base, nil, rev(), true), 403)
	}
	check(request(dbgen.Person{}, "GET", base, nil, rev(), true), 401)
	body := map[string]any{"action": "pause", "reason": "Explicit synthetic pause"}
	check(request(owner, "POST", base, body, rev(), false), 403)
	check(request(outsider, "POST", base, body, rev(), true), 403)
	check(
		request(
			owner,
			"GET",
			base+"?limit=1&bindings_after="+uuid.NewString()+"&occurrences_after="+uuid.NewString(),
			nil,
			rev(),
			true,
		),
		200,
	)
	for _, query := range []string{"?limit=0", "?limit=201", "?bindings_after=bad", "?occurrences_after=bad"} {
		check(request(owner, "GET", base+query, nil, rev(), true), 400)
	}
	previousRevision := rev()
	result := request(owner, "POST", base, body, previousRevision, true)
	check(result, 200)
	check(request(owner, "POST", base, body, previousRevision, true), 409)
	if !strings.Contains(result.Body.String(), `"paused":true`) {
		t.Fatal("pause decision missing")
	}
	bad := map[string]any{
		"action":        "pause",
		"reason":        "Cannot choose arbitrary recipients",
		"recipient_ids": []string{outsider.ID.String()},
	}
	check(request(owner, "POST", base, bad, rev(), true), 400)
	otherID := testdb.Event(t, pool, outsider.ID)
	check(
		request(
			owner,
			"GET",
			"/api/v1/events/"+otherID.String()+"/communications/automation",
			nil,
			rev(),
			true,
		),
		403,
	)
	if _, err := pool.Exec(
		ctx,
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2",
		eventID,
		owner.ID,
	); err != nil {
		t.Fatal(err)
	}
	check(request(owner, "GET", base, nil, rev(), true), 403)
}
