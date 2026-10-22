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
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/teams"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestAllocationHTTPContractRolesRevisionsAndPrivateInputs(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	owner, competitor, admin, outsider := testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool)
	id, other := testdb.Event(t, pool, owner.ID), testdb.Event(t, pool, outsider.ID)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("UPDATE people SET platform_admin=true WHERE id=$1", admin.ID)
	now := time.Date(2027, 6, 1, 9, 0, 0, 0, time.UTC)
	c := event.DefaultConfiguration()
	c.MinimumAttendance = 3
	c.TeamFreeze = now.Add(time.Hour)
	c.AdmissionCutoff = now
	c.LiveStart = now.AddDate(0, 0, 10)
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	c.SubmissionCutoff = c.LiveStart.Add(7 * time.Hour)
	data, _ := json.Marshal(c)
	exec("UPDATE events SET stage='preparation',configuration=$2 WHERE id=$1", id, data)
	group := uuid.New()
	exec("INSERT INTO hack_teams(id,event_id,name,captain_id,published_captain_id,state,allocation_locked) VALUES($1,$2,'Synthetic HTTP team',$3,$3,'published',true)", group, id, competitor.ID)
	people := []dbgen.Person{competitor}
	for range 5 {
		people = append(people, testdb.Person(t, pool))
	}
	for _, p := range people {
		exec("INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')", id, p.ID)
		for _, snapshot := range []string{"draft", "published"} {
			exec("INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,$4)", id, group, p.ID, snapshot)
		}
	}
	exec("INSERT INTO operational_profiles(event_id,person_id,dietary,accessibility) VALUES($1,$2,'PRIVATE-DIET','PRIVATE-ACCESS')", id, competitor.ID)
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
	router := NewRouter(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{Pool: pool, Auth: auth, Teams: teams.Service{Pool: pool, Now: func() time.Time { return now }}})
	tokens := map[uuid.UUID]string{}
	csrf := strings.Repeat("a", 32)
	revision := func() int64 {
		t.Helper()
		row, err := q.Event(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return row.Revision
	}
	request := func(actor dbgen.Person, path string, body any, rev int64, validCSRF bool) *httptest.ResponseRecorder {
		t.Helper()
		if actor.ID != uuid.Nil && tokens[actor.ID] == "" {
			session, err := auth.Sessions.Load(ctx, "")
			if err != nil {
				t.Fatal(err)
			}
			auth.Sessions.Put(session, "person_id", actor.ID.String())
			auth.Sessions.Put(session, "session_revision", actor.SessionRevision)
			auth.Sessions.Put(session, "csrf", csrf)
			token, _, err := auth.Sessions.Commit(session)
			if err != nil {
				t.Fatal(err)
			}
			tokens[actor.ID] = token
		}
		data, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", path, bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("If-Match", fmt.Sprintf("\"%d\"", rev))
		if validCSRF {
			r.Header.Set("X-CSRF-Token", csrf)
		} else {
			r.Header.Set("X-CSRF-Token", strings.Repeat("b", 32))
		}
		if actor.ID != uuid.Nil {
			r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: tokens[actor.ID]})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		return w
	}
	check := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("want%d got%d %s", status, w.Code, w.Body)
		}
		if w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "PRIVATE-") {
			t.Fatal("private input or cache boundary failed")
		}
	}
	base := "/api/v1/events/" + id.String()
	path := base + "/allocation-suggestions"
	input := map[string]any{"seed": "HTTP", "limits": map[string]any{"maximum_size": nil, "maximum_teams": nil}, "declarations": []any{}}
	before := revision()
	w := request(owner, path, input, before, true)
	check(w, 200)
	var out teams.AllocationSuggestion
	if err = json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Teams) != 1 || len(out.Teams[0].Members) != 6 || !out.Teams[0].Locked || revision() != before {
		t.Fatalf("suggestion mutated roster or lost locked group: %+v", out)
	}
	for _, person := range []dbgen.Person{competitor, admin, outsider} {
		check(request(person, path, input, before, true), 403)
	}
	check(request(dbgen.Person{}, path, input, before, true), 401)
	check(request(owner, path, input, before, false), 403)
	check(request(owner, "/api/v1/events/"+other.String()+"/allocation-suggestions", input, 1, true), 403)
	invalid := map[string]any{"seed": "bad", "limits": input["limits"], "declarations": []any{}, "dietary": "PRIVATE-DIET"}
	check(request(owner, path, invalid, before, true), 400)
	invalid = map[string]any{"seed": "bad", "limits": input["limits"], "declarations": []any{map[string]any{"id": outsider.ID, "skills": []string{}, "experience": 0, "prefer_with": []uuid.UUID{}}}}
	check(request(owner, path, invalid, before, true), 400)
	check(request(owner, base+"/teams", map[string]any{"action": "unlock", "team_id": group, "reason": "Reviewed rebalancing"}, before, true), 200)
	check(request(owner, path, input, before, true), 409)
	check(request(owner, path, input, revision(), true), 200)
	now = c.TeamFreeze
	check(request(owner, path, input, revision(), true), 409)
	check(request(owner, base+"/teams", map[string]any{"action": "unlock", "team_id": group, "reason": "Too late"}, revision(), true), 409)
}
