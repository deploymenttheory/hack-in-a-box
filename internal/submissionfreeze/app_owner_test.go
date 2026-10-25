package submissionfreeze

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
	"strings"
	"testing"
)

type ownedBranchProvider struct {
	expected, borrowed []byte
	calls              int
	valid              bool
	repository         int64
	branch, name       string
}

func (p *ownedBranchProvider) ValidateKey(_ int64, key []byte) bool {
	return bytes.Equal(key, p.expected)
}
func (p *ownedBranchProvider) Inspect(_ context.Context, c githubapp.Configuration, key []byte, installation int64) githubapp.Diagnostics {
	if !bytes.Equal(key, p.expected) || c.AppID != 42 || c.OrganizationID != 99 || installation != 55 {
		return githubapp.Diagnostics{Code: "integration_unavailable"}
	}
	return githubapp.Diagnostics{Code: "healthy"}
}
func (p *ownedBranchProvider) ObserveBranch(_ context.Context, c githubapp.Configuration, key []byte, installation int64, name string, repository int64, branch string) (githubapp.BranchObservation, error) {
	p.borrowed = key
	p.valid = bytes.Equal(key, p.expected) && c.OrganizationID == 99 && installation == 55
	p.calls++
	p.repository = repository
	p.branch = branch
	p.name = name
	return githubapp.BranchObservation{SHA: strings.Repeat("a", 40), Status: "current_ref"}, nil
}
func TestNativeCutoffGraphUsesEncryptedAppOwnerAndClearsBorrowedCredentials(t *testing.T) {
	pool := testdb.New(t, true)
	f := newFixture(t, pool)
	f.submit("repository")
	keys, err := secretbox.Parse(`{"active":"test","keys":{"test":"` + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32)) + `"}}`)
	if err != nil {
		t.Fatal(err)
	}
	provider := &ownedBranchProvider{expected: []byte("SYNTHETIC-ONLY-APP-KEY")}
	app := githubapp.Service{Pool: pool, Keys: keys, Provider: provider}
	row, err := f.q.Event(t.Context(), f.event)
	if err != nil {
		t.Fatal(err)
	}
	err = app.Configure(t.Context(), f.owner, f.event, row.Revision, githubapp.Command{Action: "configure", Configuration: githubapp.Configuration{AppID: 42, AppSlug: "synthetic-app", OrganizationID: 99, Organization: "synthetic-org", TemplateRepository: "template"}, PrivateKey: string(provider.expected)})
	if err != nil {
		t.Fatal(err)
	}
	// Existing installation/provisioning handshakes are separate native owners;
	// this fixture records their already-proven synthetic installation fact.
	f.exec("UPDATE github_app_integrations SET installation_id=55 WHERE event_id=$1", f.event)
	f.service.App = app
	f.now = f.cfg.SubmissionCutoff
	f.capture()
	p := f.read(f.owner)
	if !provider.valid || provider.calls != 1 || provider.repository != f.repository || provider.branch != "main" || provider.name != "freeze-"+f.team.String() || len(p.Observations) != 1 || p.Observations[0].SHA != strings.Repeat("a", 40) || p.Status != "unresolved" {
		t.Fatal("native encrypted App graph did not preserve source identity")
	}
	if len(provider.borrowed) == 0 || !bytes.Equal(provider.borrowed, make([]byte, len(provider.borrowed))) {
		t.Fatal("App owner retained plaintext credentials after observation")
	}
	encoded, _ := json.Marshal(p)
	if bytes.Contains(encoded, provider.expected) || strings.Contains(string(encoded), "private_key") || strings.Contains(string(encoded), "encrypted") {
		t.Fatal("credentials escaped into source projection")
	}
}
