package app

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
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
	}
	// This is an external-provider fixture only. It cannot create competition facts
	// or report native owner success. Actual clients must present verified wall-time
	// App JWTs and repository-scoped installation tokens for their REST requests.
	lifecycleGitHub struct {
		key    *rsa.PrivateKey
		mu     sync.Mutex
		tokens map[string]lifecycleGitHubToken
		proof  lifecycleGitHubProof
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
	return &lifecycleGitHub{key: key, tokens: map[string]lifecycleGitHubToken{}}, nil
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
		if decoder.Decode(&input) != nil || len(input.Repositories) != 1 ||
			input.Repositories[0] != lifecycleGitHubTemplate ||
			len(input.Permissions) != 1 ||
			input.Permissions["metadata"] != "read" {
			fail(http.StatusUnprocessableEntity)
			return
		}
		token := rand.Text() + rand.Text()
		expires := time.Now().UTC().Add(time.Hour)
		g.mu.Lock()
		g.tokens[token] = lifecycleGitHubToken{
			Repositories: map[string]bool{lifecycleGitHubTemplate: true},
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
		fail(http.StatusNotFound)
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
