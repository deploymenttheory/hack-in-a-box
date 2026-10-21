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

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestRegistrationFormHTTPContracts(t *testing.T) {
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

	base := "/api/v1/events/" + id.String() + "/registration/forms"
	definition := forms.Definition{
		ID:       uuid.New(),
		Name:     "HTTP registration",
		Audience: "attendee",
		Fields: []forms.Field{
			{
				ID:       uuid.New(),
				Label:    "Choice",
				Type:     "single",
				Required: true,
				Choices:  []string{"yes", "no"},
			},
		},
	}
	path := base + "/" + definition.ID.String()
	if w := request(owner, "PUT", path, definition, 0); w.Code != 200 {
		t.Fatalf("save form: %d %s", w.Code, w.Body)
	}
	if w := request(owner, "GET", base, nil, 0); w.Code != 200 {
		t.Fatalf("list forms: %d %s", w.Code, w.Body)
	}
	if w := request(
		owner,
		"POST",
		base+"/preview",
		map[string]any{"definition": definition, "answers": map[string]any{}},
		0,
	); w.Code != 200 ||
		!strings.Contains(w.Body.String(), "required question") {
		t.Fatalf("preview: %d %s", w.Code, w.Body)
	}
	if w := request(
		owner,
		"POST",
		path+"/publish",
		map[string]any{"version": 1, "acknowledge_response_versions": false},
		1,
	); w.Code != 400 {
		t.Fatal("missing acknowledgment accepted")
	}
	if w := request(
		owner,
		"POST",
		path+"/publish",
		map[string]any{"version": 1, "acknowledge_response_versions": true},
		1,
	); w.Code != 200 {
		t.Fatalf("publish: %d %s", w.Code, w.Body)
	}
	if w := request(outsider, "GET", base, nil, 0); w.Code != 403 {
		t.Fatal("form drafts leaked")
	}
	if w := request(owner, "PUT", path, definition, 1); w.Code != 409 {
		t.Fatal("stale form accepted")
	}
}
