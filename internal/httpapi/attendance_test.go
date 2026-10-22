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
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestAttendanceHTTPContractsAndSensitiveProjections(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
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
		Dependencies{
			Pool:       pool,
			Auth:       auth,
			Attendance: attendance.Service{Pool: pool},
			Privacy:    privacy.Service{Pool: pool},
			Events:     event.Service{Pool: pool},
		},
	)
	owner, logistic, competitor := testdb.Person(
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
	c := event.DefaultConfiguration()
	c.LiveStart = time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	data, _ := json.Marshal(c)
	record, err := q.SaveEventConfiguration(
		ctx,
		dbgen.SaveEventConfigurationParams{
			ID:            id,
			Name:          "Synthetic event",
			Slug:          uuid.NewString(),
			Configuration: data,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.InitializeEventFacts(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := q.SetMembership(
		ctx,
		dbgen.SetMembershipParams{
			EventID:  id,
			PersonID: logistic.ID,
			Role:     "logistics",
			Active:   true,
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := q.SetCompetitionState(
		ctx,
		dbgen.SetCompetitionStateParams{EventID: id, PersonID: competitor.ID, State: "confirmed"},
	); err != nil {
		t.Fatal(err)
	}
	travel, social := "sensitive-journey", "unknown"
	if _, err := (privacy.Service{Pool: pool}).Correct(
		ctx,
		competitor.ID,
		id,
		0,
		privacy.Fields{
			Dietary:          "sensitive-diet",
			Accessibility:    "sensitive-access",
			PhotoConsent:     "no",
			TravelDetails:    &travel,
			SocialAttendance: &social,
		},
	); err != nil {
		t.Fatal(err)
	}
	request := func(person dbgen.Person, method, path string, body any, revision int64, csrfOK bool) *httptest.ResponseRecorder {
		t.Helper()
		session, err := auth.Sessions.Load(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		csrf := strings.Repeat("a", 32)
		auth.Sessions.Put(session, "person_id", person.ID.String())
		auth.Sessions.Put(session, "session_revision", person.SessionRevision)
		auth.Sessions.Put(session, "csrf", csrf)
		token, _, err := auth.Sessions.Commit(session)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("If-Match", fmt.Sprintf("\"%d\"", revision))
		if !csrfOK {
			csrf = strings.Repeat("x", 32)
		}
		r.Header.Set("X-CSRF-Token", csrf)
		r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		return w
	}
	base := "/api/v1/events/" + id.String()
	arrival := base + "/attendance/" + competitor.ID.String() + "/2027-06-20"
	response := request(logistic, "GET", base+"/attendance?day=2027-06-20", nil, 0, true)
	if response.Code != 200 || strings.Contains(response.Body.String(), "sensitive") ||
		strings.Contains(response.Body.String(), "photo_consent") {
		t.Fatalf("shared roster: %d %s", response.Code, response.Body)
	}
	var page attendance.Page
	if err := json.Unmarshal(
		response.Body.Bytes(),
		&page,
	); err != nil || page.Counts.ConfirmedCompetitors != 1 ||
		!page.CanReadLogistics {
		t.Fatal(page, err)
	}
	command := attendance.Command{RequestID: uuid.New(), Present: true}
	if w := request(logistic, "PUT", arrival, command, 0, false); w.Code != 403 {
		t.Fatalf("CSRF %d %s", w.Code, w.Body)
	}
	if w := request(competitor, "PUT", arrival, command, 0, true); w.Code != 403 {
		t.Fatalf("self check-in %d %s", w.Code, w.Body)
	}
	for range 2 {
		if w := request(
			logistic,
			"PUT",
			arrival,
			command,
			0,
			true,
		); w.Code != 200 ||
			!strings.Contains(w.Body.String(), `"revision":1`) {
			t.Fatalf("idempotent check-in %d %s", w.Code, w.Body)
		}
	}
	correction := attendance.Command{
		RequestID: uuid.New(),
		Present:   false,
		Reason:    "Wrong person selected",
	}
	if w := request(owner, "PUT", arrival, correction, 0, true); w.Code != 409 {
		t.Fatalf("stale correction %d %s", w.Code, w.Body)
	}
	if w := request(owner, "PUT", arrival, correction, 1, true); w.Code != 200 {
		t.Fatalf("correction %d %s", w.Code, w.Body)
	}
	if w := request(
		logistic,
		"GET",
		arrival+"/history",
		nil,
		0,
		true,
	); w.Code != 200 ||
		!strings.Contains(w.Body.String(), correction.Reason) {
		t.Fatalf("history %d %s", w.Code, w.Body)
	}
	if w := request(
		owner,
		"PUT",
		base+"/attendance/catering",
		map[string]int{"buffer": 5},
		record.Revision,
		true,
	); w.Code != 200 ||
		!strings.Contains(w.Body.String(), `"catering_total":8`) {
		t.Fatalf("catering %d %s", w.Code, w.Body)
	}
	if w := request(owner, "GET", base+"/logistics", nil, 0, true); w.Code != 403 {
		t.Fatalf("organiser received sensitive data %d", w.Code)
	}
	w := request(logistic, "GET", base+"/logistics/export?limit=100", nil, 0, true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "sensitive-journey") ||
		strings.Contains(w.Body.String(), "photo_consent") ||
		w.Header().Get("Content-Disposition") == "" {
		t.Fatalf("logistics export %d %s", w.Code, w.Body)
	}
	if w := request(competitor, "GET", base+"/logistics/export", nil, 0, true); w.Code != 403 {
		t.Fatal("competitor exported logistics", w.Code)
	}
	if w := request(
		logistic,
		"GET",
		base+"/attendance?day=2027-06-21",
		nil,
		0,
		true,
	); w.Code != 400 {
		t.Fatalf("outside live dates %d %s", w.Code, w.Body)
	}
	other := testdb.Event(t, pool, competitor.ID)
	if w := request(
		logistic,
		"GET",
		"/api/v1/events/"+other.String()+"/attendance",
		nil,
		0,
		true,
	); w.Code != 403 {
		t.Fatal("cross-event roster", w.Code)
	}
	if err := q.SetCompetitionState(
		ctx,
		dbgen.SetCompetitionStateParams{EventID: id, PersonID: competitor.ID, State: "withdrawn"},
	); err != nil {
		t.Fatal(err)
	}
	if w := request(logistic, "PUT", arrival, command, 0, true); w.Code != 403 {
		t.Fatal("withdrawn attendee replayed check-in", w.Code)
	}
	if w := request(
		logistic,
		"GET",
		base+"/logistics",
		nil,
		0,
		true,
	); w.Code != 200 ||
		strings.Contains(w.Body.String(), "sensitive-") {
		t.Fatalf("withdrawn fields %d %s", w.Code, w.Body)
	}
	if err := q.SetMembership(
		ctx,
		dbgen.SetMembershipParams{
			EventID:  id,
			PersonID: logistic.ID,
			Role:     "logistics",
			Active:   false,
		},
	); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{base + "/attendance", base + "/logistics/export", arrival + "/history"} {
		if w := request(logistic, "GET", path, nil, 0, true); w.Code != 403 {
			t.Fatalf("revoked access %s: %d", path, w.Code)
		}
	}
}
