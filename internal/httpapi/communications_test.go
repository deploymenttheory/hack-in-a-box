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

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestCommunicationHTTPContractsCurrentAuthorityAndApproval(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
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
	router := NewRouter(
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Dependencies{Pool: pool, Auth: auth, Communications: communications.Service{Pool: pool}},
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
			t.Fatal("message privacy or cache boundary failed")
		}
	}

	base := "/api/v1/events/" + id.String() + "/communications"
	check(request(owner, "GET", base, nil, rev(), true), 200)
	for _, actor := range []dbgen.Person{recipient, admin} {
		check(request(actor, "GET", base, nil, rev(), true), 403)
	}
	check(request(dbgen.Person{}, "GET", base, nil, rev(), true), 401)
	templateID, audienceID := uuid.New(), uuid.New()
	template := map[string]any{
		"id":      templateID,
		"version": 0,
		"name":    "Safe reminder",
		"purpose": "reminder",
		"format":  "html",
		"subject": "Hello {{person.name}}",
		"body":    "{{event.name}}",
	}
	command := map[string]any{
		"action":   "save_template",
		"template": template,
		"reason":   "Synthetic review",
	}
	check(request(owner, "POST", base, command, rev(), false), 403)
	check(request(owner, "POST", base, command, rev()+1, true), 409)
	check(request(owner, "POST", base, command, rev(), true), 200)
	check(
		request(
			owner,
			"POST",
			base,
			map[string]any{
				"action": "save_audience",
				"reason": "Synthetic selector",
				"audience": map[string]any{
					"id":                  audienceID,
					"version":             0,
					"name":                "Confirmed",
					"roles":               []string{"attendee"},
					"registration_states": []string{},
					"teams":               []string{},
					"outstanding":         "",
				},
			},
			rev(),
			true,
		),
		200,
	)
	response := request(
		owner,
		"POST",
		base,
		map[string]any{
			"action":      "preview",
			"reason":      "Synthetic preview",
			"template_id": templateID,
			"audience_id": audienceID,
		},
		rev(),
		true,
	)
	check(response, 200)
	var page communications.Page
	if err = json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Preview == nil || len(page.Preview.Recipients) != 1 {
		t.Fatalf("preview response: %s", response.Body)
	}
	check(
		request(
			owner,
			"POST",
			base,
			map[string]any{
				"action":     "approve",
				"reason":     "Reviewed exact preview",
				"preview_id": page.Preview.ID,
			},
			rev(),
			true,
		),
		200,
	)
	check(
		request(
			owner,
			"POST",
			base,
			map[string]any{
				"action":           "preview",
				"reason":           "Bad field",
				"private_question": "PRIVATE-QUESTION",
			},
			rev(),
			true,
		),
		400,
	)
	if _, err = pool.Exec(
		ctx,
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2",
		id,
		owner.ID,
	); err != nil {
		t.Fatal(err)
	}
	check(request(owner, "GET", base, nil, rev(), true), 403)
	check(request(owner, "POST", base, command, rev(), true), 403)
}
