package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/feedback"
	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/moderation"
	"github.com/deploymenttheory/hack-in-a-box/internal/reviewdashboard"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestReviewNavigationCapabilityMatchesCurrentCoverageAuthority(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	owner, mentor, other, admin := testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	if _, err := q.BootstrapAdmin(ctx, admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mentor'),($1,$3,'photographer')`, eventID, mentor.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(func(key string) string {
		if key == "HACKBOX_ENV" {
			return "test"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	auth, err := identity.New(ctx, pool, identity.Settings{Origin: cfg.PublicOrigin, CookiePrefix: "hackbox_public"})
	if err != nil {
		t.Fatal(err)
	}
	deps := Dependencies{Pool: pool, Auth: auth, Events: event.Service{Pool: pool}, ReviewDashboard: &reviewdashboard.Service{Pool: pool, Help: help.Service{Pool: pool}, Cards: &scorecards.Service{Pool: pool}, Moderation: &moderation.Service{Pool: pool}, Feedback: &feedback.Service{Pool: pool}}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouter(cfg, logger, deps)
	cookie := func(person dbgen.Person) *http.Cookie {
		t.Helper()
		session, loadErr := auth.Sessions.Load(ctx, "")
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		auth.Sessions.Put(session, "person_id", person.ID.String())
		auth.Sessions.Put(session, "session_revision", person.SessionRevision)
		value, _, commitErr := auth.Sessions.Commit(session)
		if commitErr != nil {
			t.Fatal(commitErr)
		}
		return &http.Cookie{Name: "hackbox_public_session", Value: value}
	}
	base := "/api/v1/events/" + eventID.String()
	request := func(c *http.Cookie, path string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, cfg.PublicOrigin+path, nil)
		r.AddCookie(c)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		return w
	}
	check := func(c *http.Cookie, want bool) {
		t.Helper()
		w := request(c, base)
		if w.Code != http.StatusOK {
			t.Fatalf("event: %d %s", w.Code, w.Body)
		}
		var view event.View
		if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		if view.CanReadReviewCoverage != want {
			t.Fatalf("review navigation=%t want=%t", view.CanReadReviewCoverage, want)
		}
		w = request(c, base+"/review-coverage")
		code := http.StatusForbidden
		if want {
			code = http.StatusOK
		}
		if w.Code != code {
			t.Fatalf("coverage=%d want=%d %s", w.Code, code, w.Body)
		}
	}
	admin, err = q.Person(ctx, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	check(cookie(owner), true)
	mentorCookie := cookie(mentor)
	check(mentorCookie, true)
	check(cookie(other), false)
	check(cookie(admin), false)
	// An unrelated current role retains EventRead after the mentor grant ends.
	if _, err := pool.Exec(ctx, `INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'photographer')`, eventID, mentor.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mentor'`, eventID, mentor.ID); err != nil {
		t.Fatal(err)
	}
	check(mentorCookie, false)
	// Permission is independent from installation/service availability.
	deps.ReviewDashboard = nil
	router = NewRouter(cfg, logger, deps)
	w := request(cookie(owner), base)
	var view event.View
	if err = json.Unmarshal(w.Body.Bytes(), &view); err != nil || !view.CanReadReviewCoverage {
		t.Fatal("permission was coupled to service availability", err)
	}
	if w = request(cookie(owner), base+"/review-coverage"); w.Code != http.StatusForbidden {
		t.Fatal("unconfigured service unexpectedly available", w.Code)
	}
}
