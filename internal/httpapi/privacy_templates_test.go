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
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestPrivacyAndTemplateHTTPContracts(t *testing.T) {
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
			Pool:      pool,
			Auth:      auth,
			Privacy:   privacy.Service{Pool: pool},
			Templates: templates.Service{Pool: pool},
		},
	)
	owner := testdb.Person(t, pool)
	id := testdb.Event(t, pool, owner.ID)
	other := testdb.Event(t, pool, owner.ID)
	request := func(person dbgen.Person, method, path string, body any, revision int64) *httptest.ResponseRecorder {
		session, err := auth.Sessions.Load(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		csrf := strings.Repeat("s", 32)
		auth.Sessions.Put(session, "person_id", person.ID.String())
		auth.Sessions.Put(session, "session_revision", person.SessionRevision)
		auth.Sessions.Put(session, "csrf", csrf)
		token, _, err := auth.Sessions.Commit(session)
		if err != nil {
			t.Fatal(err)
		}
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
	base := "/api/v1/events/" + id.String()
	profilePath := base + "/profiles/" + owner.ID.String()
	fields := privacy.Fields{
		ContactEmail:  "private@example.test",
		Dietary:       "sensitive-diet",
		Accessibility: "sensitive-access",
		PhotoConsent:  "no",
	}
	if w := request(owner, "PUT", profilePath, fields, 0); w.Code != 200 {
		t.Fatalf("correction: %d %s", w.Code, w.Body)
	}
	for _, role := range []string{"organiser", "mentor", "judge", "mc", "photographer", "logistics"} {
		viewer := testdb.Person(t, pool)
		if err := q.SetMembership(
			ctx,
			dbgen.SetMembershipParams{EventID: id, PersonID: viewer.ID, Role: role, Active: true},
		); err != nil {
			t.Fatal(err)
		}
		w := request(viewer, "GET", profilePath, nil, 1)
		if role == "photographer" {
			if w.Code != 200 || strings.Contains(w.Body.String(), "sensitive-") ||
				strings.Contains(w.Body.String(), "contact_email") {
				t.Fatalf("photographer payload: %s", w.Body)
			}
		} else if role == "logistics" {
			if w.Code != 200 || strings.Contains(w.Body.String(), "photo_consent") {
				t.Fatalf("logistics payload: %s", w.Body)
			}
		} else if w.Code != 403 || strings.Contains(w.Body.String(), "sensitive-") {
			t.Fatalf("%s private payload: %d %s", role, w.Code, w.Body)
		}
		export := request(viewer, "GET", base+"/profiles/export", nil, 1)
		if role == "logistics" {
			if export.Code != 200 ||
				!strings.Contains(export.Header().Get("Content-Disposition"), "attachment") {
				t.Fatal("export contract")
			}
		} else if export.Code != 403 {
			t.Fatalf("%s exported profiles", role)
		}
		// Same person has legitimate access to both events; the profile exists only in A.
		if role == "logistics" {
			if err := q.SetMembership(
				ctx,
				dbgen.SetMembershipParams{
					EventID:  other,
					PersonID: viewer.ID,
					Role:     role,
					Active:   true,
				},
			); err != nil {
				t.Fatal(err)
			}
			w := request(
				viewer,
				"GET",
				"/api/v1/events/"+other.String()+"/profiles/"+owner.ID.String(),
				nil,
				1,
			)
			if w.Code != 404 {
				t.Fatalf("event-scope predicate missing: %d %s", w.Code, w.Body)
			}
		}
	}
	if w := request(
		owner,
		"PUT",
		base+"/privacy",
		privacy.Settings{RetentionDays: 30, DraftDays: 2},
		1,
	); w.Code != 200 {
		t.Fatalf("settings: %d %s", w.Code, w.Body)
	}
	if w := request(owner, "GET", base+"/privacy", nil, 1); w.Code != 200 {
		t.Fatal(w.Body)
	}
	c := event.DefaultConfiguration()
	c.LiveStart = time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	input := templates.Input{
		ID:            uuid.New(),
		Name:          "HTTP template",
		Configuration: templates.Bundle{Event: c, Definitions: templates.EmptyDefinitions()},
	}
	if w := request(owner, "POST", "/api/v1/templates", input, 1); w.Code != 200 {
		t.Fatalf("template creation: %d %s", w.Code, w.Body)
	}
	path := "/api/v1/templates/" + input.ID.String()
	for _, path := range []string{path, path + "?version=1", "/api/v1/templates?limit=1", base + "/configuration-snapshot"} {
		if w := request(owner, "GET", path, nil, 1); w.Code != 200 {
			t.Fatalf("read %s: %d %s", path, w.Code, w.Body)
		}
	}
	if w := request(owner, "PUT", path, input, 1); w.Code != 200 {
		t.Fatalf("edit: %d %s", w.Code, w.Body)
	}
	if w := request(
		owner,
		"POST",
		path+"/publish",
		map[string]int{"version": 2},
		2,
	); w.Code != 200 {
		t.Fatalf("publish: %d %s", w.Code, w.Body)
	}
	clone := templates.CloneInput{
		ID:                                uuid.New(),
		Name:                              "HTTP clone",
		Slug:                              "http-clone",
		LiveStart:                         c.LiveStart.AddDate(0, 1, 0),
		Timezone:                          c.Timezone,
		Version:                           2,
		AcknowledgeCompetitionRestriction: true,
	}
	if w := request(owner, "POST", path+"/clone", clone, 1); w.Code != 200 {
		t.Fatalf("clone: %d %s", w.Code, w.Body)
	}
	if w := request(
		owner,
		"PUT",
		base+"/configuration-snapshot",
		templates.EmptyDefinitions(),
		2,
	); w.Code != 200 {
		t.Fatalf("save definitions: %d %s", w.Code, w.Body)
	}
	raw, _ := json.Marshal(input)
	var bad map[string]any
	_ = json.Unmarshal(raw, &bad)
	bad["participants"] = []string{"must-never-copy"}
	if w := request(owner, "POST", "/api/v1/templates", bad, 1); w.Code != 400 {
		t.Fatal("unknown live data accepted by clone contract")
	}
	current, err := q.Event(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	brand := templates.EmptyDefinitions().Branding
	brand.Primary = "#233c78"
	if w := request(owner, "PUT", base+"/branding", brand, current.Revision); w.Code != 200 {
		t.Fatalf("branding: %d %s", w.Code, w.Body)
	}
	if w := request(owner, "PUT", base+"/branding", brand, current.Revision); w.Code != 409 {
		t.Fatal("stale branding saved")
	}
	viewer := testdb.Person(t, pool)
	if err := q.SetMembership(
		ctx,
		dbgen.SetMembershipParams{EventID: id, PersonID: viewer.ID, Role: "mentor", Active: true},
	); err != nil {
		t.Fatal(err)
	}
	if w := request(
		viewer,
		"GET",
		base+"/branding",
		nil,
		0,
	); w.Code != 200 ||
		strings.Contains(w.Body.String(), "role_briefs") {
		t.Fatalf("branding payload: %d %s", w.Code, w.Body)
	}
	if w := request(viewer, "PUT", base+"/branding", brand, current.Revision+1); w.Code != 403 {
		t.Fatal("non-organiser edited branding")
	}
	brand.Foreground = brand.Background
	if w := request(
		owner,
		"PUT",
		base+"/branding",
		brand,
		current.Revision+1,
	); w.Code != 409 ||
		!strings.Contains(w.Body.String(), "contrast") {
		t.Fatalf("contrast error: %d %s", w.Code, w.Body)
	}
}
