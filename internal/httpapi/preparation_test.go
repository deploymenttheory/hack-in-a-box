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
	"github.com/deploymenttheory/hack-in-a-box/internal/preparation"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestPreparationHTTPContractsAndCurrentAuthority(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
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
	router := NewRouter(
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Dependencies{Pool: pool, Auth: auth, Preparation: preparation.Service{Pool: pool}},
	)
	owner, outsider := testdb.Person(t, pool), testdb.Person(t, pool)
	id := testdb.Event(t, pool, owner.ID)
	request := func(person dbgen.Person, method, path string, body any, revision int64, csrfOK bool) *httptest.ResponseRecorder {
		session, err := auth.Sessions.Load(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		csrf := strings.Repeat("p", 32)
		auth.Sessions.Put(session, "person_id", person.ID.String())
		auth.Sessions.Put(session, "session_revision", person.SessionRevision)
		auth.Sessions.Put(session, "csrf", csrf)
		token, _, err := auth.Sessions.Commit(session)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", cfg.PublicOrigin)
		if csrfOK {
			r.Header.Set("X-CSRF-Token", csrf)
		} else {
			r.Header.Set("X-CSRF-Token", strings.Repeat("x", 32))
		}
		r.Header.Set("If-Match", fmt.Sprintf("\"%d\"", revision))
		r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		return w
	}
	base := "/api/v1/events/" + id.String() + "/preparation/tasks"
	w := request(owner, "GET", base, nil, 1, true)
	if w.Code != 200 {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}
	var page preparation.Page
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	u := preparation.Update{
		Definition: preparation.Definition{
			ID:        uuid.New(),
			Title:     "Venue check",
			OwnerRole: "organiser",
			Status:    "not_started",
			DependsOn: []uuid.UUID{},
			Checklist: []preparation.ChecklistItem{},
			Evidence:  []preparation.Evidence{},
		},
	}
	path := base + "/" + u.Definition.ID.String()
	if w := request(owner, "PUT", path, u, page.Revision, false); w.Code != 403 {
		t.Fatalf("CSRF: %d %s", w.Code, w.Body)
	}
	if w := request(outsider, "PUT", path, u, page.Revision, true); w.Code != 403 {
		t.Fatalf("write access: %d %s", w.Code, w.Body)
	}
	w = request(owner, "PUT", path, u, page.Revision, true)
	if w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	if w := request(owner, "PUT", path, u, page.Revision, true); w.Code != 409 {
		t.Fatalf("stale save: %d %s", w.Code, w.Body)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if w := request(
		owner,
		"GET",
		base+"?unowned=true&status=not_started",
		nil,
		1,
		true,
	); w.Code != 200 ||
		!strings.Contains(w.Body.String(), "Venue check") {
		t.Fatalf("filter: %d %s", w.Code, w.Body)
	}
	u.Definition.Status = "waived"
	u.Reason = "No external venue needed"
	if w := request(owner, "PUT", path, u, page.Revision, true); w.Code != 200 {
		t.Fatalf("waiver: %d %s", w.Code, w.Body)
	}
	if w := request(
		owner,
		"GET",
		path+"/history",
		nil,
		1,
		true,
	); w.Code != 200 ||
		!strings.Contains(w.Body.String(), u.Reason) {
		t.Fatalf("history: %d %s", w.Code, w.Body)
	}
	if w := request(outsider, "GET", base, nil, 1, true); w.Code != 403 {
		t.Fatal("tasks disclosed")
	}
	if err := dbgen.New(pool).
		SetMembership(ctx, dbgen.SetMembershipParams{EventID: id, PersonID: owner.ID, Role: "organiser", Active: false}); err != nil {
		t.Fatal(err)
	}
	if w := request(owner, "GET", base, nil, 1, true); w.Code != 403 {
		t.Fatal("revoked authority retained access")
	}
}
