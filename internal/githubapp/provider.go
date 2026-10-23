// Package githubapp connects event organisers to organization-installed GitHub
// Apps. It is independent of participant identity linking and team secret reveal.
package githubapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	authgithub "github.com/google/go-github/v88/github"
	"github.com/google/go-github/v89/github"
)

var ErrIntegration = errors.New("GitHub App configuration could not be verified")

type Configuration struct {
	AppID              int64  `json:"app_id"`
	AppSlug            string `json:"app_slug"`
	OrganizationID     int64  `json:"organization_id"`
	Organization       string `json:"organization"`
	TemplateRepository string `json:"template_repository"`
}
type PermissionCheck struct {
	Operation string   `json:"operation"`
	Required  []string `json:"required"`
	Allowed   bool     `json:"allowed"`
}
type Diagnostics struct {
	Code      string            `json:"code"`
	Checks    []PermissionCheck `json:"checks"`
	CheckedAt time.Time         `json:"checked_at"`
}
type Provider interface {
	ValidateKey(appID int64, key []byte) bool
	Inspect(context.Context, Configuration, []byte, int64) Diagnostics
}

// RESTProvider uses go-github v89 for REST and ghinstallation v2.19.0 for
// authentication. ghinstallation's v88 option type is private to its transport;
// both majors interoperate through net/http. No provider error body is exposed.
type RESTProvider struct {
	transport http.RoundTripper
	baseURL   string
}

func NewProvider() *RESTProvider {
	return &RESTProvider{transport: http.DefaultTransport, baseURL: "https://api.github.com/"}
}

func (p *RESTProvider) ValidateKey(appID int64, key []byte) bool {
	_, err := ghinstallation.NewAppsTransport(p.transport, appID, key)
	return err == nil
}

func (p *RESTProvider) client(transport http.RoundTripper) (*github.Client, error) {
	client, err := github.NewClient(
		github.WithHTTPClient(
			&http.Client{
				Transport:     transport,
				Timeout:       10 * time.Second,
				CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
			},
		),
		github.WithURLs(&p.baseURL, nil),
	)
	if err != nil {
		return nil, ErrIntegration
	}
	return client, nil
}

func diagnostic(code string) Diagnostics {
	return Diagnostics{Code: code, Checks: []PermissionCheck{}, CheckedAt: time.Now().UTC()}
}

func providerFailure(response *github.Response, failure error) Diagnostics {
	if response == nil {
		var tokenFailure *ghinstallation.HTTPError
		if errors.As(failure, &tokenFailure) && tokenFailure.Response != nil {
			response = &github.Response{Response: tokenFailure.Response}
		}
	}
	if response == nil {
		return diagnostic("provider_unavailable")
	}
	switch response.StatusCode {
	case 401:
		return diagnostic("credentials_invalid")
	case 403:
		if (response.Rate.Limit > 0 && response.Rate.Remaining == 0) ||
			(response.Header.Get("X-RateLimit-Limit") != "" && response.Header.Get("X-RateLimit-Remaining") == "0") ||
			response.Header.Get("Retry-After") != "" {
			return diagnostic("rate_limited")
		}
		return diagnostic("permission_denied")
	case 404:
		return diagnostic("installation_removed")
	case 429:
		return diagnostic("rate_limited")
	default:
		return diagnostic("provider_unavailable")
	}
}

type installation struct {
	ID      int64 `json:"id"`
	AppID   int64 `json:"app_id"`
	Account struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"account"`
	Permissions map[string]string `json:"permissions"`
	SuspendedAt *time.Time        `json:"suspended_at"`
}

func permissionChecks(permissions map[string]string) []PermissionCheck {
	read := permissions["contents"] == "read" || permissions["contents"] == "write"
	admin := permissions["administration"] == "write"
	return []PermissionCheck{
		{
			Operation: "read_template",
			Required:  []string{"metadata:read"},
			Allowed:   true,
		},
		{
			Operation: "create_repository",
			Required: []string{
				"contents:read",
				"administration:write OR repository_creation:write",
			},
			Allowed: read && (admin || permissions["repository_creation"] == "write"),
		},
		{
			Operation: "manage_repository_invitations",
			Required:  []string{"administration:write"},
			Allowed:   admin,
		},
		{
			Operation: "read_repository_activity",
			Required:  []string{"contents:read", "metadata:read"},
			Allowed:   read,
		},
	}
}

func (p *RESTProvider) Inspect(
	ctx context.Context,
	c Configuration,
	key []byte,
	id int64,
) Diagnostics {
	tr, err := ghinstallation.NewAppsTransport(p.transport, c.AppID, key)
	if err != nil {
		return diagnostic("credentials_invalid")
	}
	tr.BaseURL = strings.TrimRight(p.baseURL, "/")
	client, err := p.client(tr)
	if err != nil {
		return diagnostic("provider_unavailable")
	}
	app, response, err := client.Apps.Get(ctx, "")
	if err != nil {
		return providerFailure(response, err)
	}
	if app.GetID() != c.AppID || app.GetSlug() != c.AppSlug {
		return diagnostic("app_mismatch")
	}
	req, err := client.NewRequest(ctx, "GET", fmt.Sprintf("app/installations/%d", id), nil)
	if err != nil {
		return diagnostic("provider_unavailable")
	}
	var record installation
	response, err = client.Do(req, &record)
	if err != nil {
		return providerFailure(response, err)
	}
	if record.ID != id || record.AppID != c.AppID {
		return diagnostic("app_mismatch")
	}
	if record.Account.ID != c.OrganizationID || record.Account.Type != "Organization" ||
		!strings.EqualFold(record.Account.Login, c.Organization) {
		return diagnostic("organization_mismatch")
	}
	if record.SuspendedAt != nil {
		return diagnostic("installation_suspended")
	}
	result := diagnostic("healthy")
	result.Checks = permissionChecks(record.Permissions)
	for _, check := range result.Checks {
		if !check.Allowed {
			result.Code = "permissions_missing"
		}
	}
	if !result.Checks[0].Allowed {
		return result
	}
	scoped, err := p.scopedClient(c, key, id, []string{c.TemplateRepository}, "metadata")
	if err != nil {
		return diagnostic("credentials_invalid")
	}
	repo, response, err := scoped.Repositories.Get(ctx, c.Organization, c.TemplateRepository)
	if err != nil {
		var tokenFailure *ghinstallation.HTTPError
		if errors.As(err, &tokenFailure) && tokenFailure.Response != nil &&
			tokenFailure.Response.StatusCode == 422 {
			return diagnostic("template_inaccessible")
		}
		d := providerFailure(response, err)
		if response != nil && response.StatusCode == 404 {
			d.Code = "template_inaccessible"
		}
		return d
	}
	if repo.GetID() <= 0 || repo.GetOwner().GetID() != c.OrganizationID ||
		!strings.EqualFold(repo.GetOwner().GetLogin(), c.Organization) ||
		!repo.GetIsTemplate() ||
		repo.GetArchived() ||
		repo.GetDisabled() {
		return diagnostic("template_invalid")
	}
	return result
}

// scopedClient mints only repository-specific, operation-specific tokens. The
// App owner supplies event-authorised repository names and rechecks live
// installation/permission state before asking for a write client. No token or
// authenticated client crosses a browser-facing service boundary.
func (p *RESTProvider) scopedClient(
	c Configuration,
	key []byte,
	id int64,
	repositories []string,
	operation string,
) (*github.Client, error) {
	if len(repositories) == 0 || len(repositories) > 500 || id <= 0 {
		return nil, ErrIntegration
	}
	permissions := &authgithub.InstallationPermissions{Metadata: new("read")}
	switch operation {
	case "metadata": // Metadata read is implicit for every installation.
	case "read":
		permissions.Contents = new("read")
	case "create":
		permissions.Contents = new("read")
		permissions.Administration = new("write")
	case "invite":
		permissions.Administration = new("write")
	default:
		return nil, ErrIntegration
	}
	tr, err := ghinstallation.New(p.transport, c.AppID, id, key)
	if err != nil {
		return nil, ErrIntegration
	}
	tr.BaseURL = strings.TrimRight(p.baseURL, "/")
	tr.InstallationTokenOptions = &authgithub.InstallationTokenOptions{
		Repositories: repositories,
		Permissions:  permissions,
	}
	// ghinstallation's refresh client uses the same bounded no-redirect policy.
	tr.Client = &http.Client{
		Transport:     p.transport,
		Timeout:       10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	return p.client(tr)
}
