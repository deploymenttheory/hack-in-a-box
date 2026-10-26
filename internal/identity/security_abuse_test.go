package identity_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

// Exercise the real discovery/JWKS/code/PKCE verifier and PostgreSQL sessions.
// No forged session identity or authentication-bypass route is used.
func TestSecurity121OIDCTokenBrowserAndSessionAbuse(t *testing.T) {
	pool := testdb.New(t, true)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	sign := func(algorithm jose.SignatureAlgorithm, key any) jose.Signer {
		t.Helper()
		s, err := jose.NewSigner(jose.SigningKey{Algorithm: algorithm, Key: key}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "security121"))
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	trusted, wrongKey, symmetric := sign(jose.RS256, key), sign(jose.RS256, foreign), sign(jose.HS256, []byte(strings.Repeat("synthetic-key-", 4)))
	var issuer string
	var mu sync.Mutex
	var nonce, challenge, mode string
	var exchanges int
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys", "response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "security121", Algorithm: "RS256", Use: "sig"}}})
		case "/token":
			mu.Lock()
			defer mu.Unlock()
			exchanges++
			if err := r.ParseForm(); err != nil {
				w.WriteHeader(400)
				return
			}
			hash := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if base64.RawURLEncoding.EncodeToString(hash[:]) != challenge {
				t.Error("native PKCE verifier was not bound to this browser login")
				w.WriteHeader(400)
				return
			}
			claims := map[string]any{"iss": issuer, "sub": "security121-person", "aud": "security121-client", "exp": time.Now().Add(time.Minute).Unix(), "iat": time.Now().Unix(), "nonce": nonce, "email": "synthetic@example.invalid", "name": "Synthetic security reader"}
			signer := trusted
			switch mode {
			case "issuer":
				claims["iss"] = "https://untrusted.invalid"
			case "audience":
				claims["aud"] = "another-client"
			case "multiple_audiences_without_azp":
				claims["aud"] = []string{"security121-client", "another-client"}
			case "wrong_authorized_party":
				claims["azp"] = "another-client"
			case "expired":
				claims["exp"] = time.Now().Add(-time.Hour).Unix()
			case "missing_subject":
				delete(claims, "sub")
			case "missing_nonce":
				delete(claims, "nonce")
			case "wrong_nonce":
				claims["nonce"] = "different-login"
			case "foreign_signature":
				signer = wrongKey
			case "symmetric_algorithm":
				signer = symmetric
			case "second_identity":
				claims["sub"] = "security121-other-person"
			}
			payload, err := json.Marshal(claims)
			if err != nil {
				t.Error("encode synthetic claims")
				w.WriteHeader(500)
				return
			}
			signed, err := signer.Sign(payload)
			if err != nil {
				t.Error("sign synthetic claims")
				w.WriteHeader(500)
				return
			}
			raw, err := signed.CompactSerialize()
			if err != nil {
				t.Error("serialize synthetic claims")
				w.WriteHeader(500)
				return
			}
			if mode == "tampered_payload" {
				parts := strings.Split(raw, ".")
				claims["sub"] = "security121-forged-administrator"
				changed, _ := json.Marshal(claims)
				parts[1] = base64.RawURLEncoding.EncodeToString(changed)
				raw = strings.Join(parts, ".")
			}
			if mode == "unsigned" {
				raw = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)) + "." + base64.RawURLEncoding.EncodeToString(payload) + "."
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "SYNTHETIC-ACCESS-TOKEN-NEVER-RETAINED", "token_type": "Bearer", "id_token": raw, "expires_in": 60})
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(provider.Close)
	issuer = provider.URL
	const origin = "https://security121.example.invalid"
	auth, err := identity.New(t.Context(), pool, identity.Settings{Issuer: issuer, ClientID: "security121-client", Origin: origin, Secure: true})
	if err != nil {
		t.Fatal(err)
	}
	handler := auth.Sessions.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var e error
		switch r.URL.Path {
		case "/anonymous":
			auth.Sessions.Put(r.Context(), "anonymous-marker", "synthetic-fixation-attempt")
		case "/auth/login":
			e = auth.Login(w, r)
		case "/auth/callback":
			e = auth.Callback(w, r)
		default:
			_, e = auth.Current(r.Context())
			if e == nil && r.Method == http.MethodPost {
				e = auth.CSRF(r)
			}
		}
		if e != nil {
			http.Error(w, "authentication rejected", http.StatusUnauthorized)
		}
	}))
	request := func(method, path string, cookies []*http.Cookie, csrf, requestOrigin string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, nil)
		for _, c := range cookies {
			r.AddCookie(c)
		}
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		if requestOrigin != "" {
			r.Header.Set("Origin", requestOrigin)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	start := func(scenario string) (*url.URL, []*http.Cookie) {
		t.Helper()
		w := request("GET", "/auth/login", nil, "", "")
		if w.Code != 302 {
			t.Fatal("native login did not start")
		}
		u, err := url.Parse(w.Header().Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		mode, nonce, challenge = scenario, u.Query().Get("nonce"), u.Query().Get("code_challenge")
		mu.Unlock()
		if nonce == "" || challenge == "" || u.Query().Get("code_challenge_method") != "S256" {
			t.Fatal("missing native nonce/PKCE")
		}
		return u, w.Result().Cookies()
	}
	for _, scenario := range []string{"issuer", "audience", "multiple_audiences_without_azp", "wrong_authorized_party", "expired", "missing_subject", "missing_nonce", "wrong_nonce", "foreign_signature", "symmetric_algorithm", "tampered_payload", "unsigned", "missing_browser", "wrong_browser", "wrong_state", "expired_state"} {
		t.Run(scenario, func(t *testing.T) {
			u, cookies := start(scenario)
			state := u.Query().Get("state")
			switch scenario {
			case "missing_browser":
				cookies = nil
			case "wrong_browser":
				cookies = []*http.Cookie{{Name: "hackbox_login", Value: "different-browser"}}
			case "wrong_state":
				state = "unknown-state"
			case "expired_state":
				if _, err := pool.Exec(t.Context(), "UPDATE login_attempts SET expires_at=now()-interval '1 hour'"); err != nil {
					t.Fatal(err)
				}
			}
			path := "/auth/callback?code=synthetic&state=" + url.QueryEscape(state)
			before := exchanges
			w := request("GET", path, cookies, "", "")
			if w.Code != 401 {
				t.Fatal("untrusted token/browser established authentication", scenario)
			}
			if scenario == "missing_browser" || scenario == "wrong_browser" || scenario == "wrong_state" || scenario == "expired_state" {
				if exchanges != before {
					t.Fatal("untrusted browser/state reached provider exchange")
				}
			} else if request("GET", path, cookies, "", "").Code != 401 || exchanges != before+1 {
				t.Fatal("failed token attempt was replayable")
			}
			var people int
			if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM people").Scan(&people); err != nil || people != 0 {
				t.Fatal("rejected authentication created a person", err)
			}
		})
	}
	// Positive control: the same real verifier admits a signed token, rotates a
	// pre-existing anonymous session, and binds CSRF to the new session/origin.
	old := request("GET", "/anonymous", nil, "", "").Result().Cookies()
	u, cookies := start("valid")
	callback := append(append([]*http.Cookie{}, cookies...), old...)
	w := request("GET", "/auth/callback?code=synthetic&state="+url.QueryEscape(u.Query().Get("state")), callback, "", "")
	if w.Code != 303 {
		t.Fatal("trusted signed positive control rejected")
	}
	var token string
	for _, c := range w.Result().Cookies() {
		if c.Name == "hackbox_session" {
			token = c.Value
			if !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
				t.Fatal("unsafe authenticated cookie")
			}
		}
	}
	if token == "" || len(old) == 0 || token == old[0].Value {
		t.Fatal("authentication did not rotate anonymous cookie")
	}
	if request("GET", "/private", old, "", "").Code != 401 {
		t.Fatal("fixed anonymous cookie acquired new authentication")
	}
	current := []*http.Cookie{{Name: "hackbox_session", Value: token}}
	ctx, err := auth.Sessions.Load(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	csrf := auth.Sessions.GetString(ctx, "csrf")
	if len(csrf) < 32 || request("POST", "/private", current, csrf, origin).Code != 200 {
		t.Fatal("authenticated positive CSRF control failed")
	}
	for _, attempt := range [][2]string{{"", origin}, {"wrong-token", origin}, {csrf, "https://attacker.invalid"}, {csrf, origin + ".attacker.invalid"}, {csrf, "null"}, {csrf, ""}} {
		if request("POST", "/private", current, attempt[0], attempt[1]).Code != 401 {
			t.Fatal("session mutation accepted a mismatched CSRF/origin")
		}
	}
	person, err := auth.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := dbgen.New(pool).RevokeSessions(t.Context(), person.ID); err != nil {
		t.Fatal(err)
	}
	if request("GET", "/private", current, "", "").Code != 401 {
		t.Fatal("current session revision did not revoke cookie")
	}
	u, cookies = start("second_identity")
	if request("GET", "/auth/callback?code=synthetic&state="+url.QueryEscape(u.Query().Get("state")), cookies, "", "").Code != 303 {
		t.Fatal("second subject positive control failed")
	}
	var people int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM people").Scan(&people); err != nil || people != 2 {
		t.Fatal("same email linked distinct OIDC subjects", err)
	}
}
