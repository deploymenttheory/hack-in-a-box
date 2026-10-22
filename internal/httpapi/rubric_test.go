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

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestRubricHTTPPrivateConfigurationAndCommonPublication(t *testing.T) {
	pool := testdb.New(t, false)
	if err := database.Migrate(t.Context(), pool, 31); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	owner, recipient, admin := testdb.Person(
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
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
		id,
		recipient.ID,
	); err != nil {
		t.Fatal(err)
	}
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
	now := time.Date(2027, 6, 13, 9, 0, 0, 0, time.UTC)
	svc := rubric.Service{Pool: pool, Clock: func() time.Time { return now }}
	var logs bytes.Buffer
	router := NewRouter(
		cfg,
		slog.New(slog.NewJSONHandler(&logs, nil)),
		Dependencies{Pool: pool, Auth: auth, Rubrics: svc},
	)
	csrf := strings.Repeat("a", 32)
	tokens := map[uuid.UUID]string{}
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

	base := "/api/v1/events/" + id.String() + "/rubric"
	check := func(w *httptest.ResponseRecorder, status int, private bool) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("want%d got%d %s", status, w.Code, w.Body)
		}
		if w.Header().Get("Cache-Control") != "no-store" ||
			(!private && strings.Contains(w.Body.String(), "PRIVATE-")) {
			t.Fatal("private rubric or cache boundary failed")
		}
	}
	for _, person := range []dbgen.Person{recipient, admin} {
		check(request(person, "GET", base+"/configuration", nil, 0, true), 403, false)
	}
	check(request(dbgen.Person{}, "GET", base+"/configuration", nil, 0, true), 401, false)
	check(request(owner, "GET", base+"/configuration", nil, 0, true), 200, true)
	definition := rubric.Default(rubric.AllTeams, id)
	definition.PrivateJudgeGuidance = "PRIVATE-JUDGE-GUIDANCE-ONLY"
	command := map[string]any{
		"action":     "save",
		"definition": definition,
		"reason":     "Synthetic private rubric review",
	}
	check(request(owner, "POST", base+"/configuration", command, 0, false), 403, false)
	check(request(owner, "POST", base+"/configuration", command, 99, true), 409, false)
	saved := request(owner, "POST", base+"/configuration", command, 0, true)
	check(saved, 200, true)
	var configuration rubric.Configuration
	if err = json.Unmarshal(saved.Body.Bytes(), &configuration); err != nil {
		t.Fatal(err)
	}
	check(request(recipient, "GET", base, nil, 0, true), 404, false)
	approved := request(
		owner,
		"POST",
		base+"/configuration",
		map[string]any{
			"action":  "approve",
			"version": configuration.Reference.Version,
			"reason":  "Synthetic exact approval",
		},
		configuration.Revision,
		true,
	)
	check(approved, 200, true)
	if err = json.Unmarshal(approved.Body.Bytes(), &configuration); err != nil {
		t.Fatal(err)
	}
	history := request(owner, "GET", base+"/history?limit=1", nil, 0, true)
	check(history, 200, true)
	var changes rubric.History
	if err = json.Unmarshal(
		history.Body.Bytes(),
		&changes,
	); err != nil || len(changes.Items) != 1 || changes.Items[0].Action != "approved" ||
		changes.NextCursor == nil {
		t.Fatal("organiser version history omitted approval/pagination")
	}
	check(request(recipient, "GET", base+"/history", nil, 0, true), 403, false)
	check(request(owner, "GET", base+"/configuration?version=1", nil, 0, true), 200, true)
	// HTTP access remains closed even though the exact private rubric is approved.
	check(request(recipient, "GET", base, nil, 0, true), 404, false)
	dates := event.DefaultConfiguration()
	dates.Timezone = "UTC"
	dates.MinimumAttendance = 1
	dates.LiveStart = now.AddDate(0, 0, 7)
	dates.LiveEnd = dates.LiveStart.Add(8 * time.Hour)
	dates.SubmissionCutoff = dates.LiveEnd.Add(-time.Hour)
	dates.TeamFreeze = now.Add(-time.Hour)
	dates.AdmissionCutoff = dates.TeamFreeze.Add(-time.Hour)
	data, _ := json.Marshal(dates)
	execute := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	execute(
		"UPDATE events SET stage='preparation',configuration=$2,team_frozen_at=$3 WHERE id=$1",
		id,
		data,
		dates.TeamFreeze,
	)
	execute(
		"INSERT INTO event_gate_facts(event_id,attendance_ready,teams_ready,total_teams,frozen_teams,assigned_attendees) VALUES($1,true,true,1,1,1)",
		id,
	)
	team := uuid.New()
	execute(
		"INSERT INTO hack_teams(event_id,id,name,state,frozen) VALUES($1,$2,'Synthetic team','published',true)",
		id,
		team,
	)
	execute(
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
		id,
		team,
		recipient.ID,
	)
	contents := content.Service{Pool: pool, Clock: func() time.Time { return now }}
	due := now
	now = due.Add(-time.Minute)
	challenge, err := contents.Save(
		ctx,
		owner.ID,
		id,
		uuid.New(),
		0,
		content.Input{
			Kind: "challenge",
			Document: content.Document{
				Rubric:      &configuration.Reference,
				Title:       "Common challenge",
				Markdown:    "Public task",
				Audience:    access.ContentAudience{Confirmed: true},
				Attachments: []uuid.UUID{},
				ScheduledAt: &due,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"review", "approve"} {
		challenge, err = contents.Action(
			ctx,
			owner.ID,
			id,
			challenge.ID,
			challenge.Revision,
			action,
			"Synthetic common publication",
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	now = due.Add(-time.Second)
	check(request(recipient, "GET", base, nil, 0, true), 404, false)
	if err = contents.Dispatch(ctx, id, challenge.ID, challenge.Revision); err != nil {
		t.Fatal(err)
	}
	check(request(recipient, "GET", base, nil, 0, true), 404, false)
	var prematurelyPublished bool
	if err = pool.QueryRow(ctx, "SELECT published_at IS NOT NULL FROM events WHERE id=$1", id).
		Scan(&prematurelyPublished); err != nil ||
		prematurelyPublished {
		t.Fatal("pre-T7 dispatch published the event")
	}

	now = due
	if err = contents.Dispatch(ctx, id, challenge.ID, challenge.Revision); err != nil {
		t.Fatal(err)
	}
	visible := request(recipient, "GET", base, nil, 0, true)
	check(visible, 200, false)
	var public rubric.PublicPage
	if err = json.Unmarshal(
		visible.Body.Bytes(),
		&public,
	); err != nil || public.Reference != configuration.Reference ||
		public.Definition.Name != definition.Name {
		t.Fatal("HTTP projection substituted approved rubric")
	}
	check(request(admin, "GET", base, nil, 0, true), 404, false)
	execute(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		id,
		recipient.ID,
	)
	check(request(recipient, "GET", base, nil, 0, true), 404, false)
	execute("DELETE FROM event_memberships WHERE event_id=$1 AND person_id=$2", id, owner.ID)
	check(request(owner, "GET", base+"/configuration", nil, 0, true), 403, false)
	if strings.Contains(logs.String(), "PRIVATE-JUDGE-GUIDANCE") {
		t.Fatal("request logging retained private judging guidance")
	}
}
