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
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestOIDCStatePKCENonceAudienceAndRevocation(t *testing.T) {
	pool := testdb.New(t, true)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "test"),
	)
	if err != nil {
		t.Fatal(err)
	}
	var issuer, nonce, challenge string
	audience := "hackbox-test"
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).
				Encode(map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys", "response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			json.NewEncoder(w).
				Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "test", Algorithm: "RS256", Use: "sig"}}})
		case "/token":
			r.ParseForm()
			hashed := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if base64.RawURLEncoding.EncodeToString(hashed[:]) != challenge {
				t.Error("PKCE verifier missing or invalid")
				w.WriteHeader(400)
				return
			}
			payload, _ := json.Marshal(
				map[string]any{
					"iss":   issuer,
					"sub":   "stable-subject",
					"aud":   audience,
					"exp":   time.Now().Add(time.Minute).Unix(),
					"iat":   time.Now().Unix(),
					"nonce": nonce,
					"name":  "Synthetic Person",
					"email": "same@example.test",
				},
			)
			signed, _ := signer.Sign(payload)
			raw, _ := signed.CompactSerialize()
			json.NewEncoder(w).
				Encode(map[string]any{"access_token": "synthetic-never-persisted", "token_type": "Bearer", "id_token": raw, "expires_in": 60})
		default:
			w.WriteHeader(404)
		}
	}))
	defer fixture.Close()
	issuer = fixture.URL
	auth, err := identity.New(
		t.Context(),
		pool,
		identity.Settings{
			Issuer:           issuer,
			ClientID:         "hackbox-test",
			Origin:           "http://hack.example.test",
			BootstrapSubject: "stable-subject",
			Secure:           true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	handler := auth.Sessions.LoadAndSave(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var err error
			if r.URL.Path == "/auth/login" {
				err = auth.Login(w, r)
			} else {
				err = auth.Callback(w, r)
			}
			if err != nil {
				http.Error(w, err.Error(), 401)
			}
		}),
	)
	for _, scenario := range []string{"bad_nonce", "bad_audience", "success"} {
		t.Run(scenario, func(t *testing.T) {
			start := httptest.NewRecorder()
			handler.ServeHTTP(
				start,
				httptest.NewRequest(
					"GET",
					"/auth/login?return_to=%2Fevents%2Fsynthetic%3Ftab%3Dstaff",
					nil,
				),
			)
			redirect, err := url.Parse(start.Header().Get("Location"))
			if err != nil {
				t.Fatal(err)
			}
			nonce = redirect.Query().Get("nonce")
			challenge = redirect.Query().Get("code_challenge")
			audience = "hackbox-test"
			if redirect.Query().Get("code_challenge_method") != "S256" || nonce == "" ||
				challenge == "" {
				t.Fatal("missing PKCE/nonce")
			}
			if scenario == "bad_nonce" {
				nonce = "wrong"
			}
			if scenario == "bad_audience" {
				audience = "wrong"
			}
			callback := httptest.NewRequest(
				"GET",
				"/auth/callback?code=test&state="+url.QueryEscape(redirect.Query().Get("state")),
				nil,
			)
			for _, cookie := range start.Result().Cookies() {
				callback.AddCookie(cookie)
			}
			result := httptest.NewRecorder()
			handler.ServeHTTP(result, callback)
			expected := 401
			if scenario == "success" {
				expected = 303
			}
			if result.Code != expected {
				t.Fatalf("callback=%d %s", result.Code, result.Body)
			}
			replay := httptest.NewRecorder()
			handler.ServeHTTP(replay, callback.Clone(t.Context()))
			if replay.Code != 401 {
				t.Fatal("state replay accepted")
			}
			if scenario != "success" {
				return
			}
			if result.Header().Get("Location") != "/events/synthetic?tab=staff" {
				t.Fatal("validated login destination was lost")
			}
			var token string
			for _, cookie := range result.Result().Cookies() {
				if cookie.Name == "hackbox_session" {
					token = cookie.Value
					if !cookie.HttpOnly || !cookie.Secure ||
						cookie.SameSite != http.SameSiteLaxMode {
						t.Fatal("unsafe session cookie")
					}
				}
			}
			if token == "" {
				t.Fatal("no session cookie")
			}
			ctx, err := auth.Sessions.Load(t.Context(), token)
			if err != nil {
				t.Fatal(err)
			}
			person, err := auth.Current(ctx)
			if len(auth.Sessions.GetString(ctx, "csrf")) < 32 {
				t.Fatal("CSRF token does not satisfy the API contract")
			}
			if err != nil || !person.PlatformAdmin {
				t.Fatalf("bootstrap identity: %+v %v", person, err)
			}
			request := httptest.NewRequest("POST", "/api/v1/logout", nil).WithContext(ctx)
			request.Header.Set("Origin", "http://hack.example.test")
			request.Header.Set("X-CSRF-Token", auth.Sessions.GetString(ctx, "csrf"))
			if err := auth.CSRF(request); err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Origin", "https://attacker.test")
			if err := auth.CSRF(request); err == nil {
				t.Fatal("cross-site request accepted")
			}
			if err := dbgen.New(pool).RevokeSessions(ctx, person.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := auth.Current(ctx); err == nil {
				t.Fatal("revoked session accepted")
			}
			if err := auth.Logout(ctx); err != nil {
				t.Fatal(err)
			}
			_, found, err := (identity.Store{Queries: dbgen.New(pool)}).FindCtx(ctx, token)
			if err != nil || found {
				t.Fatalf("logout retained server session: %v %v", found, err)
			}
		})
	}
	// Session hashes, not bearer cookies, are the database lookup keys.
	store := identity.Store{Queries: dbgen.New(pool)}
	if err := store.CommitCtx(
		t.Context(),
		"raw-secret-cookie",
		[]byte("data"),
		time.Now().Add(time.Hour),
	); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := pool.QueryRow(t.Context(), "SELECT token FROM sessions LIMIT 1").
		Scan(&stored); err != nil ||
		strings.Contains(stored, "raw-secret") {
		t.Fatalf("raw cookie stored: %v", err)
	}
}
