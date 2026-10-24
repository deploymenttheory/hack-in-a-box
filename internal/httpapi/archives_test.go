package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestArchiveHTTPNoStoreCurrentRoleAndNativeSourceContracts(t *testing.T) {
	pool := testdb.New(t, true)
	cfg, err := config.Load(func(k string) string {
		if k == "HACKBOX_ENV" {
			return "test"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	auth, err := identity.New(t.Context(), pool, identity.Settings{Origin: cfg.PublicOrigin})
	if err != nil {
		t.Fatal(err)
	}
	owner, member, outsider := testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	)
	id := testdb.Event(t, pool, owner.ID)
	if _, err = pool.Exec(
		t.Context(),
		"UPDATE events SET stage='archived',writes_allowed=false,completed_at=now(),name='=HOSTILE-CELL' WHERE id=$1",
		id,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(
		t.Context(),
		"INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')",
		id,
		member.ID,
	); err != nil {
		t.Fatal(err)
	}
	router := NewRouter(
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Dependencies{Pool: pool, Auth: auth},
	)
	request := func(person *dbgen.Person, path string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("GET", path, nil)
		if person != nil {
			session, e := auth.Sessions.Load(t.Context(), "")
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
		if w.Code != want {
			t.Fatalf("%s: want%d got%d: %s", path, want, w.Code, w.Body)
		}
		if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("private archive response cacheable", w.Header())
		}
		return w
	}
	base := fmt.Sprintf("/api/v1/events/%s/archive", id)
	request(nil, base, 401)
	request(&outsider, base, 403)
	private := request(&owner, base, 200)
	if !strings.Contains(private.Body.String(), "recorded_attendance_people") ||
		strings.Contains(private.Body.String(), `"key":"actual_attendance"`) {
		t.Fatal("physical attendance inferred", private.Body)
	}
	participant := request(&member, base, 200)
	var body map[string]any
	if err = json.Unmarshal(participant.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["summary"] != nil || body["summary_state"] != "restricted" {
		t.Fatal("participant gained staff summary", body)
	}
	request(&owner, "/api/v1/archives?limit=1", 200)
	request(&owner, "/api/v1/archives/reports?limit=1", 200)
	exported := request(&owner, base+"/summary.csv", 200)
	if !strings.Contains(exported.Header().Get("Content-Disposition"), "attachment") ||
		!strings.Contains(exported.Body.String(), "'=HOSTILE-CELL") ||
		!strings.Contains(exported.Body.String(), "source_availability") {
		t.Fatal("CSV lacks download/formula-safe/provenance contract", exported.Body)
	}
	request(&member, base+"/summary.csv", 403)
	request(&owner, base+"/exports/objective-evidence?limit=1", 200)
	request(&owner, base+"/exports/published-outcomes?limit=1", 200)
	request(&owner, base+"/exports/assessments?limit=1", 400)
	request(&owner, base+"/exports/credentials?limit=1", 400)
	request(&owner, "/api/v1/archives?after=invalid", 400)
	if _, err = pool.Exec(
		t.Context(),
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2",
		id,
		owner.ID,
	); err != nil {
		t.Fatal(err)
	}
	request(&owner, base+"/summary.csv", 403)
}
