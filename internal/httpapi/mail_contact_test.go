package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/mailcontact"
	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type accountMailSender struct{ messages []maildelivery.Message }

func (*accountMailSender) Configured() bool { return true }

func (s *accountMailSender) Send(
	_ context.Context,
	message maildelivery.Message,
) maildelivery.Receipt {
	s.messages = append(s.messages, message)
	return maildelivery.Receipt{State: maildelivery.Accepted, Code: "relay_accepted"}
}

func TestAccountEmailHTTPOwnershipCSRFPrivacyAndCurrentSession(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	person, other := testdb.Person(t, pool), testdb.Person(t, pool)
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
	sender := &accountMailSender{}
	var logs bytes.Buffer
	router := NewRouter(
		cfg,
		slog.New(slog.NewTextHandler(&logs, nil)),
		Dependencies{
			Pool:         pool,
			Auth:         auth,
			MailContacts: mailcontact.Service{Pool: pool, Sender: sender},
		},
	)
	csrf := strings.Repeat("x", 32)
	tokens := map[uuid.UUID]string{}
	request := func(actor dbgen.Person, method, path string, body any, goodCSRF bool, status int) *httptest.ResponseRecorder {
		t.Helper()
		var data []byte
		if body != nil {
			data, _ = json.Marshal(body)
		}
		r := httptest.NewRequest(method, path, bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", cfg.PublicOrigin)
		if goodCSRF {
			r.Header.Set("X-CSRF-Token", csrf)
		} else {
			r.Header.Set("X-CSRF-Token", strings.Repeat("z", 32))
		}
		if actor.ID != uuid.Nil {
			if tokens[actor.ID] == "" {
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
			r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: tokens[actor.ID]})
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, r)
		validateResponse(t, r, response)
		if response.Code != status {
			t.Fatalf(
				"account email %s:status%d want%d (%s)",
				method,
				response.Code,
				status,
				response.Body.String(),
			)
		}
		if !strings.Contains(response.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("account email response was cacheable")
		}
		return response
	}
	const base = "/api/v1/account/email"
	const address = "synthetic-private-mailbox@example.test"
	request(dbgen.Person{}, "GET", base, nil, true, 401)
	request(
		dbgen.Person{},
		"POST",
		base+"/verification",
		map[string]any{"email": address},
		true,
		401,
	)
	request(person, "POST", base+"/verification", map[string]any{"email": address}, false, 403)
	request(
		person,
		"POST",
		base+"/verification",
		map[string]any{"email": address, "person_id": other.ID},
		true,
		400,
	)
	request(
		person,
		"POST",
		base+"/verification",
		map[string]any{"email": "a@example.test,b@example.test"},
		true,
		400,
	)
	request(
		person,
		"POST",
		base+"/verification",
		map[string]any{"email": "a@example.test\r\nBcc:b@example.test"},
		true,
		400,
	)
	if len(sender.messages) != 0 {
		t.Fatal("rejected input reached SMTP")
	}
	response := request(
		person,
		"POST",
		base+"/verification",
		map[string]any{"email": address},
		true,
		202,
	)
	if len(sender.messages) != 1 || sender.messages[0].Recipient != address ||
		strings.Contains(sender.messages[0].PlainText, "event") {
		t.Fatal("confirmation was not a neutral individual message")
	}
	match := regexp.MustCompile(`code is ([0-9]{8})`).
		FindStringSubmatch(sender.messages[0].PlainText)
	if len(match) != 2 {
		t.Fatal("confirmation code absent from neutral provider message")
	}
	code := match[1]
	assertPrivate := func(response *httptest.ResponseRecorder) {
		t.Helper()
		if strings.Contains(response.Body.String(), address) ||
			strings.Contains(response.Body.String(), code) {
			t.Fatal("wire response exposed private mailbox or code")
		}
	}
	assertPrivate(response)
	var status mailcontact.Status
	if err = json.Unmarshal(
		response.Body.Bytes(),
		&status,
	); err != nil || status.Verified || !status.Pending ||
		status.RequestState != "accepted" {
		t.Fatal("SMTP acceptance was treated as mailbox proof")
	}
	assertPrivate(request(person, "GET", base, nil, true, 200))
	request(person, "POST", base+"/verification", map[string]any{"email": address}, true, 429)
	request(other, "POST", base+"/confirmation", map[string]any{"code": code}, true, 400)
	wrong := fmt.Sprintf("%d%s", (int(code[0]-'0')+1)%10, code[1:])
	assertPrivate(
		request(person, "POST", base+"/confirmation", map[string]any{"code": wrong}, true, 400),
	)
	var failures int32
	if err = pool.QueryRow(ctx, "SELECT failed_attempts FROM email_verifications WHERE person_id=$1", person.ID).
		Scan(&failures); err != nil ||
		failures != 1 {
		t.Fatal("bad-code evidence rolled back")
	}
	response = request(
		person,
		"POST",
		base+"/confirmation",
		map[string]any{"code": code},
		true,
		200,
	)
	assertPrivate(response)
	if err = json.Unmarshal(
		response.Body.Bytes(),
		&status,
	); err != nil || !status.Verified ||
		status.Provenance != "code_confirmed" {
		t.Fatal("code proof did not confirm only the current account")
	}
	assertPrivate(
		request(person, "POST", base+"/confirmation", map[string]any{"code": code}, true, 400),
	)
	response = request(other, "GET", base, nil, true, 200)
	assertPrivate(response)
	if err = json.Unmarshal(response.Body.Bytes(), &status); err != nil || status.Verified {
		t.Fatal("mailbox confirmation crossed account")
	}
	if err = dbgen.New(pool).RevokeSessions(ctx, person.ID); err != nil {
		t.Fatal(err)
	}
	request(person, "GET", base, nil, true, 401)
	request(person, "POST", base+"/verification", map[string]any{"email": address}, true, 401)
	if _, err = pool.Exec(ctx, "UPDATE people SET active=false WHERE id=$1", other.ID); err != nil {
		t.Fatal(err)
	}
	request(other, "GET", base, nil, true, 401)
	request(other, "POST", base+"/confirmation", map[string]any{"code": code}, true, 401)
	if len(sender.messages) != 1 {
		t.Fatal("stale/inactive session sent mail")
	}
	var audit string
	if err = pool.QueryRow(ctx, "SELECT coalesce(string_agg(row_to_json(a)::text,' '),'') FROM audit_events a").
		Scan(&audit); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{logs.String(), audit} {
		if strings.Contains(text, address) || strings.Contains(text, code) {
			t.Fatal("private mailbox/code entered logs or audit")
		}
	}
}
