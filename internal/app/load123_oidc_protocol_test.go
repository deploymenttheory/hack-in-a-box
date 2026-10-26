package app

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
)

// These are external-provider protocol controls. Even 1000 known fixture
// subjects establish no application identity, session, activity or load proof.
func load123KnownPeople() map[string]lifecyclePerson {
	people := make(map[string]lifecyclePerson, 1000)
	for event := 1; event <= 5; event++ {
		for person := 1; person <= 200; person++ {
			subject := fmt.Sprintf("load-e%02d-p%03d", event, person)
			people[subject] = lifecyclePerson{Subject: subject, Name: "Synthetic " + subject, Email: subject + "@example.test"}
		}
	}
	return people
}

func load123KnownClients() map[string]string {
	return map[string]string{"load-public": "https://public.load.test/auth/callback", "load-admin": "https://admin.load.test/auth/callback"}
}

func TestLoad123OIDCExplicitThousandPeopleAreImmutableAndIsolated(t *testing.T) {
	people, clients := load123KnownPeople(), load123KnownClients()
	provider, err := newLifecycleOIDCForPeople("https://issuer.load.test", clients, people)
	if err != nil {
		t.Fatal(err)
	}
	delete(people, "load-e01-p001")
	people["unexpected"] = lifecyclePerson{Subject: "unexpected"}
	clients["load-public"] = "https://different.load.test/auth/callback"
	if len(provider.people) != 1000 || provider.people["load-e01-p001"].Subject != "load-e01-p001" ||
		provider.clients["load-public"] != "https://public.load.test/auth/callback" {
		t.Fatal("caller mutation changed the exact provider cohort/redirect set")
	}
	for _, forbidden := range []string{"owner", "attendee-01", "unexpected"} {
		response := load123Authorize(provider, forbidden, "load-public", rand.Text()+rand.Text(), rand.Text()+rand.Text())
		if response.Code != http.StatusBadRequest || len(provider.codes) != 0 {
			t.Fatal("explicit provider minted a code for another identity source")
		}
	}
	other, err := newLifecycleOIDCForPeople("https://other.load.test", load123KnownClients(), map[string]lifecyclePerson{
		"independent": {Subject: "independent", Name: "Synthetic independent", Email: "independent@example.test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(other.people) != 1 || len(provider.people) != 1000 || other.people["load-e01-p001"].Subject != "" || provider.people["independent"].Subject != "" {
		t.Fatal("separate explicit provider sources aliased or merged")
	}
}

func TestLoad123OIDCDefault52RemainSeparateFromLoadCohort(t *testing.T) {
	provider, err := newLifecycleOIDC("https://issuer.load.test", load123KnownClients())
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.people) != 52 || provider.people["owner"].Subject != "owner" ||
		provider.people["attendee-40"].Subject != "attendee-40" || provider.people["attendee-41"].Subject != "" ||
		provider.people["load-e01-p001"].Subject != "" {
		t.Fatal("load bridge changed the original bounded lifecycle defaults")
	}
}

func TestLoad123OIDCRejectsAmbiguousExplicitIdentityInputs(t *testing.T) {
	for name, people := range map[string]map[string]lifecyclePerson{
		"empty":            {},
		"subject-mismatch": {"one": {Subject: "two", Name: "Synthetic", Email: "one@example.test"}},
		"empty-name":       {"one": {Subject: "one", Email: "one@example.test"}},
		"duplicate-normalised-mailbox": {
			"one": {Subject: "one", Name: "Synthetic one", Email: "shared@example.test"},
			"two": {Subject: "two", Name: "Synthetic two", Email: " SHARED@example.test "},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := newLifecycleOIDCForPeople("https://issuer.load.test", load123KnownClients(), people); err == nil {
				t.Fatal("ambiguous external issuer identity input accepted")
			}
		})
	}
}

func load123Authorize(provider *lifecycleOIDC, subject, client, verifier, nonce string) *httptest.ResponseRecorder {
	digest := sha256.Sum256([]byte(verifier))
	query := url.Values{"client_id": {client}, "redirect_uri": {provider.clients[client]},
		"response_type": {"code"}, "scope": {"openid profile email"}, "code_challenge_method": {"S256"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(digest[:])}, "state": {strings.Repeat("s", 32)},
		"nonce": {nonce}, "actor": {subject}}
	response := httptest.NewRecorder()
	provider.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/authorize?"+query.Encode(), nil))
	return response
}

func load123AuthorizationCode(t *testing.T, provider *lifecycleOIDC, verifier, nonce string) string {
	t.Helper()
	response := load123Authorize(provider, "load-e01-p001", "load-public", verifier, nonce)
	target, err := url.Parse(response.Header().Get("Location"))
	if err != nil || response.Code != http.StatusFound ||
		target.Scheme+"://"+target.Host+target.Path != provider.clients["load-public"] ||
		target.Query().Get("state") != strings.Repeat("s", 32) || target.Query().Get("code") == "" {
		t.Fatal("known actor did not use the exact native client/state redirect path")
	}
	return target.Query().Get("code")
}

func load123Token(provider *lifecycleOIDC, code, verifier, client, redirect, secret string) *httptest.ResponseRecorder {
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code},
		"code_verifier": {verifier}, "redirect_uri": {redirect}}
	request := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetBasicAuth(client, secret)
	response := httptest.NewRecorder()
	provider.ServeHTTP(response, request)
	return response
}

func TestLoad123OIDCRealRSAPKCESingleUseAndWallExpiry(t *testing.T) {
	provider, err := newLifecycleOIDCForPeople("https://issuer.load.test", load123KnownClients(), load123KnownPeople())
	if err != nil {
		t.Fatal(err)
	}
	verifier, nonce := rand.Text()+rand.Text(), rand.Text()+rand.Text()
	before := time.Now().Unix()
	code := load123AuthorizationCode(t, provider, verifier, nonce)
	response := load123Token(provider, code, verifier, "load-public", provider.clients["load-public"], "synthetic-lifecycle-secret")
	var token struct {
		IDToken string `json:"id_token"`
		Expires int    `json:"expires_in"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &token) != nil || token.Expires != 120 {
		t.Fatal("valid known-client PKCE token exchange failed")
	}
	signed, err := jose.ParseSigned(token.IDToken, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		t.Fatal("fixture token is not an actual RS256 signature")
	}
	raw, err := signed.Verify(provider.public.Key)
	if err != nil {
		t.Fatal("fixture RSA signature failed verification")
	}
	var claims struct {
		Issuer   string `json:"iss"`
		Subject  string `json:"sub"`
		Audience string `json:"aud"`
		Nonce    string `json:"nonce"`
		Email    string `json:"email"`
		IssuedAt int64  `json:"iat"`
		Expires  int64  `json:"exp"`
		Verified bool   `json:"email_verified"`
	}
	if json.Unmarshal(raw, &claims) != nil {
		t.Fatal("verified token claims invalid")
	}
	other, err := newLifecycleOIDCForPeople("https://different.load.test", load123KnownClients(), map[string]lifecyclePerson{"other": {Subject: "other", Name: "Synthetic other", Email: "other@example.test"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := signed.Verify(other.public.Key); err == nil {
		t.Fatal("different issuer RSA key verified the token")
	}
	if claims.Issuer != provider.issuer || claims.Subject != "load-e01-p001" || claims.Audience != "load-public" || claims.Nonce != nonce ||
		claims.Email != "load-e01-p001@example.test" || !claims.Verified || claims.IssuedAt < before || claims.IssuedAt > time.Now().Unix() ||
		claims.Expires < before+119 || claims.Expires > time.Now().Unix()+121 {
		t.Fatal("RSA-verified claims lost subject/client/nonce or real-wall expiry")
	}
	if replay := load123Token(provider, code, verifier, "load-public", provider.clients["load-public"], "synthetic-lifecycle-secret"); replay.Code != http.StatusBadRequest {
		t.Fatal("consumed authorization code was reused")
	}
	for _, kind := range []string{"verifier", "client", "redirect", "secret", "expired"} {
		t.Run(kind, func(t *testing.T) {
			code := load123AuthorizationCode(t, provider, verifier, nonce)
			candidateVerifier, client, redirect, secret := verifier, "load-public", provider.clients["load-public"], "synthetic-lifecycle-secret"
			switch kind {
			case "verifier":
				candidateVerifier = rand.Text() + rand.Text()
			case "client":
				client = "load-admin"
			case "redirect":
				redirect = provider.clients["load-admin"]
			case "secret":
				secret = "different-synthetic-secret"
			case "expired":
				provider.mu.Lock()
				value := provider.codes[code]
				value.Expires = time.Now().Add(-time.Second)
				provider.codes[code] = value
				provider.mu.Unlock()
			}
			if rejected := load123Token(provider, code, candidateVerifier, client, redirect, secret); rejected.Code != http.StatusBadRequest || len(provider.codes) != 0 {
				t.Fatal("mismatched or expired code was accepted or retained")
			}
		})
	}
}
