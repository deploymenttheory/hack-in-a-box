package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/presentation"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
	"github.com/google/uuid"
)

func TestTodayHTTPCurrentReaderRolesNoStoreAndNoContextEscalation(t *testing.T) {
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
	owner, reader, offered := testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool)
	id := testdb.Event(t, pool, owner.ID)
	now := time.Date(2027, 6, 20, 10, 0, 0, 0, time.UTC)
	if err = q.SetMembership(ctx, dbgen.SetMembershipParams{EventID: id, PersonID: reader.ID, Role: "mentor", Active: true}); err != nil {
		t.Fatal(err)
	}
	if err = q.SetMembership(ctx, dbgen.SetMembershipParams{EventID: id, PersonID: reader.ID, Role: "judge", Active: true}); err != nil {
		t.Fatal(err)
	}
	if err = q.SetCompetitionState(ctx, dbgen.SetCompetitionStateParams{EventID: id, PersonID: offered.ID, State: "offered"}); err != nil {
		t.Fatal(err)
	}
	router := NewRouter(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{Pool: pool, Auth: auth, Events: event.Service{Pool: pool, Clock: func() time.Time { return now }}, Presentation: presentation.Service{Pool: pool, Clock: func() time.Time { return now }}})
	request := func(person *dbgen.Person, target uuid.UUID, query string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/events/%s/today%s", target, query), nil)
		if person != nil {
			session, e := auth.Sessions.Load(ctx, "")
			if e != nil {
				t.Fatal(e)
			}
			auth.Sessions.Put(session, "person_id", person.ID.String())
			auth.Sessions.Put(session, "session_revision", person.SessionRevision)
			auth.Sessions.Put(session, "csrf", strings.Repeat("s", 32))
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
	w := request(&reader, id, "?role=organiser")
	if w.Code != http.StatusOK {
		t.Fatalf("current reader: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("private Today response was cacheable")
	}
	var page contract.TodayPage
	if err = json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(page.Roles, []contract.StaffRole{contract.StaffRoleJudge, contract.StaffRoleMentor}) || page.Event.CanEdit || page.Event.CanReadStaff || !page.Program.ServerNow.Equal(now) {
		t.Fatalf("context request escalated current roles: %+v", page)
	}
	for _, key := range []string{`"draft":`, `"pairings":`, `"token":`, `"can_control":`} {
		if strings.Contains(w.Body.String(), key) {
			t.Fatalf("controller data in Today: %s", key)
		}
	}
	if w = request(nil, id, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated Today: %d", w.Code)
	}
	if w = request(&offered, id, ""); w.Code != http.StatusForbidden {
		t.Fatalf("offer must not grant event-reader access: %d", w.Code)
	}
	if w = request(&reader, uuid.New(), ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown event: %d %s", w.Code, w.Body.String())
	}
	if err = q.SetMembership(ctx, dbgen.SetMembershipParams{EventID: id, PersonID: reader.ID, Role: "mentor", Active: false}); err != nil {
		t.Fatal(err)
	}
	w = request(&reader, id, "")
	if err = json.Unmarshal(w.Body.Bytes(), &page); w.Code != http.StatusOK || err != nil || !reflect.DeepEqual(page.Roles, []contract.StaffRole{contract.StaffRoleJudge}) {
		t.Fatalf("revoked role retained over HTTP: %d %s", w.Code, w.Body.String())
	}
	if err = q.SetMembership(ctx, dbgen.SetMembershipParams{EventID: id, PersonID: reader.ID, Role: "judge", Active: false}); err != nil {
		t.Fatal(err)
	}
	if w = request(&reader, id, ""); w.Code != http.StatusForbidden {
		t.Fatalf("revoked current event reader: %d", w.Code)
	}
}
