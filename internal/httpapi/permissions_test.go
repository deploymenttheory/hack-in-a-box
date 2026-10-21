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
	"github.com/deploymenttheory/hack-in-a-box/internal/permissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestStaffHTTPContractAndAuthority(t *testing.T) {
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
	auth, err := identity.New(ctx, pool, identity.Settings{Origin: cfg.PublicOrigin})
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Dependencies{Pool: pool, Auth: auth, Permissions: permissions.Service{Pool: pool}},
	)
	owner := testdb.Person(t, pool)
	member := testdb.Person(t, pool)
	event := testdb.Event(t, pool, owner.ID)
	other := testdb.Event(t, pool, member.ID)
	request := func(person dbgen.Person, method string, event uuid.UUID, suffix string, body any) *httptest.ResponseRecorder {
		session, err := auth.Sessions.Load(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		csrf := strings.Repeat("s", 32)
		auth.Sessions.Put(session, "person_id", person.ID.String())
		auth.Sessions.Put(session, "session_revision", person.SessionRevision)
		auth.Sessions.Put(session, "csrf", csrf)
		token, _, err := auth.Sessions.Commit(session)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(
			method,
			"/api/v1/events/"+event.String()+suffix,
			bytes.NewReader(data),
		)
		current, err := q.Event(ctx, event)
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("If-Match", fmt.Sprintf("\"%d\"", current.Revision))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("X-CSRF-Token", csrf)
		r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		return w
	}
	if w := request(owner, "GET", event, "/staff", nil); w.Code != 200 {
		t.Fatalf("staff list: %d %s", w.Code, w.Body)
	}
	if w := request(owner, "GET", other, "/staff", nil); w.Code != 403 {
		t.Fatalf("cross-event staff: %d", w.Code)
	}
	body := map[string]any{
		"person_id":                           member.ID,
		"role":                                "organiser",
		"active":                              true,
		"acknowledge_competition_restriction": true,
	}
	if w := request(owner, "PUT", event, "/staff", body); w.Code != 200 {
		t.Fatalf("grant: %d %s", w.Code, w.Body)
	}
	body["person_id"] = owner.ID
	body["active"] = false
	if w := request(
		owner,
		"PUT",
		event,
		"/staff",
		body,
	); w.Code != 409 ||
		!strings.Contains(w.Body.String(), "last_organiser") {
		t.Fatalf("owner removal: %d %s", w.Code, w.Body)
	}
	if w := request(
		owner,
		"PUT",
		event,
		"/owner",
		map[string]any{"person_id": member.ID, "acknowledge_competition_restriction": true},
	); w.Code != 200 {
		t.Fatalf("transfer: %d %s", w.Code, w.Body)
	}
	if w := request(member, "PUT", event, "/staff", body); w.Code != 200 {
		t.Fatalf("revoke former owner: %d %s", w.Code, w.Body)
	}
	if w := request(owner, "GET", event, "/staff", nil); w.Code != 403 {
		t.Fatal("revoked grant persisted")
	}
}
