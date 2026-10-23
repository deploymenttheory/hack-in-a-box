package githubapp

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func syntheticKey(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(
		&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)},
	)
}

func TestProviderVerifiesAppOrganizationTemplateAndLeastPrivilegeTokens(t *testing.T) {
	key := syntheticKey(t)
	c := Configuration{
		AppID:              42,
		AppSlug:            "synthetic-app",
		OrganizationID:     99,
		Organization:       "synthetic-org",
		TemplateRepository: "synthetic-template",
	}
	tests := []struct {
		name, code                                      string
		accountID                                       int64
		accountType                                     string
		permission                                      string
		installationStatus, tokenStatus, templateStatus int
		template                                        bool
		appID                                           int64
		suspended                                       bool
	}{
		{
			name:        "healthy",
			code:        "healthy",
			accountID:   99,
			accountType: "Organization",
			permission:  "write",
			template:    true,
			appID:       42,
		},
		{
			name:        "wrong org",
			code:        "organization_mismatch",
			accountID:   100,
			accountType: "Organization",
			permission:  "write",
			template:    true,
			appID:       42,
		},
		{
			name:        "user install",
			code:        "organization_mismatch",
			accountID:   99,
			accountType: "User",
			permission:  "write",
			template:    true,
			appID:       42,
		},
		{
			name:        "wrong app",
			code:        "app_mismatch",
			accountID:   99,
			accountType: "Organization",
			permission:  "write",
			template:    true,
			appID:       43,
		},
		{
			name:        "missing invitations permission",
			code:        "permissions_missing",
			accountID:   99,
			accountType: "Organization",
			permission:  "read",
			template:    true,
			appID:       42,
		},
		{name: "removed", code: "installation_removed", installationStatus: 404, appID: 42},
		{
			name:        "suspended",
			code:        "installation_suspended",
			accountID:   99,
			accountType: "Organization",
			permission:  "write",
			template:    true,
			appID:       42,
			suspended:   true,
		},
		{
			name:        "token forbidden",
			code:        "permission_denied",
			accountID:   99,
			accountType: "Organization",
			permission:  "write",
			template:    true,
			appID:       42,
			tokenStatus: 403,
		},
		{
			name:           "template inaccessible",
			code:           "template_inaccessible",
			accountID:      99,
			accountType:    "Organization",
			permission:     "write",
			template:       true,
			appID:          42,
			templateStatus: 404,
		},
		{
			name:        "not a template",
			code:        "template_invalid",
			accountID:   99,
			accountType: "Organization",
			permission:  "write",
			template:    false,
			appID:       42,
		},
	}
	missingTemplate := tests[0]
	missingTemplate.name = "template not installed"
	missingTemplate.code = "template_inaccessible"
	missingTemplate.tokenStatus = 422
	rateLimited := tests[0]
	rateLimited.name = "token rate limit"
	rateLimited.code = "rate_limited"
	rateLimited.tokenStatus = 429
	tests = append(tests, missingTemplate, rateLimited)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := 0
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					switch r.URL.Path {
					case "/app":
						if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
							t.Error("missing app JWT")
						}
						fmt.Fprintf(w, `{"id":%d,"slug":"synthetic-app"}`, tt.appID)
					case "/app/installations/7":
						if tt.installationStatus != 0 {
							w.WriteHeader(tt.installationStatus)
							fmt.Fprint(w, `{"message":"SYNTHETIC-PROVIDER-SECRET"}`)
							return
						}
						suspended := "null"
						if tt.suspended {
							suspended = `"2026-01-01T00:00:00Z"`
						}
						fmt.Fprintf(
							w,
							`{"id":7,"app_id":42,"account":{"id":%d,"login":"synthetic-org","type":%q},"permissions":{"contents":"read","metadata":"read","administration":%q},"suspended_at":%s}`,
							tt.accountID,
							tt.accountType,
							tt.permission,
							suspended,
						)
					case "/app/installations/7/access_tokens":
						tokens++
						var input struct {
							Repositories []string          `json:"repositories"`
							Permissions  map[string]string `json:"permissions"`
						}
						if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
							t.Error(err)
						}
						if len(input.Repositories) != 1 ||
							input.Repositories[0] != "synthetic-template" ||
							len(input.Permissions) != 1 ||

							input.Permissions["metadata"] != "read" {
							t.Errorf("unbounded token scope: %+v", input)
						}
						if tt.tokenStatus != 0 {
							w.WriteHeader(tt.tokenStatus)
							fmt.Fprint(w, `{"message":"SYNTHETIC-PROVIDER-SECRET"}`)
							return
						}
						fmt.Fprintf(
							w,
							`{"token":"SYNTHETIC-INSTALLATION-TOKEN","expires_at":%q}`,
							time.Now().Add(time.Hour).Format(time.RFC3339),
						)
					case "/repos/synthetic-org/synthetic-template":
						if !strings.Contains(
							r.Header.Get("Authorization"),
							"SYNTHETIC-INSTALLATION-TOKEN",
						) {
							t.Error("missing scoped token")
						}
						if tt.templateStatus != 0 {
							w.WriteHeader(tt.templateStatus)
							fmt.Fprint(w, `{"message":"SYNTHETIC-PROVIDER-SECRET"}`)
							return
						}
						fmt.Fprintf(
							w,
							`{"id":5,"name":"synthetic-template","is_template":%t,"owner":{"id":99,"login":"synthetic-org"}}`,
							tt.template,
						)
					default:
						t.Errorf("unexpected provider request %s", r.URL.Path)
						http.NotFound(w, r)
					}
				}),
			)
			defer server.Close()
			p := &RESTProvider{transport: server.Client().Transport, baseURL: server.URL + "/"}
			result := p.Inspect(t.Context(), c, key, 7)
			if result.Code != tt.code {
				t.Fatalf("got %q want %q", result.Code, tt.code)
			}
			data, _ := json.Marshal(result)
			if strings.Contains(string(data), "SECRET") ||
				strings.Contains(string(data), "TOKEN") ||
				strings.Contains(string(data), "PRIVATE KEY") {
				t.Fatal("provider payload leaked")
			}
			if tokens > 1 {
				t.Fatal("token unnecessarily refreshed")
			}
		})
	}
}

func TestOperationTokensNeverDefaultToInstallationWideAccess(t *testing.T) {
	p := NewProvider()
	key := syntheticKey(t)
	c := Configuration{AppID: 42}
	for _, op := range []string{"metadata", "read", "create", "invite"} {
		if _, err := p.scopedClient(c, key, 7, nil, op); err == nil {
			t.Fatalf("unbounded %s token allowed", op)
		}
	}
	if _, err := p.scopedClient(c, key, 7, []string{"repo"}, "delete"); err == nil {
		t.Fatal("unsupported operation allowed")
	}
	checks := permissionChecks(
		map[string]string{"contents": "read", "repository_creation": "write"},
	)
	if !checks[1].Allowed || checks[2].Allowed {
		t.Fatal("new repository-creation permission overstates invitation authority")
	}
}
