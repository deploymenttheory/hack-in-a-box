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
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestMentorHTTPContractPublicationRolesAndCurrentAuthority(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	owner, competitor, primary, second, admin, outsider := testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	)
	id, other := testdb.Event(t, pool, owner.ID), testdb.Event(t, pool, outsider.ID)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("UPDATE people SET platform_admin=true WHERE id=$1", admin.ID)
	c := event.DefaultConfiguration()
	c.LiveStart = time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	c.SubmissionCutoff = c.LiveEnd.Add(-time.Hour)
	data, _ := json.Marshal(c)
	exec(
		"UPDATE events SET stage='preparation',configuration=$2,team_frozen_at=now() WHERE id=$1",
		id,
		data,
	)
	team := uuid.New()
	exec(
		"INSERT INTO hack_teams(id,event_id,name,state,frozen,allocation_locked) VALUES($1,$2,'Synthetic HTTP team','published',true,true)",
		team,
		id,
	)
	for _, p := range []dbgen.Person{competitor, testdb.Person(t, pool), testdb.Person(t, pool)} {
		exec(
			"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
			id,
			p.ID,
		)
		for _, snapshot := range []string{"draft", "published"} {
			exec(
				"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,$4)",
				id,
				team,
				p.ID,
				snapshot,
			)
		}
	}
	for _, p := range []dbgen.Person{primary, second} {
		exec(
			"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mentor')",
			id,
			p.ID,
		)
	}
	if err := q.CreateCompetitionEntry(
		ctx,
		dbgen.CreateCompetitionEntryParams{EventID: id, TeamID: team},
	); err != nil {
		t.Fatal(err)
	}
	exec(
		"INSERT INTO operational_profiles(event_id,person_id,dietary,accessibility) VALUES($1,$2,'PRIVATE-DIET','PRIVATE-ACCESS')",
		id,
		competitor.ID,
	)
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
		Dependencies{Pool: pool, Auth: auth, Mentors: mentors.Service{Pool: pool}},
	)
	csrf := strings.Repeat("a", 32)
	tokens := map[uuid.UUID]string{}
	rev := func() int64 {
		t.Helper()
		r, err := q.Event(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return r.Revision
	}
	request := func(actor dbgen.Person, method, path string, body any, revision int64, goodCSRF bool) *httptest.ResponseRecorder {
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
		var reader io.Reader
		if body != nil {
			data, _ := json.Marshal(body)
			reader = bytes.NewReader(data)
		}
		r := httptest.NewRequest(method, path, reader)
		if body != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("If-Match", fmt.Sprintf("\"%d\"", revision))
		token := csrf
		if !goodCSRF {
			token = strings.Repeat("b", 32)
		}
		r.Header.Set("X-CSRF-Token", token)
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
		if w.Header().Get("Cache-Control") != "no-store" ||
			strings.Contains(w.Body.String(), "PRIVATE-") {
			t.Fatal("mentor privacy or cache boundary failed")
		}
	}
	base := "/api/v1/events/" + id.String() + "/mentor-coverage"
	check(request(owner, "GET", base, nil, rev(), true), 200)
	for _, person := range []dbgen.Person{admin, outsider} {
		check(request(person, "GET", base, nil, rev(), true), 403)
	}
	check(request(dbgen.Person{}, "GET", base, nil, rev(), true), 401)
	check(
		request(owner, "GET", "/api/v1/events/"+other.String()+"/mentor-coverage", nil, 1, true),
		403,
	)
	for _, p := range []dbgen.Person{primary, second} {
		check(
			request(
				owner,
				"POST",
				base,
				map[string]any{
					"action":        "profile",
					"reason":        "Declared availability",
					"person_id":     p.ID,
					"skills":        []string{"design"},
					"availability":  []mentors.Window{{Start: c.LiveStart, End: c.LiveEnd}},
					"maximum_teams": nil,
				},
				rev(),
				true,
			),
			200,
		)
	}
	check(
		request(
			owner,
			"POST",
			base,
			map[string]any{
				"action":       "profile",
				"reason":       "Foreign mentor",
				"person_id":    outsider.ID,
				"availability": []mentors.Window{{Start: c.LiveStart, End: c.LiveEnd}},
			},
			rev(),
			true,
		),
		400,
	)
	before := rev()
	check(request(owner, "POST", base+"/suggestions", nil, before, true), 200)
	if rev() != before {
		t.Fatal("proposal bumped revision")
	}
	for _, p := range []dbgen.Person{primary, competitor, admin, outsider} {
		check(request(p, "POST", base+"/suggestions", nil, rev(), true), 403)
	}
	check(request(owner, "POST", base+"/suggestions", nil, rev(), false), 403)
	check(
		request(
			owner,
			"POST",
			base,
			map[string]any{
				"action":    "profile",
				"reason":    "Bad private input",
				"person_id": primary.ID,
				"dietary":   "PRIVATE-DIET",
			},
			rev(),
			true,
		),
		400,
	)
	check(
		request(
			owner,
			"POST",
			base,
			map[string]any{
				"action":    "profile",
				"reason":    "Forged authority",
				"person_id": primary.ID,
				"active":    true,
			},
			rev(),
			true,
		),
		400,
	)
	for i, p := range []dbgen.Person{primary, second} {
		slot := "primary"
		if i == 1 {
			slot = "second"
		}
		check(
			request(
				owner,
				"POST",
				base,
				map[string]any{
					"action":    "assign",
					"reason":    "Reviewed assignment",
					"team_id":   team,
					"person_id": p.ID,
					"slot":      slot,
					"window":    mentors.Window{Start: c.LiveStart, End: c.LiveEnd},
				},
				rev(),
				true,
			),
			200,
		)
	}
	check(
		request(
			owner,
			"POST",
			base,
			map[string]any{
				"action":    "assign",
				"reason":    "Duplicate reviewer",
				"team_id":   team,
				"person_id": primary.ID,
				"slot":      "second",
				"window":    mentors.Window{Start: c.LiveStart, End: c.LiveEnd},
			},
			rev(),
			true,
		),
		409,
	)
	stale := rev()
	check(
		request(
			owner,
			"POST",
			base,
			map[string]any{"action": "publish", "reason": "Reviewed complete coverage"},
			stale,
			true,
		),
		200,
	)
	check(
		request(
			owner,
			"POST",
			base,
			map[string]any{"action": "publish", "reason": "Stale queued publication"},
			stale,
			true,
		),
		409,
	)
	w := request(competitor, "GET", base, nil, rev(), true)
	check(w, 200)
	var page mentors.Page
	if err = json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.CanManage || len(page.Draft) != 0 || len(page.Published) != 2 ||
		len(page.Conflicts) != 0 ||
		len(page.PublishedGaps) != 0 {
		t.Fatalf("competitor received private planning state: %+v", page)
	}
	exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mentor'",
		id,
		primary.ID,
	)
	check(request(primary, "GET", base, nil, rev(), true), 403)
	exec("UPDATE events SET writes_allowed=false,stage='cancelled' WHERE id=$1", id)
	check(
		request(
			owner,
			"POST",
			base,
			map[string]any{"action": "publish", "reason": "Cancelled queued publication"},
			rev(),
			true,
		),
		403,
	)
}
