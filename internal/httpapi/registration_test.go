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
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestRegistrationAllocationHTTPContracts(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()

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
			Pool:         pool,
			Auth:         auth,
			Privacy:      privacy.Service{Pool: pool},
			Templates:    templates.Service{Pool: pool},
			Registration: registration.Service{Pool: pool},
		},
	)
	owner := testdb.Person(t, pool)
	id := testdb.Event(t, pool, owner.ID)

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
	if w := request(
		owner,
		"GET",
		"/api/v1/template-starters",
		nil,
		1,
	); w.Code != 200 ||
		!strings.Contains(w.Body.String(), "qualification-final") {
		t.Fatalf("starters: %d %s", w.Code, w.Body)
	}
	outsider := testdb.Person(t, pool)
	if w := request(outsider, "GET", "/api/v1/template-starters", nil, 1); w.Code != 403 {
		t.Fatal("unprivileged starter access")
	}

	q := dbgen.New(pool)
	c := event.DefaultConfiguration()
	c.Capacity = 1
	c.MinimumAttendance = 1
	c.LiveStart = time.Now().UTC().AddDate(0, 1, 0).Truncate(time.Second)
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	c.SubmissionCutoff = c.LiveStart.Add(6 * time.Hour)
	c.TeamFreeze = c.LiveStart.AddDate(0, 0, -8)
	c.AdmissionCutoff = c.TeamFreeze.Add(-time.Hour)
	data, _ := json.Marshal(c)
	if _, err := pool.Exec(
		ctx,
		"UPDATE events SET configuration=$2,stage='registration' WHERE id=$1",
		id,
		data,
	); err != nil {
		t.Fatal(err)
	}
	if err := q.InitializeEventFacts(ctx, id); err != nil {
		t.Fatal(err)
	}
	definition := forms.Definition{
		ID:       uuid.New(),
		Name:     "Attendee",
		Audience: "attendee",
		Fields:   []forms.Field{},
	}
	service := registration.Service{Pool: pool}
	form, err := service.SaveForm(ctx, owner.ID, id, 0, definition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PublishForm(
		ctx,
		owner.ID,
		id,
		definition.ID,
		form.Revision,
		form.Version,
		true,
	); err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/events/" + id.String() + "/registrations"
	body := registration.Application{
		ID:             uuid.New(),
		FormID:         definition.ID,
		FormVersion:    1,
		Answers:        forms.Answers{},
		TeamPreference: "solo",
	}
	if w := request(outsider, "GET", "/api/v1/registration/events", nil, 0); w.Code != 200 {
		t.Fatalf("discover: %d %s", w.Code, w.Body)
	}
	if w := request(
		outsider,
		"GET",
		"/api/v1/registration/events/"+id.String(),
		nil,
		0,
	); w.Code != 200 ||
		strings.Contains(w.Body.String(), "role_briefs") {
		t.Fatalf("public page: %d %s", w.Code, w.Body)
	}
	if w := request(
		outsider,
		"POST",
		base,
		body,
		0,
	); w.Code != 200 ||
		!strings.Contains(w.Body.String(), "confirmed") {
		t.Fatalf("apply: %d %s", w.Code, w.Body)
	}
	if w := request(outsider, "POST", base, body, 0); w.Code != 200 {
		t.Fatal("retry failed")
	}
	if w := request(
		owner,
		"POST",
		base,
		registration.Application{
			ID:             uuid.New(),
			FormID:         definition.ID,
			FormVersion:    1,
			Answers:        forms.Answers{},
			TeamPreference: "solo",
		},
		0,
	); w.Code != 409 {
		t.Fatal("organiser competed")
	}
	other := testdb.Person(t, pool)
	if w := request(
		other,
		"GET",
		base+"/mine",
		nil,
		0,
	); w.Code != 200 ||
		strings.Contains(w.Body.String(), body.ID.String()) {
		t.Fatal("other registration exposed")
	}
	policyPath := base + "/offer-policy"
	if w := request(
		other,
		"PUT",
		policyPath,
		map[string]any{"automated": true, "working_days": 2},
		0,
	); w.Code != 403 {
		t.Fatalf("unprivileged automation: %d %s", w.Code, w.Body)
	}
	if w := request(
		owner,
		"PUT",
		policyPath,
		map[string]any{"automated": true, "working_days": 2},
		0,
	); w.Code != 200 {
		t.Fatalf("approve automation: %d %s", w.Code, w.Body)
	}
	responsePath := base + "/" + body.ID.String() + "/response"
	if w := request(other, "GET", responsePath, nil, 0); w.Code != 404 {
		t.Fatal("private answers exposed")
	}
	if w := request(outsider, "GET", responsePath, nil, 0); w.Code != 200 {
		t.Fatalf("own answers: %d %s", w.Code, w.Body)
	}
	if w := request(outsider, "PUT", responsePath, forms.Answers{}, 1); w.Code != 200 {
		t.Fatalf("correct answers: %d %s", w.Code, w.Body)
	}
	if w := request(
		outsider,
		"POST",
		base+"/"+body.ID.String()+"/respond",
		map[string]string{"action": "withdraw"},
		2,
	); w.Code != 200 {
		t.Fatalf("withdraw: %d %s", w.Code, w.Body)
	}

	if _, err := pool.Exec(
		ctx,
		"UPDATE events SET stage='preparation' WHERE id=$1",
		id,
	); err != nil {
		t.Fatal(err)
	}
	if w := request(
		other,
		"POST",
		base,
		registration.Application{
			ID:             uuid.New(),
			FormID:         definition.ID,
			FormVersion:    1,
			Answers:        forms.Answers{},
			TeamPreference: "solo",
		},
		0,
	); w.Code != 409 {
		t.Fatal("closed registration accepted")
	}
	if w := request(
		outsider,
		"GET",
		"/api/v1/registration/events/"+id.String(),
		nil,
		0,
	); w.Code != 200 {
		t.Fatal("own status inaccessible after closure")
	}
	if w := request(
		outsider,
		"GET",
		"/api/v1/registration/events",
		nil,
		0,
	); w.Code != 200 ||
		!strings.Contains(w.Body.String(), id.String()) {
		t.Fatalf("closed own status not discoverable: %d %s", w.Code, w.Body)
	}
	if w := request(
		other,
		"GET",
		"/api/v1/registration/events",
		nil,
		0,
	); w.Code != 200 ||
		strings.Contains(w.Body.String(), id.String()) {
		t.Fatalf("closed unrelated event exposed: %d %s", w.Code, w.Body)
	}
}
