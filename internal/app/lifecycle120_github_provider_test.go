package app

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
)

const (
	lifecycleGitHubAppID          = 42
	lifecycleGitHubInstallationID = 7
	lifecycleGitHubOrganizationID = 99
	lifecycleGitHubAppSlug        = "lifecycle-synthetic"
	lifecycleGitHubOrganization   = "lifecycle-synthetic-org"
	lifecycleGitHubTemplate       = "lifecycle-template"
)

type (
	lifecycleGitHubToken struct {
		Repositories map[string]bool
		Permissions  map[string]string
		Expires      time.Time
	}
	lifecycleGitHubProof struct {
		AppJWTsVerified          int `json:"app_jwts_verified"`
		InstallationTokensMinted int `json:"installation_tokens_minted"`
		TemplateReads            int `json:"template_reads"`
		RepositoryCreates        int `json:"repository_creates"`
		RepositoryReads          int `json:"repository_reads"`
		OAuthExchangesVerified   int `json:"oauth_exchanges_verified"`
		OAuthUserReads           int `json:"oauth_user_reads"`
	}
	// This is an external-provider fixture only. It cannot create competition facts
	// or report native owner success. Actual clients must present verified wall-time
	// App JWTs and repository-scoped installation tokens for their REST requests.
	lifecycleGitHub struct {
		key          *rsa.PrivateKey
		mu           sync.Mutex
		tokens       map[string]lifecycleGitHubToken
		proof        lifecycleGitHubProof
		repositories map[string]githubapp.Repository
		codes        map[string]lifecycleGitHubCode
		users        map[string]lifecycleGitHubUserToken
	}
)

func newLifecycleGitHub(directory string) (*lifecycleGitHub, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	encoded := pem.EncodeToMemory(
		&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)},
	)
	if err := os.WriteFile(filepath.Join(directory, "github-app.pem"), encoded, 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(
		filepath.Join(directory, "github-webhook-secret"),
		[]byte(rand.Text()+rand.Text()),
		0o600,
	); err != nil {
		return nil, err
	}
	return &lifecycleGitHub{key: key, tokens: map[string]lifecycleGitHubToken{}, repositories: map[string]githubapp.Repository{}, codes: map[string]lifecycleGitHubCode{}, users: map[string]lifecycleGitHubUserToken{}}, nil
}

func (g *lifecycleGitHub) appJWT(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") || len(auth) > 8192 {
		return false
	}
	signed, err := jose.ParseSigned(
		strings.TrimPrefix(auth, "Bearer "),
		[]jose.SignatureAlgorithm{jose.RS256},
	)
	if err != nil {
		return false
	}
	payload, err := signed.Verify(&g.key.PublicKey)
	if err != nil {
		return false
	}
	var claims struct {
		Issuer  json.RawMessage `json:"iss"`
		Issued  int64           `json:"iat"`
		Expires int64           `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return false
	}
	now := time.Now().Unix()
	if strings.Trim(string(claims.Issuer), "\"") != "42" || claims.Issued > now+30 ||
		claims.Issued < now-660 ||
		claims.Expires <= now ||
		claims.Expires > now+660 || claims.Expires-claims.Issued > 660 {
		return false
	}
	g.mu.Lock()
	g.proof.AppJWTsVerified++
	g.mu.Unlock()
	return true
}

func (g *lifecycleGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	fail := func(status int) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"message":"Synthetic provider request unavailable"}`)
	}
	if r.URL.Path == "/github/apps/"+lifecycleGitHubAppSlug+"/installations/new" {
		state := r.URL.Query().Get("state")
		if r.Method != http.MethodGet || len(state) < 32 || len(state) > 256 {
			fail(http.StatusBadRequest)
			return
		}
		// The synthetic App has a separately configured fixed admin callback, just
		// as a real App installation does. The native owner verifies one-use state.
		target := "http://admin:8080/auth/github-app/callback?installation_id=7&state=" + url.QueryEscape(
			state,
		)
		http.Redirect(w, r, target, http.StatusFound)
		return
	}
	if g.oauthHTTP(w, r) {
		return
	}
	switch r.URL.Path {
	case "/github-api/app":
		if r.Method != http.MethodGet || !g.appJWT(r) {
			fail(http.StatusUnauthorized)
			return
		}
		lifecycleJSON(w, map[string]any{"id": lifecycleGitHubAppID, "slug": lifecycleGitHubAppSlug})
	case "/github-api/app/installations/7":
		if r.Method != http.MethodGet || !g.appJWT(r) {
			fail(http.StatusUnauthorized)
			return
		}
		lifecycleJSON(
			w,
			map[string]any{
				"id":     lifecycleGitHubInstallationID,
				"app_id": lifecycleGitHubAppID,
				"account": map[string]any{
					"id":    lifecycleGitHubOrganizationID,
					"login": lifecycleGitHubOrganization,
					"type":  "Organization",
				},
				"permissions": map[string]string{
					"contents":       "read",
					"metadata":       "read",
					"administration": "write",
				},
				"suspended_at": nil,
			},
		)
	case "/github-api/app/installations/7/access_tokens":
		if r.Method != http.MethodPost || !g.appJWT(r) {
			fail(http.StatusUnauthorized)
			return
		}
		var input struct {
			Repositories []string          `json:"repositories"`
			Permissions  map[string]string `json:"permissions"`
		}
		decoder := json.NewDecoder(io.LimitReader(r.Body, 4096))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || !g.allowedToken(input.Repositories, input.Permissions) {
			fail(http.StatusUnprocessableEntity)
			return
		}
		token := rand.Text() + rand.Text()
		expires := time.Now().UTC().Add(time.Hour)
		g.mu.Lock()
		g.tokens[token] = lifecycleGitHubToken{
			Repositories: map[string]bool{input.Repositories[0]: true},
			Permissions:  input.Permissions,
			Expires:      expires,
		}
		g.proof.InstallationTokensMinted++
		g.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		lifecycleJSON(
			w,
			map[string]any{
				"token":                token,
				"expires_at":           expires,
				"permissions":          input.Permissions,
				"repository_selection": "selected",
			},
		)
	case "/github-api/repos/" + lifecycleGitHubOrganization + "/" + lifecycleGitHubTemplate:
		if r.Method != http.MethodGet ||
			!g.repositoryToken(r, lifecycleGitHubTemplate, "metadata", "read") {
			fail(http.StatusUnauthorized)
			return
		}
		g.mu.Lock()
		g.proof.TemplateReads++
		g.mu.Unlock()
		lifecycleJSON(
			w,
			map[string]any{
				"id":        100,
				"name":      lifecycleGitHubTemplate,
				"full_name": lifecycleGitHubOrganization + "/" + lifecycleGitHubTemplate,
				"owner": map[string]any{
					"id":    lifecycleGitHubOrganizationID,
					"login": lifecycleGitHubOrganization,
					"type":  "Organization",
				},
				"is_template":    true,
				"private":        true,
				"archived":       false,
				"disabled":       false,
				"default_branch": "main",
			},
		)
	default:
		if !g.repositoryHTTP(w, r) {
			fail(http.StatusNotFound)
		}
	}
}

func (g *lifecycleGitHub) repositoryToken(r *http.Request, name, permission, level string) bool {
	value := strings.TrimPrefix(r.Header.Get("Authorization"), "token ")
	if value == r.Header.Get("Authorization") {
		value = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	token, exists := g.tokens[value]
	return exists && time.Now().Before(token.Expires) && token.Repositories[name] &&
		token.Permissions[permission] == level
}

func (g *lifecycleGitHub) control(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.URL.Path != "/github/proof" {
		http.NotFound(w, r)
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	lifecycleJSON(w, g.proof)
}

// This bounded component check proves fixture authentication/protocol fidelity,
// not native App attachment, queued repositories or the integrated lifecycle.
func lifecycleGitHubRESTFixture(t *testing.T) (*lifecycleGitHub, *githubapp.RESTProvider, []byte, githubapp.Configuration) {
	t.Helper()
	directory := t.TempDir()
	fixture, err := newLifecycleGitHub(directory)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(fixture)
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport, err := newLifecycleProviderTransport()
	if err != nil {
		t.Fatal(err)
	}
	transport.base.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" || address != "provider:8080" {
			return nil, errors.New("unexpected fixture REST destination")
		}
		var dial net.Dialer
		return dial.DialContext(ctx, network, target.Host)
	}
	previous := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = previous; transport.base.CloseIdleConnections() })
	key, err := os.ReadFile(filepath.Join(directory, "github-app.pem"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { clear(key) })
	native := githubapp.NewProvider()
	configuration := githubapp.Configuration{
		AppID:              lifecycleGitHubAppID,
		AppSlug:            lifecycleGitHubAppSlug,
		OrganizationID:     lifecycleGitHubOrganizationID,
		Organization:       lifecycleGitHubOrganization,
		TemplateRepository: lifecycleGitHubTemplate,
	}
	return fixture, native, key, configuration
}

func TestLifecycleSyntheticGitHubNativeRESTContract(t *testing.T) {
	fixture, native, key, configuration := lifecycleGitHubRESTFixture(t)
	if value := native.Inspect(
		t.Context(),
		configuration,
		key,
		lifecycleGitHubInstallationID,
	); value.Code != "healthy" {
		t.Fatalf("native REST inspection: %s", value.Code)
	}
	fixture.mu.Lock()
	proof := fixture.proof
	fixture.mu.Unlock()
	if proof.AppJWTsVerified < 3 || proof.InstallationTokensMinted != 1 ||
		proof.TemplateReads != 1 {
		t.Fatal("native REST did not exercise verified JWT and scoped token ownership")
	}
	wrongKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	wrong := pem.EncodeToMemory(
		&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(wrongKey)},
	)
	defer clear(wrong)
	if value := native.Inspect(
		t.Context(),
		configuration,
		wrong,
		lifecycleGitHubInstallationID,
	); value.Code != "credentials_invalid" {
		t.Fatalf("unverified App JWT accepted: %s", value.Code)
	}
	unrelated := configuration
	unrelated.OrganizationID++
	if value := native.Inspect(
		t.Context(),
		unrelated,
		key,
		lifecycleGitHubInstallationID,
	); value.Code != "organization_mismatch" {
		t.Fatalf("native organization identity not checked: %s", value.Code)
	}
	outsideScope := configuration
	outsideScope.TemplateRepository = "uninstalled-template"
	if value := native.Inspect(
		t.Context(),
		outsideScope,
		key,
		lifecycleGitHubInstallationID,
	); value.Code != "template_inaccessible" {
		t.Fatalf("unselected repository token minted: %s", value.Code)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.proof.InstallationTokensMinted != proof.InstallationTokensMinted ||
		fixture.proof.TemplateReads != proof.TemplateReads {
		t.Fatal("rejected identity/scope created an additional provider token or template read")
	}
}

// The actual native REST client must create and subsequently look up one private
// template-derived provider resource. This does not provision a native team.
func TestLifecycleSyntheticGitHubRepositoryRESTContract(t *testing.T) {
	fixture, native, key, configuration := lifecycleGitHubRESTFixture(t)
	name := "hackbox-" + strings.ReplaceAll(uuid.NewString(), "-", "") + "-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	marker := "hackbox-provisioning:" + uuid.NewString()
	if _, err := native.LookupRepository(t.Context(), configuration, key, lifecycleGitHubInstallationID, name); !errors.Is(err, githubapp.ErrRepositoryAbsent) {
		t.Fatal("uncreated repository was not absent", err)
	}
	created, err := native.CreateRepository(t.Context(), configuration, key, lifecycleGitHubInstallationID, name, marker)
	if err != nil {
		t.Fatal("native template creation failed", err)
	}
	if created.ID <= 100 || created.Name != name || created.OrganizationID != lifecycleGitHubOrganizationID || !created.Private || created.Archived || created.TemplateID != 100 || created.Description != marker {
		t.Fatal("provider creation lost private template/provenance identity")
	}
	found, err := native.LookupRepository(t.Context(), configuration, key, lifecycleGitHubInstallationID, name)
	if err != nil || found != created {
		t.Fatal("native lookup did not recover exact created identity", err)
	}
	if _, err := native.CreateRepository(t.Context(), configuration, key, lifecycleGitHubInstallationID, name, marker); !errors.Is(err, githubapp.ErrRepositoryRejected) {
		t.Fatal("duplicate creation was not rejected", err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.proof.RepositoryCreates != 1 || fixture.proof.RepositoryReads != 1 {
		t.Fatal("repository effects were duplicated or lookup was not observed")
	}
}
