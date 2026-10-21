package githubidentity

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestVerifiedOwnershipRenamesRelinksAndCallbackDefences(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	type response struct {
		ID               int64
		Login, Challenge string
	}
	var mu sync.Mutex
	codes := map[string]response{}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/token" {
			_ = r.ParseForm()
			mu.Lock()
			data, ok := codes[r.Form.Get("code")]
			mu.Unlock()
			hash := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if !ok || base64.RawURLEncoding.EncodeToString(hash[:]) != data.Challenge ||
				r.Form.Get("redirect_uri") != "https://app.example.test/auth/github/callback" ||
				r.Form.Get("client_secret") != "synthetic-secret" {
				w.WriteHeader(400)
				fmt.Fprint(
					w,
					`{"error":"invalid_grant","error_description":"secret-provider-payload"}`,
				)
				return
			}
			_ = json.NewEncoder(w).
				Encode(map[string]any{"access_token": "secret-access-" + r.Form.Get("code"), "token_type": "bearer", "refresh_token": "secret-refresh", "expires_in": 3600})
			return
		}
		if r.URL.Path == "/user" {
			code := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer secret-access-")
			mu.Lock()
			data, ok := codes[code]
			mu.Unlock()
			if !ok {
				w.WriteHeader(401)
				return
			}
			_ = json.NewEncoder(w).
				Encode(map[string]any{"id": data.ID, "login": data.Login, "type": "User"})
			return
		}
		w.WriteHeader(404)
	}))
	defer fixture.Close()
	s, err := New(
		pool,
		Settings{
			ClientID:     "synthetic",
			ClientSecret: "synthetic-secret",
			TokenKey:     base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)),
			Origin:       "https://app.example.test",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	s.oauth.Endpoint.TokenURL = fixture.URL + "/token"
	s.apiURL = fixture.URL + "/"
	person := testdb.Person(t, pool)
	other := testdb.Person(t, pool)
	event := testdb.Event(t, pool, person.ID)
	session := func() string {
		token := uuid.NewString()
		if err := q.PutSession(
			ctx,
			dbgen.PutSessionParams{
				Token:  hex.EncodeToString(hash(token)),
				Data:   []byte("synthetic-session"),
				Expiry: time.Now().Add(time.Hour),
			},
		); err != nil {
			t.Fatal(err)
		}
		return token
	}
	browser, otherBrowser := session(), session()
	begin := func(actor uuid.UUID, browser string, id int64, login string) (string, string) {
		t.Helper()
		status, err := s.Status(ctx, actor)
		if err != nil {
			t.Fatal(err)
		}
		address, err := s.Begin(ctx, actor, browser, status.Revision, true)
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(address)
		v := u.Query()
		if v.Get("code_challenge_method") != "S256" || v.Get("scope") != "" ||
			v.Get("redirect_uri") != "https://app.example.test/auth/github/callback" {
			t.Fatal("unsafe authorization request")
		}
		code := uuid.NewString()
		mu.Lock()
		codes[code] = response{id, login, v.Get("code_challenge")}
		mu.Unlock()
		return v.Get("state"), code
	}
	state, code := begin(person.ID, browser, 101, "original-name")
	for _, probe := range []struct {
		actor          uuid.UUID
		browser, state string
	}{{other.ID, browser, state}, {person.ID, otherBrowser, state}, {person.ID, browser, "forged"}} {
		if err := s.Complete(
			ctx,
			probe.actor,
			probe.browser,
			probe.state,
			code,
		); !errors.Is(
			err,
			ErrLink,
		) {
			t.Fatal("forged or cross-session callback accepted")
		}
	}
	if err := s.Complete(ctx, person.ID, browser, state, code); err != nil {
		t.Fatal(err)
	}
	if err := s.Complete(ctx, person.ID, browser, state, code); !errors.Is(err, ErrLink) {
		t.Fatal("callback replay accepted")
	}
	row, err := q.GitHubAccount(ctx, person.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(row.TokenEncrypted, []byte("secret-access")) {
		t.Fatal("plaintext credential persisted")
	}
	plain, err := s.decrypt(person.ID, row.TokenEncrypted)
	if err != nil || !bytes.Contains(plain, []byte("secret-refresh")) {
		t.Fatal("credential envelope invalid")
	}
	if _, err := s.decrypt(other.ID, row.TokenEncrypted); err == nil {
		t.Fatal("encrypted token transferable between people")
	}
	status, _ := s.Status(ctx, person.ID)
	wire, _ := json.Marshal(status)
	if bytes.Contains(wire, []byte("token")) || bytes.Contains(wire, []byte("secret")) {
		t.Fatal("credential serialized")
	}
	binding := uuid.New()
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO github_invitation_bindings(id,event_id,person_id,github_id,link_revision) VALUES($1,$2,$3,101,$4)",
		binding,
		event,
		person.ID,
		status.Revision,
	); err != nil {
		t.Fatal(err)
	}
	state, code = begin(person.ID, browser, 101, "renamed-account")
	if err := s.Complete(ctx, person.ID, browser, state, code); err != nil {
		t.Fatal(err)
	}
	status, _ = s.Status(ctx, person.ID)
	if status.Account.GitHubID != 101 || status.Account.Login != "renamed-account" ||
		status.InvitationsNeedingReview != 0 {
		t.Fatal("rename changed ownership")
	}
	state, code = begin(other.ID, otherBrowser, 101, "renamed-account")
	if err := s.Complete(
		ctx,
		other.ID,
		otherBrowser,
		state,
		code,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatalf("duplicate account ownership: %v", err)
	}
	state, code = begin(person.ID, browser, 202, "replacement")
	if err := s.Complete(ctx, person.ID, browser, state, code); err != nil {
		t.Fatal(err)
	}
	status, _ = s.Status(ctx, person.ID)
	if status.InvitationsNeedingReview != 1 {
		t.Fatal("relink failed to flag pending invitation")
	}
	var boundID int64
	var bindingState string
	if err := pool.QueryRow(ctx, "SELECT github_id,state FROM github_invitation_bindings WHERE id=$1", binding).
		Scan(&boundID, &bindingState); err != nil {
		t.Fatal(err)
	}
	if boundID != 101 || bindingState != "requires_review" {
		t.Fatal("pending invitation silently transferred")
	}
	var audits int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE actor_id=$1 AND action='github.relinked'", person.ID).
		Scan(&audits); err != nil ||
		audits != 1 {
		t.Fatal("relink audit absent")
	}
	staleState, staleCode := begin(person.ID, browser, 303, "stale")
	if err := s.Unlink(ctx, person.ID, status.Revision, false); !errors.Is(err, access.ErrInvalid) {
		t.Fatal("unlink skipped impact warning")
	}
	if err := s.Unlink(ctx, person.ID, status.Revision, true); err != nil {
		t.Fatal(err)
	}
	status, _ = s.Status(ctx, person.ID)
	if status.Account != nil {
		t.Fatal("unlink retained account")
	}
	if err := s.Complete(ctx, person.ID, browser, staleState, staleCode); err == nil {
		t.Fatal("outstanding callback restored removed account")
	}
	state, code = begin(person.ID, browser, 303, "expired")
	if _, err := pool.Exec(
		ctx,
		"UPDATE github_link_attempts SET expires_at=now()-interval '1 second'",
	); err != nil {
		t.Fatal(err)
	}
	if err := s.Complete(ctx, person.ID, browser, state, code); !errors.Is(err, ErrLink) {
		t.Fatal("expired callback accepted")
	}
	state, code = begin(person.ID, browser, 303, "revoked")
	if err := q.RevokeSessions(ctx, person.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Complete(
		ctx,
		person.ID,
		browser,
		state,
		code,
	); !errors.Is(
		err,
		access.ErrUnauthenticated,
	) {
		t.Fatal("callback ignored revoked corporate session")
	}
	state, code = begin(person.ID, browser, 303, "logged-out")
	if err := q.DeleteSession(ctx, hex.EncodeToString(hash(browser))); err != nil {
		t.Fatal(err)
	}
	if err := s.Complete(
		ctx,
		person.ID,
		browser,
		state,
		code,
	); !errors.Is(
		err,
		access.ErrUnauthenticated,
	) {
		t.Fatal("callback ignored logout")
	}
	state, _ = begin(other.ID, otherBrowser, 404, "provider-error")
	if err := s.Complete(
		ctx,
		other.ID,
		otherBrowser,
		state,
		"unknown-code",
	); !errors.Is(err, ErrLink) ||
		strings.Contains(err.Error(), "secret-provider") {
		t.Fatal("provider error leaked")
	}
}
