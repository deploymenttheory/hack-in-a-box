package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/permissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type embargoFixture struct {
	t                              *testing.T
	pool                           *pgxpool.Pool
	q                              *dbgen.Queries
	id                             uuid.UUID
	teams                          [2]uuid.UUID
	members                        [2][]dbgen.Person
	owner, mentor, judge, stranger dbgen.Person
	cfg                            config.Config
	c                              event.Configuration
	now, due                       time.Time
	service                        content.Service
	storage                        files.Storage
	auth                           *identity.Auth
	router                         http.Handler
	tokens                         map[uuid.UUID]string
	logs                           bytes.Buffer
	prefix                         string
}

func newEmbargoFixture(t *testing.T, pool *pgxpool.Pool, live time.Time) *embargoFixture {
	t.Helper()
	if err := database.Migrate(t.Context(), pool, 31); err != nil {
		t.Fatal(err)
	}
	f := &embargoFixture{t: t, pool: pool, q: dbgen.New(pool), tokens: map[uuid.UUID]string{}}
	f.owner, f.mentor, f.judge, f.stranger = testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool)
	f.id = testdb.Event(t, pool, f.owner.ID)
	f.prefix = "EMBARGO-" + f.id.String()
	f.c = event.DefaultConfiguration()
	f.c.Timezone = "Europe/London"
	f.c.MinimumAttendance = 6
	f.c.LiveStart, f.c.LiveEnd, f.c.SubmissionCutoff = live, live.Add(8*time.Hour), live.Add(7*time.Hour)
	var err error
	f.due, err = schedule.ChallengePublication(live, f.c.Timezone, f.c.DSTChoice)
	if err != nil {
		t.Fatal(err)
	}
	f.c.TeamFreeze, f.c.AdmissionCutoff = f.due.Add(-time.Hour), f.due.Add(-2*time.Hour)
	f.now = f.due.Add(-time.Minute)
	data, _ := json.Marshal(f.c)
	f.exec("UPDATE events SET name='Embargo verification',slug=$2,stage='preparation',configuration=$3,team_frozen_at=$4 WHERE id=$1", f.id, uuid.NewString(), data, f.c.TeamFreeze)
	for index := range f.teams {
		f.teams[index] = uuid.New()
		f.exec("INSERT INTO hack_teams(event_id,id,name,state,frozen) VALUES($1,$2,$3,'published',true)", f.id, f.teams[index], fmt.Sprintf("Team %d", index))
		for range 3 {
			p := testdb.Person(t, pool)
			f.members[index] = append(f.members[index], p)
			f.exec("INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')", f.id, p.ID)
			f.exec("INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')", f.id, f.teams[index], p.ID)
		}
	}
	f.exec("INSERT INTO event_gate_facts(event_id,attendance_ready,teams_ready,total_teams,frozen_teams,assigned_attendees) VALUES($1,true,true,2,2,6)", f.id)
	for role, p := range map[string]dbgen.Person{"mentor": f.mentor, "judge": f.judge} {
		f.exec("INSERT INTO event_memberships(event_id,person_id,role,active) VALUES($1,$2,$3,true)", f.id, p.ID, role)
	}
	store, err := files.OpenLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	f.storage = store
	f.service = content.Service{Pool: pool, Storage: store, Clock: func() time.Time { return f.now }}
	f.cfg, err = config.Load(func(key string) string {
		if key == "HACKBOX_ENV" {
			return "test"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	f.cfg.WebDir = t.TempDir()
	if err = os.Mkdir(filepath.Join(f.cfg.WebDir, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"index.html": "<!doctype html><html><body>Public application shell</body></html>", "assets/app.js": "export const application = true"} {
		if err = os.WriteFile(filepath.Join(f.cfg.WebDir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	f.auth, err = identity.New(t.Context(), pool, identity.Settings{Origin: f.cfg.PublicOrigin})
	if err != nil {
		t.Fatal(err)
	}
	policy := access.EventPolicy{Queries: f.q, Clock: func() time.Time { return f.now }}
	f.router = NewRouter(f.cfg, slog.New(slog.NewJSONHandler(&f.logs, nil)), Dependencies{Pool: pool, Auth: f.auth, Content: f.service, Files: files.Service{Pool: pool, Storage: store, Policy: policy, MaxBytes: 1000, EventQuota: 100000}, Audit: audit.Service{Queries: f.q, Policy: policy}, Templates: templates.Service{Pool: pool}, Permissions: permissions.Service{Pool: pool}})
	return f
}
func (f *embargoFixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.t.Context(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}
func (f *embargoFixture) row() dbgen.Event {
	f.t.Helper()
	row, err := f.q.Event(f.t.Context(), f.id)
	if err != nil {
		f.t.Fatal(err)
	}
	return row
}
func (f *embargoFixture) base() string             { return "/api/v1/events/" + f.id.String() }
func (f *embargoFixture) path(id uuid.UUID) string { return f.base() + "/content/" + id.String() }
func (f *embargoFixture) file(id uuid.UUID) string { return f.base() + "/files/" + id.String() }

// Reuse the same authenticated cookie across grant and attendance changes, so
// passing tests cannot accidentally depend on signing in again after revocation.
func (f *embargoFixture) request(actor dbgen.Person, method, path string, body any, revision int64) *httptest.ResponseRecorder {
	f.t.Helper()
	csrf := strings.Repeat("e", 32)
	if actor.ID != uuid.Nil && f.tokens[actor.ID] == "" {
		ctx, err := f.auth.Sessions.Load(f.t.Context(), "")
		if err != nil {
			f.t.Fatal(err)
		}
		f.auth.Sessions.Put(ctx, "person_id", actor.ID.String())
		f.auth.Sessions.Put(ctx, "session_revision", actor.SessionRevision)
		f.auth.Sessions.Put(ctx, "csrf", csrf)
		token, _, err := f.auth.Sessions.Commit(ctx)
		if err != nil {
			f.t.Fatal(err)
		}
		f.tokens[actor.ID] = token
	}
	data, _ := json.Marshal(body)
	r := httptest.NewRequest(method, path, bytes.NewReader(data))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", f.cfg.PublicOrigin)
	r.Header.Set("X-CSRF-Token", csrf)
	if method != "GET" && method != "HEAD" {
		r.Header.Set("If-Match", fmt.Sprintf("\"%d\"", revision))
		r.Header.Set("Idempotency-Key", uuid.NewString())
	}
	if method == "GET" && strings.Contains(path, "/files/") {
		r.Header.Set("If-None-Match", "*")
		r.Header.Set("If-Modified-Since", "Wed, 01 Jan 2031 00:00:00 GMT")
		r.Header.Set("Range", "bytes=0-8")
	}
	if actor.ID != uuid.Nil {
		r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: f.tokens[actor.ID]})
	}
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	return w
}
func (f *embargoFixture) expect(w *httptest.ResponseRecorder, status int) {
	f.t.Helper()
	if w.Code != status {
		f.t.Fatalf("want %d got %d: %s", status, w.Code, w.Body)
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		f.t.Fatalf("private response cache headers: %v", w.Header())
	}
}
func (f *embargoFixture) denied(w *httptest.ResponseRecorder, status int) {
	f.t.Helper()
	f.expect(w, status)
	if strings.Contains(w.Body.String(), f.prefix) {
		f.t.Fatal("private text, title or filename in denied response")
	}
}
func (f *embargoFixture) page(w *httptest.ResponseRecorder) content.Page {
	f.t.Helper()
	f.expect(w, 200)
	var p content.Page
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		f.t.Fatal(err)
	}
	return p
}
func (f *embargoFixture) action(p content.Page, action string) content.Page {
	f.t.Helper()
	return f.page(f.request(f.owner, "POST", f.path(p.ID)+"/actions", map[string]string{"action": action, "reason": "Synthetic verification"}, p.Revision))
}
func (f *embargoFixture) upload(label string) uuid.UUID {
	f.t.Helper()
	w := f.request(f.owner, "POST", f.base()+"/files", map[string]any{"filename": f.prefix + "-" + label + ".txt", "media_type": "text/plain", "content": []byte(f.prefix + " file " + label)}, f.row().Revision)
	f.expect(w, 201)
	var result struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		f.t.Fatal(err)
	}
	return result.ID
}
func (f *embargoFixture) makePage(kind, label string, audience access.ContentAudience) content.Page {
	f.t.Helper()
	in := content.Input{Kind: kind, Document: content.Document{Title: f.prefix + " title " + label, Markdown: f.prefix + " body " + label, Audience: audience, Attachments: []uuid.UUID{f.upload(label)}, ScheduledAt: &f.due, RequireAcknowledgement: true}}
	if kind == "challenge" {
		ref := testdb.ApprovedRubric(f.t, f.pool, f.owner.ID, f.id)
		in.Document.Rubric = &ref
	}
	p := f.page(f.request(f.owner, "PUT", f.path(uuid.New()), in, 0))
	return f.action(f.action(p, "review"), "approve")
}
func (f *embargoFixture) replace(p content.Page, label string, due *time.Time) content.Page {
	f.t.Helper()
	p.Document.Title = f.prefix + " title " + label
	p.Document.Markdown = f.prefix + " body " + label
	p.Document.Attachments = []uuid.UUID{f.upload(label)}
	p.Document.ScheduledAt = due
	return f.page(f.request(f.owner, "PUT", f.path(p.ID), content.Input{Kind: p.Kind, Document: p.Document}, p.Revision))
}

func (f *embargoFixture) publicSurfaces(p content.Page) {
	f.t.Helper()
	for _, path := range []string{f.base() + "/audit/export", f.base() + "/configuration-snapshot", "/", "/events/" + f.id.String()} {
		w := f.request(f.owner, "GET", path, nil, 0)
		f.expect(w, 200)
		if strings.Contains(w.Body.String(), f.prefix) {
			f.t.Fatal("native content leaked through export or application shell")
		}
	}
	file, err := f.q.File(f.t.Context(), dbgen.FileParams{EventID: f.id, ID: p.Document.Attachments[0]})
	if err != nil {
		f.t.Fatal(err)
	}
	for _, path := range []string{f.path(p.ID) + "/export", f.base() + "/content/export", "/assets/" + file.ObjectKey, "/assets/" + file.ID.String() + ".txt", "/uploads/" + file.ID.String() + ".txt"} {
		w := f.request(f.owner, "GET", path, nil, 0)
		if w.Code != 404 && w.Code != 400 {
			f.t.Fatalf("private asset/export path accepted %s %d", path, w.Code)
		}
		if strings.Contains(w.Body.String(), f.prefix) || w.Header().Get("Cache-Control") != "no-store" {
			f.t.Fatal("private fallback leaked or was cacheable")
		}
	}
	// Immutable caching is confined to actual application bundles, outside the
	// protected file store. A positive control prevents a deny-everything fixture.
	w := f.request(f.stranger, "GET", "/assets/app.js", nil, 0)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" || w.Body.String() != "export const application = true" {
		f.t.Fatalf("public bundle boundary: %d %v %s", w.Code, w.Header(), w.Body)
	}
}

func (f *embargoFixture) startup() {
	f.t.Helper()
	// Run the actual startup hook on a new worker/service. Stop after local
	// maintenance, before starting unrelated provider clients in this fixture.
	stop := errors.New("stop after local startup dispatch")
	worker, err := jobs.New(f.pool, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if err != nil {
		f.t.Fatal(err)
	}
	fresh := content.Service{Pool: f.pool, Storage: f.storage, Clock: func() time.Time { return f.now }}
	worker.FastMaintenance = func(ctx context.Context) error {
		if err := fresh.Maintain(ctx); err != nil {
			return err
		}
		return stop
	}
	if err = worker.Run(f.t.Context(), time.Second); !errors.Is(err, stop) {
		f.t.Fatalf("worker startup: %v", err)
	}
}

func TestContentEmbargoHTTPRoleTeamAndCacheMatrix(t *testing.T) {
	pool := testdb.New(t, true)
	zone, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		live    time.Time
		elapsed time.Duration
	}{{"spring", time.Date(2027, 3, 29, 9, 0, 0, 0, zone), 167 * time.Hour}, {"autumn", time.Date(2027, 11, 1, 9, 0, 0, 0, zone), 169 * time.Hour}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newEmbargoFixture(t, pool, tc.live)
			challenge := f.makePage("challenge", "challenge-v1", access.ContentAudience{Confirmed: true, Roles: []string{}, Teams: []uuid.UUID{}})
			stale := challenge
			challenge = f.action(f.action(f.replace(challenge, "challenge-v2", &f.due), "review"), "approve")
			mentor := f.makePage("brief", "mentor", access.ContentAudience{Roles: []string{"mentor"}, Teams: []uuid.UUID{}})
			judge := f.makePage("brief", "judge", access.ContentAudience{Roles: []string{"judge"}, Teams: []uuid.UUID{}})
			teamA := f.makePage("page", "team-a", access.ContentAudience{Roles: []string{}, Teams: []uuid.UUID{f.teams[0]}})
			teamB := f.makePage("page", "team-b", access.ContentAudience{Roles: []string{}, Teams: []uuid.UUID{f.teams[1]}})
			pages := []content.Page{challenge, mentor, judge, teamA, teamB}
			if tc.live.Sub(*challenge.Document.ScheduledAt) != tc.elapsed {
				t.Fatal("T-minus-seven lost calendar/DST semantics")
			}
			actors := []dbgen.Person{f.members[0][0], f.members[1][0], f.mentor, f.judge}
			f.now = f.due.Add(-time.Nanosecond)
			for _, actor := range actors {
				for _, query := range []string{"", "?q=" + url.QueryEscape(f.prefix)} {
					w := f.request(actor, "GET", f.base()+"/content"+query, nil, 0)
					f.expect(w, 200)
					var list content.List
					if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
						t.Fatal(err)
					}
					if len(list.Items) != 0 || list.NextCursor != nil || strings.Contains(w.Body.String(), f.prefix) {
						t.Fatal("embargoed search/list metadata leaked")
					}
				}
				for _, p := range pages {
					f.denied(f.request(actor, "GET", f.path(p.ID), nil, 0), 404)
					f.denied(f.request(actor, "GET", f.path(p.ID)+"?preview=true&version=1", nil, 0), 404)
					f.denied(f.request(actor, "GET", f.file(p.Document.Attachments[0]), nil, 0), 403)
					f.denied(f.request(actor, "POST", f.path(p.ID)+"/acknowledgements", nil, p.Version), 404)
				}
				f.denied(f.request(actor, "GET", f.base()+"/configuration-snapshot", nil, 0), 403)
				f.denied(f.request(actor, "GET", f.base()+"/audit/export", nil, 0), 403)
			}
			f.denied(f.request(dbgen.Person{}, "GET", f.file(challenge.Document.Attachments[0]), nil, 0), 401)
			f.denied(f.request(f.members[0][0], "GET", f.file(uuid.New()), nil, 0), 404)
			f.publicSurfaces(challenge)
			f.denied(f.request(f.owner, "POST", f.path(challenge.ID)+"/actions", map[string]string{"action": "release", "reason": "Synthetic manual bypass attempt"}, challenge.Revision), 409)
			if err := f.service.Dispatch(t.Context(), f.id, challenge.ID, challenge.Revision); err != nil {
				t.Fatal(err)
			}
			if f.row().PublishedAt.Valid {
				t.Fatal("early queued challenge published")
			}
			f.now = f.due
			if err := f.service.Dispatch(t.Context(), f.id, challenge.ID, stale.Revision); err != nil {
				t.Fatal(err)
			}
			if f.row().PublishedAt.Valid {
				t.Fatal("stale queued revision published a replacement")
			}
			f.startup()
			var wg sync.WaitGroup
			for range 3 {
				wg.Go(func() {
					fresh := content.Service{Pool: pool, Storage: f.storage, Clock: func() time.Time { return f.now }}
					if err := fresh.Dispatch(t.Context(), f.id, challenge.ID, challenge.Revision); err != nil {
						t.Error(err)
					}
				})
			}
			wg.Wait()
			f.startup()
			for _, p := range pages {
				var count int
				if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM content_changes WHERE event_id=$1 AND content_id=$2 AND action='released'", f.id, p.ID).Scan(&count); err != nil || count != 1 {
					t.Fatalf("duplicate or missing release %d %v", count, err)
				}
			}
			for _, team := range f.members {
				for _, actor := range team {
					p := f.page(f.request(actor, "GET", f.path(challenge.ID), nil, 0))
					if p.Version != 2 || p.Revision != 2 || p.State != "released" || p.Document.Markdown != challenge.Document.Markdown || p.ReleasedAt == nil || !p.ReleasedAt.Equal(f.due) {
						t.Fatalf("common version/time mismatch: %+v", p)
					}
					f.expect(f.request(actor, "GET", f.file(challenge.Document.Attachments[0]), nil, 0), 200)
				}
			}
			f.denied(f.request(f.members[0][0], "GET", f.file(stale.Document.Attachments[0]), nil, 0), 403)
			for _, pair := range []struct {
				actor      dbgen.Person
				own, other content.Page
			}{{f.members[0][0], teamA, teamB}, {f.members[1][0], teamB, teamA}, {f.mentor, mentor, judge}, {f.judge, judge, mentor}} {
				f.page(f.request(pair.actor, "GET", f.path(pair.own.ID), nil, 0))
				f.expect(f.request(pair.actor, "GET", f.file(pair.own.Document.Attachments[0]), nil, 0), 200)
				f.denied(f.request(pair.actor, "GET", f.path(pair.other.ID), nil, 0), 404)
				f.denied(f.request(pair.actor, "GET", f.file(pair.other.Document.Attachments[0]), nil, 0), 403)
			}
			// A blocked correction must not expose its private workflow state or replace
			// the released challenge with a misleading missed-publication diagnostic.
			correctionDue := f.now.Add(time.Minute)
			correction := f.action(f.action(f.replace(f.page(f.request(f.owner, "GET", f.path(challenge.ID)+"?preview=true", nil, 0)), "private-correction", &correctionDue), "review"), "approve")
			attachment, err := f.q.File(t.Context(), dbgen.FileParams{EventID: f.id, ID: correction.Document.Attachments[0]})
			if err != nil {
				t.Fatal(err)
			}
			if err = f.storage.Remove(attachment.ObjectKey); err != nil {
				t.Fatal(err)
			}
			f.now = correctionDue
			if err = f.service.Dispatch(t.Context(), f.id, correction.ID, correction.Revision); err != nil {
				t.Fatal(err)
			}
			released := f.page(f.request(f.members[0][0], "GET", f.path(challenge.ID), nil, 0))
			if released.Version != 2 || released.State != "released" || released.Message != "" || strings.Contains(released.Document.Markdown, "private-correction") {
				t.Fatalf("private correction metadata leaked: %+v", released)
			}
			author := f.page(f.request(f.owner, "GET", f.path(challenge.ID)+"?preview=true", nil, 0))
			if author.State != "blocked" || author.Message == "" || strings.Contains(author.Message, "Postpone") {
				t.Fatalf("correction incorrectly retracts common publication: %+v", author)
			}
			// Current-grant and attendance changes use the original sessions and copied
			// file URLs, with conditional/range headers still present on every file GET.
			change := permissions.Change{Actor: f.owner.ID, Event: f.id, Person: f.mentor.ID, Role: "mentor", Active: false, Revision: f.row().Revision}
			if _, err = (permissions.Service{Pool: pool}).Change(t.Context(), change); err != nil {
				t.Fatal(err)
			}
			f.denied(f.request(f.mentor, "GET", f.path(mentor.ID), nil, 0), 404)
			f.denied(f.request(f.mentor, "GET", f.file(mentor.Document.Attachments[0]), nil, 0), 403)
			f.exec("UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2", f.id, f.members[0][0].ID)
			f.denied(f.request(f.members[0][0], "GET", f.path(challenge.ID), nil, 0), 404)
			f.denied(f.request(f.members[0][0], "GET", f.file(challenge.Document.Attachments[0]), nil, 0), 403)
			f.action(f.page(f.request(f.owner, "GET", f.path(teamB.ID)+"?preview=true", nil, 0)), "withdraw")
			f.denied(f.request(f.members[1][0], "GET", f.path(teamB.ID), nil, 0), 404)
			f.denied(f.request(f.members[1][0], "GET", f.file(teamB.Document.Attachments[0]), nil, 0), 403)
			f.publicSurfaces(challenge)
			if strings.Contains(f.logs.String(), f.prefix) {
				t.Fatal("brief text, search term or filename leaked in request logs")
			}
		})
	}
}

func TestContentEmbargoHTTPGatesAndMissedPostponement(t *testing.T) {
	pool := testdb.New(t, true)
	zone, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	live := time.Date(2027, 3, 29, 9, 0, 0, 0, zone)
	for _, gate := range []string{"minimum", "unfrozen", "late"} {
		t.Run(gate, func(t *testing.T) {
			f := newEmbargoFixture(t, pool, live)
			p := f.makePage("challenge", "gated", access.ContentAudience{Confirmed: true, Roles: []string{}, Teams: []uuid.UUID{}})
			if gate == "minimum" {
				f.c.MinimumAttendance = 7
				data, _ := json.Marshal(f.c)
				f.exec("UPDATE events SET configuration=$2 WHERE id=$1", f.id, data)
			}
			if gate == "unfrozen" {
				f.exec("UPDATE event_gate_facts SET frozen_teams=1 WHERE event_id=$1", f.id)
			}
			f.now = f.due
			if gate == "late" {
				f.now = f.now.Add(schedule.ChallengeDispatchTolerance + time.Microsecond)
			}
			f.startup()
			if f.row().PublishedAt.Valid {
				t.Fatal("worker bypassed publication gate")
			}
			current := f.page(f.request(f.owner, "GET", f.path(p.ID)+"?preview=true", nil, 0))
			want := "blocked"
			if gate == "late" {
				want = "missed"
			}
			if current.State != want {
				t.Fatalf("gate state %s want %s", current.State, want)
			}
			for _, actor := range []dbgen.Person{f.members[0][0], f.members[1][0]} {
				f.denied(f.request(actor, "GET", f.path(p.ID), nil, 0), 404)
				f.denied(f.request(actor, "GET", f.file(p.Document.Attachments[0]), nil, 0), 403)
			}
			// Fixing attendance/freeze after a failed attempt must not silently publish.
			f.c.MinimumAttendance = 6
			data, _ := json.Marshal(f.c)
			f.exec("UPDATE events SET configuration=$2 WHERE id=$1", f.id, data)
			f.exec("UPDATE event_gate_facts SET frozen_teams=2 WHERE event_id=$1", f.id)
			f.now = f.due.Add(time.Hour)
			f.startup()
			if f.row().PublishedAt.Valid {
				t.Fatal("blocked release caught up late")
			}
			f.denied(f.request(f.owner, "POST", f.path(current.ID)+"/actions", map[string]string{"action": "release", "reason": "Synthetic manual bypass attempt"}, current.Revision), 409)
			// Only a future common schedule obtained by postponing the live event can
			// create a new approved publication, preserving a full preparation week.
			c := f.c
			c.LiveStart = c.LiveStart.AddDate(0, 0, 7)
			c.LiveEnd = c.LiveEnd.AddDate(0, 0, 7)
			c.SubmissionCutoff = c.SubmissionCutoff.AddDate(0, 0, 7)
			row := f.row()
			_, err := (event.Service{Pool: pool, Clock: func() time.Time { return f.now }}).Update(t.Context(), f.owner.ID, f.id, row.Revision, event.Input{Name: row.Name, Slug: row.Slug, Configuration: c})
			if err != nil {
				t.Fatal(err)
			}
			f.c = c
			newDue, err := schedule.ChallengePublication(c.LiveStart, c.Timezone, c.DSTChoice)
			if err != nil {
				t.Fatal(err)
			}
			next := f.action(f.action(f.replace(current, "postponed", &newDue), "review"), "approve")
			f.now = newDue
			if err = f.service.Dispatch(t.Context(), f.id, p.ID, p.Revision); err != nil {
				t.Fatal(err)
			}
			if f.row().PublishedAt.Valid {
				t.Fatal("old queued revision bypassed fresh approval")
			}
			f.startup()
			got := f.page(f.request(f.members[1][0], "GET", f.path(next.ID), nil, 0))
			if got.Version != 2 || got.ReleasedAt == nil || !got.ReleasedAt.Equal(newDue) || !got.Document.ScheduledAt.Equal(newDue) {
				t.Fatalf("postponed publication mismatch: %+v", got)
			}
		})
	}
}

func TestContentEmbargoHTTPPrivilegedGrantsAndLastingExclusion(t *testing.T) {
	pool := testdb.New(t, true)
	zone, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	f := newEmbargoFixture(t, pool, time.Date(2027, 3, 29, 9, 0, 0, 0, zone))
	p := f.makePage("challenge", "privileged", access.ContentAudience{Confirmed: true, Roles: []string{}, Teams: []uuid.UUID{}})
	for _, role := range []string{"early_brief_access", "organiser"} {
		for _, state := range []string{"confirmed", "waitlisted"} {
			f.exec("UPDATE competition_claims SET state=$3 WHERE event_id=$1 AND person_id=$2", f.id, f.members[0][0].ID, state)
			w := f.request(f.owner, "PUT", f.base()+"/staff", map[string]any{"person_id": f.members[0][0].ID, "role": role, "active": true, "acknowledge_competition_restriction": true}, f.row().Revision)
			f.denied(w, 409)
			if !strings.Contains(w.Body.String(), "withdrawal_required") {
				t.Fatal("privileged grant did not enforce active/waitlisted conflict")
			}
		}
	}
	for _, role := range []string{"early_brief_access", "organiser"} {
		person := testdb.Person(t, pool)
		w := f.request(f.owner, "PUT", f.base()+"/staff", map[string]any{"person_id": person.ID, "role": role, "active": true, "acknowledge_competition_restriction": true}, f.row().Revision)
		f.expect(w, 200)
		f.page(f.request(person, "GET", f.path(p.ID)+"?preview=true", nil, 0))
		f.expect(f.request(person, "GET", f.file(p.Document.Attachments[0]), nil, 0), 200)
		w = f.request(f.owner, "PUT", f.base()+"/staff", map[string]any{"person_id": person.ID, "role": role, "active": false, "acknowledge_competition_restriction": false}, f.row().Revision)
		f.expect(w, 200)
		f.denied(f.request(person, "GET", f.path(p.ID)+"?preview=true", nil, 0), 404)
		f.denied(f.request(person, "GET", f.file(p.Document.Attachments[0]), nil, 0), 403)
		excluded, err := f.q.CompetitionExcluded(t.Context(), dbgen.CompetitionExcludedParams{EventID: f.id, PersonID: person.ID})
		if err != nil || !excluded {
			t.Fatalf("revocation erased lasting competition exclusion: %v", err)
		}
	}
}
