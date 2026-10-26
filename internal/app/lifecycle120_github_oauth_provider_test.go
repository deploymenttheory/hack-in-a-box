package app

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type lifecycleGitHubCode struct {
	Actor     int
	Redirect  string
	Challenge string
	Expires   time.Time
}
type lifecycleGitHubUserToken struct {
	Actor   int
	Expires time.Time
}

func lifecycleGitHubActor(raw string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimPrefix(raw, "attendee-"))
	return n, err == nil && n >= 1 && n <= 36 && raw == fmt.Sprintf("attendee-%02d", n)
}

// Actor selection belongs exclusively to this synthetic external identity
// provider. Native callback state, browser session, PKCE and encrypted token
// storage are still owned and verified by the real application.
func (g *lifecycleGitHub) oauthHTTP(w http.ResponseWriter, r *http.Request) bool {
	fail := func(status int) {
		w.WriteHeader(status)
		lifecycleJSON(w, map[string]string{"error": "invalid_synthetic_oauth_request"})
	}
	switch r.URL.Path {
	case "/github/login/oauth/authorize":
		q := r.URL.Query()
		actor, valid := lifecycleGitHubActor(q.Get("actor"))
		redirect := q.Get("redirect_uri")
		allowed := redirect == "http://public:8080/auth/github/callback" || redirect == "http://admin:8080/auth/github/callback"
		challenge := q.Get("code_challenge")
		decoded, err := base64.RawURLEncoding.DecodeString(challenge)
		if r.Method != http.MethodGet || !valid || !allowed || q.Get("client_id") != "lifecycle-github" || q.Get("response_type") != "code" || len(q.Get("state")) < 32 || len(q.Get("state")) > 256 || q.Get("code_challenge_method") != "S256" || err != nil || len(decoded) != 32 {
			fail(http.StatusBadRequest)
			return true
		}
		code := rand.Text() + rand.Text()
		g.mu.Lock()
		for key, value := range g.codes {
			if time.Now().After(value.Expires) {
				delete(g.codes, key)
			}
		}
		if len(g.codes) >= 128 {
			g.mu.Unlock()
			fail(http.StatusTooManyRequests)
			return true
		}
		g.codes[code] = lifecycleGitHubCode{Actor: actor, Redirect: redirect, Challenge: challenge, Expires: time.Now().Add(5 * time.Minute)}
		browser := rand.Text() + rand.Text()
		for key, value := range g.webSessions {
			if time.Now().After(value.Expires) {
				delete(g.webSessions, key)
			}
		}
		if len(g.webSessions) >= 128 {
			g.mu.Unlock()
			fail(http.StatusTooManyRequests)
			return true
		}
		g.webSessions[browser] = lifecycleGitHubUserToken{Actor: actor, Expires: time.Now().Add(time.Hour)}
		g.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "lifecycle_github_browser", Value: browser, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		target, _ := url.Parse(redirect)
		query := target.Query()
		query.Set("state", q.Get("state"))
		query.Set("code", code)
		target.RawQuery = query.Encode()
		http.Redirect(w, r, target.String(), http.StatusFound)
		return true
	case "/github/login/oauth/access_token":
		if r.Method != http.MethodPost || r.ParseForm() != nil || r.Form.Get("client_id") != "lifecycle-github" || r.Form.Get("client_secret") != "synthetic-github-secret" || r.Form.Get("grant_type") != "authorization_code" {
			fail(http.StatusUnauthorized)
			return true
		}
		hash := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		g.mu.Lock()
		code, exists := g.codes[r.Form.Get("code")]
		delete(g.codes, r.Form.Get("code"))
		if !exists || !time.Now().Before(code.Expires) || code.Redirect != r.Form.Get("redirect_uri") || r.Form.Get("code_verifier") == "" || code.Challenge != base64.RawURLEncoding.EncodeToString(hash[:]) {
			g.mu.Unlock()
			fail(http.StatusBadRequest)
			return true
		}
		token := rand.Text() + rand.Text()
		for key, value := range g.users {
			if time.Now().After(value.Expires) {
				delete(g.users, key)
			}
		}
		if len(g.users) >= 128 {
			g.mu.Unlock()
			fail(http.StatusTooManyRequests)
			return true
		}
		g.users[token] = lifecycleGitHubUserToken{Actor: code.Actor, Expires: time.Now().Add(time.Hour)}
		g.proof.OAuthExchangesVerified++
		g.mu.Unlock()
		lifecycleJSON(w, map[string]any{"access_token": token, "token_type": "bearer", "expires_in": 3600, "scope": ""})
		return true
	case "/github-api/user":
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		g.mu.Lock()
		account, exists := g.users[token]
		if r.Method != http.MethodGet || !exists || !time.Now().Before(account.Expires) {
			g.mu.Unlock()
			fail(http.StatusUnauthorized)
			return true
		}
		g.proof.OAuthUserReads++
		g.mu.Unlock()
		lifecycleJSON(w, map[string]any{"id": 5000 + account.Actor, "login": fmt.Sprintf("lifecycle-attendee-%02d", account.Actor), "type": "User"})
		return true
	}
	return false
}
