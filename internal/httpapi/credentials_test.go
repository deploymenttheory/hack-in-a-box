package httpapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/credentials"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestCredentialsHTTPPrivacyExplicitRevealAndFailures(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	owner, member, stranger := testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	)
	eventID := testdb.Event(t, pool, owner.ID)
	team := uuid.New()
	q := dbgen.New(pool)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := pool.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec(
		"INSERT INTO hack_teams(id,event_id,name,state) VALUES($1,$2,'Synthetic team','published')",
		team,
		eventID,
	)
	exec(
		"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
		eventID,
		member.ID,
	)
	exec(
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
		eventID,
		team,
		member.ID,
	)
	if e := q.CreateCompetitionEntry(
		ctx,
		dbgen.CreateCompetitionEntryParams{EventID: eventID, TeamID: team},
	); e != nil {
		t.Fatal(e)
	}
	cfg, e := config.Load(func(k string) string {
		if k == "HACKBOX_ENV" {
			return "test"
		}
		return ""
	})
	if e != nil {
		t.Fatal(e)
	}
	auth, e := identity.New(ctx, pool, identity.Settings{Origin: cfg.PublicOrigin})
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(
		map[string]any{
			"active": "test",
			"keys": map[string]string{
				"test": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)),
			},
		},
	)
	keys, e := secretbox.Parse(string(raw))
	if e != nil {
		t.Fatal(e)
	}
	var logs bytes.Buffer
	router := NewRouter(
		cfg,
		slog.New(slog.NewTextHandler(&logs, nil)),
		Dependencies{
			Pool:        pool,
			Auth:        auth,
			Credentials: credentials.Service{Pool: pool, Keys: keys},
		},
	)
	csrf := strings.Repeat("a", 32)
	tokens := map[uuid.UUID]string{}
	revision := func() int64 {
		t.Helper()
		r, e := q.Event(ctx, eventID)
		if e != nil {
			t.Fatal(e)
		}
		return r.Revision
	}
	request := func(person dbgen.Person, method, path string, body any, goodCSRF bool) *httptest.ResponseRecorder {
		t.Helper()
		var data []byte
		if body != nil {
			data, _ = json.Marshal(body)
		}
		r := httptest.NewRequest(method, path, bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("If-Match", fmt.Sprintf("\"%d\"", revision()))
		if goodCSRF {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		if person.ID != uuid.Nil {
			if tokens[person.ID] == "" {
				session, e := auth.Sessions.Load(ctx, "")
				if e != nil {
					t.Fatal(e)
				}
				auth.Sessions.Put(session, "person_id", person.ID.String())
				auth.Sessions.Put(session, "session_revision", person.SessionRevision)
				auth.Sessions.Put(session, "csrf", csrf)
				token, _, e := auth.Sessions.Commit(session)
				if e != nil {
					t.Fatal(e)
				}
				tokens[person.ID] = token
			}
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
			t.Fatalf("status %d want %d: %s", w.Code, status, w.Body.String())
		}
		if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatalf("missing no-store: %v", w.Header())
		}
	}
	base := "/api/v1/events/" + eventID.String() + "/credentials"
	input := map[string]any{
		"action":   "create",
		"label":    "Synthetic service",
		"value":    "SYNTHETIC-HTTP-SECRET",
		"team_ids": []uuid.UUID{team},
	}
	check(request(owner, "POST", base, input, false), 400)
	check(request(stranger, "POST", base, input, true), 403)
	w := request(owner, "POST", base, input, true)
	check(w, 200)
	if strings.Contains(w.Body.String(), "SECRET") ||
		strings.Contains(w.Body.String(), "ciphertext") {
		t.Fatal("set returned secret")
	}
	var page credentials.Page
	if e = json.Unmarshal(w.Body.Bytes(), &page); e != nil {
		t.Fatal(e)
	}
	id := page.Items[0].ID
	reveal := base + "/" + id.String() + "/reveal"
	w = request(member, "GET", base, nil, true)
	check(w, 200)
	if strings.Contains(w.Body.String(), "SECRET") {
		t.Fatal("list returned secret")
	}
	check(request(stranger, "POST", reveal, nil, true), 403)
	check(request(owner, "POST", reveal, nil, true), 403)
	check(request(dbgen.Person{}, "POST", reveal, nil, true), 401)
	w = request(member, "POST", reveal, nil, true)
	check(w, 200)
	if !strings.Contains(w.Body.String(), "SYNTHETIC-HTTP-SECRET") {
		t.Fatal("explicit reveal missing value")
	}
	exec("UPDATE team_credentials SET ciphertext='\\x00' WHERE id=$1", id)
	w = request(member, "POST", reveal, nil, true)
	check(w, 503)
	if strings.Contains(w.Body.String(), "SECRET") {
		t.Fatal("key failure echoed value")
	}
	exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		eventID,
		member.ID,
	)
	check(request(member, "POST", reveal, nil, true), 403)
	w = request(member, "GET", base, nil, true)
	check(w, 403)
	if strings.Contains(w.Body.String(), id.String()) {
		t.Fatal("withdrawn member retained metadata")
	}
	var count int
	if e = pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE action='credential.revealed' AND target_id=$1", id).
		Scan(&count); e != nil ||
		count != 5 {
		t.Fatalf("audit attempts %d %v", count, e)
	}
	var auditText string
	if e = pool.QueryRow(ctx, "SELECT string_agg(metadata::text,' ') FROM audit_events WHERE target_id=$1", id).
		Scan(&auditText); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(logs.String(), "SECRET") || strings.Contains(auditText, "SECRET") {
		t.Fatal("secret leaked to logs or audit")
	}
}
