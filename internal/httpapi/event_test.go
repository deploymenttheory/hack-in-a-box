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
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestEventHTTPContractsAndActionableBlockers(t *testing.T) {
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
	owner := testdb.Person(t, pool)
	q.BootstrapAdmin(ctx, owner.ID)
	owner, _ = q.Person(ctx, owner.ID)
	router := NewRouter(
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Dependencies{Pool: pool, Auth: auth, Events: event.Service{Pool: pool}},
	)
	session, err := auth.Sessions.Load(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	csrf := strings.Repeat("s", 32)
	auth.Sessions.Put(session, "person_id", owner.ID.String())
	auth.Sessions.Put(session, "session_revision", owner.SessionRevision)
	auth.Sessions.Put(session, "csrf", csrf)
	token, _, err := auth.Sessions.Commit(session)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, body any, revision int64) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("X-CSRF-Token", csrf)
		r.Header.Set("If-Match", fmt.Sprintf("\"%d\"", revision))
		r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		return w
	}
	c := event.DefaultConfiguration()
	c.LiveStart = time.Now().UTC().AddDate(0, 2, 0).Truncate(time.Second)
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	c.SubmissionCutoff = c.LiveEnd
	c.TeamFreeze = c.LiveStart.AddDate(0, 0, -8)
	c.AdmissionCutoff = c.TeamFreeze
	item := event.AgendaItem{
		ID:       uuid.New(),
		Title:    "Welcome",
		Audience: []string{"all"},
		Anchor:   "live_start",
	}
	c.Schedule = []event.AgendaItem{item}
	input := event.Input{
		ID:                                uuid.New(),
		Name:                              "HTTP contract event",
		Slug:                              "http-" + uuid.NewString(),
		Configuration:                     c,
		AcknowledgeCompetitionRestriction: true,
	}
	if w := request("POST", "/api/v1/event-preview", input, 1); w.Code != 200 {
		t.Fatalf("preview: %d %s", w.Code, w.Body)
	}
	if w := request("POST", "/api/v1/events", input, 1); w.Code != 200 {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	path := "/api/v1/events/" + input.ID.String()
	for _, suffix := range []string{"", "/schedule", "/decisions"} {
		if w := request("GET", path+suffix, nil, 1); w.Code != 200 {
			t.Fatalf("read %s: %d %s", suffix, w.Code, w.Body)
		}
	}
	if w := request("GET", "/api/v1/events?limit=1", nil, 1); w.Code != 200 {
		t.Fatalf("directory: %d %s", w.Code, w.Body)
	}
	if w := request("POST", path+"/schedule/"+item.ID.String()+"/approve", nil, 1); w.Code != 200 {
		t.Fatalf("schedule approval: %d %s", w.Code, w.Body)
	}
	transition := map[string]any{
		"stage":                "registration",
		"reason":               "",
		"attendance_exception": false,
	}
	if w := request("POST", path+"/transition", transition, 1); w.Code != 200 {
		t.Fatalf("open registration: %d %s", w.Code, w.Body)
	}
	transition["stage"] = "preparation"
	if w := request(
		"POST",
		path+"/transition",
		transition,
		2,
	); w.Code != 409 ||
		!strings.Contains(w.Body.String(), "attendance_unavailable") {
		t.Fatalf("gate blockers: %d %s", w.Code, w.Body)
	}
	if w := request("PUT", path, input, 1); w.Code != 409 {
		t.Fatalf("stale edit: %d %s", w.Code, w.Body)
	}
	if w := request(
		"POST",
		"/api/v1/time/resolve",
		map[string]any{
			"wall_time":  "2026-10-25T01:30:00",
			"timezone":   "Europe/London",
			"dst_choice": "",
		},
		1,
	); w.Code != 409 {
		t.Fatalf("ambiguous time: %d %s", w.Code, w.Body)
	}
	if w := request(
		"POST",
		"/api/v1/time/resolve",
		map[string]any{
			"wall_time":  "2026-10-25T01:30:00",
			"timezone":   "Europe/London",
			"dst_choice": "later",
		},
		1,
	); w.Code != 200 {
		t.Fatalf("resolved time: %d %s", w.Code, w.Body)
	}
}
