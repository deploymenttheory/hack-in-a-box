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

	"github.com/deploymenttheory/hack-in-a-box/internal/attendance"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/observations"
	"github.com/deploymenttheory/hack-in-a-box/internal/presentation"
	"github.com/deploymenttheory/hack-in-a-box/internal/teams"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestMentorObservationHTTPPrivacyAndCurrentSessionAuthority(t *testing.T) {
	ctx := t.Context()
	pool := testdb.New(t, true)
	owner, mentor, member := testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool)
	id, team := testdb.Event(t, pool, owner.ID), uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	c := event.DefaultConfiguration()
	c.LiveStart = time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	c.SubmissionCutoff = c.LiveEnd.Add(-time.Hour)
	data, _ := json.Marshal(c)
	exec("UPDATE events SET configuration=$2,published_at=$3,stage='live' WHERE id=$1", id, data, c.LiveStart.Add(-7*24*time.Hour))
	exec("INSERT INTO hack_teams(event_id,id,name,state) VALUES($1,$2,'HTTP private evidence team','published')", id, team)
	exec("INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')", id, member.ID)
	exec("INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')", id, team, member.ID)
	exec("INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mentor')", id, mentor.ID)
	exec("INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at) VALUES($1,$2,$3,'primary','published',$4,$5)", id, team, mentor.ID, c.LiveStart, c.LiveEnd)
	exec("INSERT INTO event_gate_facts(event_id) VALUES($1)", id)
	logistics := testdb.Person(t, pool)
	exec("INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'logistics')", id, logistics.ID)
	mc := testdb.Person(t, pool)
	exec("INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mc')", id, mc.ID)
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
	now := c.LiveStart.Add(-time.Nanosecond)
	var logs bytes.Buffer
	room := presentation.Service{Pool: pool}
	composer := communications.Service{Pool: pool}
	router := NewRouter(cfg, slog.New(slog.NewTextHandler(&logs, nil)), Dependencies{
		Pool: pool, Auth: auth, Observations: observations.Service{Pool: pool, Now: func() time.Time { return now }},
		Teams: teams.Service{Pool: pool}, Attendance: attendance.Service{Pool: pool}, Presentation: room, Communications: composer,
	})
	const csrf = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	tokens := map[uuid.UUID]string{}
	revision := func() int64 {
		t.Helper()
		row, readErr := dbgen.New(pool).Event(ctx, id)
		if readErr != nil {
			t.Fatal(readErr)
		}
		return row.Revision
	}
	request := func(person dbgen.Person, method, path string, body any, rev int64, goodCSRF bool) *httptest.ResponseRecorder {
		t.Helper()
		if person.ID != uuid.Nil && tokens[person.ID] == "" {
			session, loadErr := auth.Sessions.Load(ctx, "")
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			auth.Sessions.Put(session, "person_id", person.ID.String())
			auth.Sessions.Put(session, "session_revision", person.SessionRevision)
			auth.Sessions.Put(session, "csrf", csrf)
			token, _, commitErr := auth.Sessions.Commit(session)
			if commitErr != nil {
				t.Fatal(commitErr)
			}
			tokens[person.ID] = token
		}
		var reader io.Reader
		if body != nil {
			encoded, _ := json.Marshal(body)
			reader = bytes.NewReader(encoded)
		}
		r := httptest.NewRequest(method, path, reader)
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("If-Match", fmt.Sprintf("\"%d\"", rev))
		if body != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		if goodCSRF {
			r.Header.Set("X-CSRF-Token", csrf)
		} else {
			r.Header.Set("X-CSRF-Token", strings.Repeat("b", 32))
		}
		if person.ID != uuid.Nil {
			r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: tokens[person.ID]})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		return w
	}
	check := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("HTTP status=%d want=%d body=%s", w.Code, status, w.Body.String())
		}
		if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("private response permits caching")
		}
	}
	path := fmt.Sprintf("/api/v1/events/%s/teams/%s/mentor-observations", id, team)
	note := observations.Command{Action: "observation", ID: uuid.New(), Evidence: observations.Evidence{Category: "technical", Evidence: "PRIVATE_SYNTHETIC_MENTOR_EVIDENCE", Outcome: "Private synthetic outcome"}}
	noteBody := map[string]any{"action": note.Action, "id": note.ID, "version": 0, "evidence": note.Evidence}
	check(request(dbgen.Person{}, http.MethodGet, path, nil, revision(), true), http.StatusUnauthorized)
	check(request(member, http.MethodGet, path, nil, revision(), true), http.StatusForbidden)
	check(request(mc, http.MethodGet, path, nil, revision(), true), http.StatusForbidden)
	check(request(mentor, http.MethodPost, path, noteBody, revision(), false), http.StatusForbidden)
	check(request(mentor, http.MethodPost, path, noteBody, revision(), true), http.StatusConflict)
	now = c.LiveStart
	stale := revision()
	w := request(mentor, http.MethodPost, path, noteBody, stale, true)
	check(w, http.StatusOK)
	if !strings.Contains(w.Body.String(), note.Evidence.Evidence) {
		t.Fatal("author's valid private response lost evidence")
	}
	check(request(mentor, http.MethodPost, path, noteBody, stale, true), http.StatusConflict)
	check(request(mentor, http.MethodGet, path+"/history?observation_id="+note.ID.String(), nil, revision(), true), http.StatusOK)
	check(request(owner, http.MethodGet, path, nil, revision(), true), http.StatusOK)
	privateFree := func(w *httptest.ResponseRecorder, required string) {
		t.Helper()
		check(w, http.StatusOK)
		body := w.Body.String()
		if !strings.Contains(body, required) {
			t.Fatalf("privacy check did not exercise a populated projection: %s", body)
		}
		if strings.Contains(body, note.Evidence.Evidence) || strings.Contains(body, note.Evidence.Outcome) || strings.Contains(body, "feedback_reference") {
			t.Fatal("private mentor evidence entered another projection")
		}
	}
	base := "/api/v1/events/" + id.String()
	privateFree(request(member, http.MethodGet, base+"/teams", nil, revision(), true), team.String())
	privateFree(request(logistics, http.MethodGet, base+"/logistics", nil, revision(), true), member.ID.String())
	privateFree(request(logistics, http.MethodGet, base+"/logistics/export", nil, revision(), true), member.ID.String())
	program, err := room.Read(ctx, owner.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	firstCue := 0
	for _, command := range []presentation.Command{
		{Action: "save", Reason: "Synthetic approved public cue", Cues: []presentation.Cue{{ID: uuid.New(), Title: "Approved public room cue", DurationSeconds: 60}}},
		{Action: "activate", FromDraft: true, Index: &firstCue, Reason: "Explicitly approve synthetic room program"},
	} {
		program, err = room.Change(ctx, owner.ID, id, program.Revision, command)
		if err != nil {
			t.Fatal(err)
		}
	}
	privateFree(request(mc, http.MethodGet, base+"/presentation", nil, revision(), true), "Approved public room cue")
	paired, err := room.Pair(ctx, owner.ID, id, program.Revision, presentation.PairInput{Label: "Synthetic private-note exclusion test", Minutes: 60, Reason: "Synthetic verification"})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, base+"/display/snapshot", nil)
	r.Header.Set("Authorization", "Bearer "+paired.Token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, r)
	validateResponse(t, r, w)
	privateFree(w, "Approved public room cue")
	template := communications.Template{ID: uuid.New(), Name: "Synthetic public reminder", Purpose: "reminder", Format: "text", Subject: "Event reminder", Body: "Hello {{person.name}}"}
	audience := communications.Audience{ID: uuid.New(), Name: "Current mentors", Roles: []string{"mentor"}, Teams: []uuid.UUID{}, RegistrationStates: []string{}}
	for _, command := range []communications.Command{{Action: "save_template", Template: &template}, {Action: "save_audience", Audience: &audience}} {
		command.Reason = "Synthetic projection privacy verification"
		if _, err = composer.Change(ctx, owner.ID, id, revision(), command); err != nil {
			t.Fatal(err)
		}
	}
	privateFree(request(owner, http.MethodPost, base+"/communications", map[string]any{"action": "preview", "template_id": template.ID, "audience_id": audience.ID, "reason": "Synthetic actual notification preview"}, revision(), true), mentor.ID.String())
	invalid := map[string]any{"action": "status", "version": 0, "state": "on_track", "reason": "Synthetic status", "score": 10}
	check(request(mentor, http.MethodPost, path, invalid, revision(), true), http.StatusBadRequest)
	// Reuse the same authenticated cookie after assignment/role revocation.
	exec("UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mentor'", id, mentor.ID)
	for _, target := range []string{path, path + "/history?observation_id=" + note.ID.String()} {
		w = request(mentor, http.MethodGet, target, nil, revision(), true)
		check(w, http.StatusForbidden)
		if strings.Contains(w.Body.String(), note.Evidence.Evidence) {
			t.Fatal("revoked session retained private evidence")
		}
	}
	if strings.Contains(logs.String(), note.Evidence.Evidence) || strings.Contains(logs.String(), note.Evidence.Outcome) {
		t.Fatal("private note prose entered request logs")
	}
}
