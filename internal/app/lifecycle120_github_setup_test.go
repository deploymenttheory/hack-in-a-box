package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
)

// Configuration and attachment use the actual encrypted native App owner and
// its single-use organiser/browser-session callback, never direct database setup.
func (r *lifecycleJourney) configureGitHubApp(t *testing.T, item *lifecycleJourneyEvent) {
	t.Helper()
	ctx := t.Context()
	directory := os.Getenv("HACKBOX_LIFECYCLE_CONTROL")
	private, err := os.ReadFile(filepath.Join(directory, "github-app.pem"))
	if err != nil {
		t.Fatal("isolated provider App key unavailable")
	}
	defer clear(private)
	webhook, err := os.ReadFile(filepath.Join(directory, "github-webhook-secret"))
	if err != nil {
		t.Fatal("isolated provider webhook secret unavailable")
	}
	defer clear(webhook)
	base := "/api/v1/events/" + item.view.ID.String() + "/github-app"
	var status githubapp.Status
	var configured json.RawMessage
	input := map[string]any{
		"action":              "configure",
		"app_id":              lifecycleGitHubAppID,
		"app_slug":            lifecycleGitHubAppSlug,
		"organization_id":     lifecycleGitHubOrganizationID,
		"organization":        lifecycleGitHubOrganization,
		"template_repository": lifecycleGitHubTemplate,
		"private_key":         string(private),
		"webhook_secret":      string(webhook),
	}
	if err := r.owner.call(
		ctx,
		http.MethodPost,
		base,
		item.view.Revision,
		input,
		&configured,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	for _, privateMarker := range [][]byte{[]byte("PRIVATE KEY"), webhook, []byte("\"private_key\""), []byte("\"key_id\""), []byte("\"encrypted\"")} {
		if bytes.Contains(configured, privateMarker) {
			t.Fatal("native App configuration leaked private credential material")
		}
	}
	if err := json.Unmarshal(configured, &status); err != nil {
		t.Fatal(err)
	}
	if !status.Configured || !status.WebhookConfigured || status.InstallationID != 0 ||
		status.Diagnostics.Code != "installation_required" {
		t.Fatal("native App configuration did not require a separately verified installation")
	}
	var authorization struct {
		URL string `json:"url"`
	}
	if err := r.owner.call(
		ctx,
		http.MethodPost,
		base+"/installation-authorizations",
		status.Revision,
		nil,
		&authorization,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	target, err := url.Parse(authorization.URL)
	if err != nil || target.Scheme != "https" || target.Host != "github.com" ||
		target.Path != "/apps/"+lifecycleGitHubAppSlug+"/installations/new" ||
		len(target.Query().Get("state")) < 32 {
		t.Fatal("native App owner did not return its bound installation authorization")
	}
	transport, err := newLifecycleProviderTransport()
	if err != nil {
		t.Fatal(err)
	}
	previous := r.owner.http.Transport
	r.owner.http.Transport = transport
	defer func() { r.owner.http.Transport = previous; transport.base.CloseIdleConnections() }()
	callback, err := r.owner.redirect(ctx, target.String(), http.StatusFound)
	if err != nil {
		t.Fatal(err)
	}
	next, err := url.Parse(callback)
	if err != nil ||
		next.Scheme+"://"+next.Host+next.Path != r.owner.origin+"/auth/github-app/callback" ||
		next.Query().Get("state") != target.Query().Get("state") {
		t.Fatal("synthetic App did not return the exact native callback state")
	}
	if _, err := r.owner.redirect(ctx, callback, http.StatusSeeOther); err != nil {
		t.Fatal(err)
	}
	if err := r.owner.call(
		ctx,
		http.MethodGet,
		next.RequestURI(),
		-1,
		nil,
		nil,
		http.StatusBadRequest,
	); err != nil {
		t.Fatal(err)
	}
	if err := r.owner.call(ctx, http.MethodGet, base, -1, nil, &status, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if status.InstallationID != lifecycleGitHubInstallationID ||
		status.Diagnostics.Code != "healthy" ||
		status.Configuration == nil ||
		status.Configuration.AppID != lifecycleGitHubAppID ||
		!status.WebhookConfigured {
		t.Fatal(
			"native App owner did not verify synthetic JWT/scoped REST permissions and template",
		)
	}
	item.github = status
	var current event.View
	if err := r.owner.call(
		ctx,
		http.MethodGet,
		"/api/v1/events/"+item.view.ID.String(),
		-1,
		nil,
		&current,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	item.view = current
}
