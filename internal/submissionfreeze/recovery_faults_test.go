package submissionfreeze

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

// Fault only the external provider. Native encrypted credentials, current App
// binding, durable dispatch, final settlement and audited decisions stay real.
type recoveryBranchProvider struct {
	ownedBranchProvider
	health    string
	outage    bool
	inspected []byte
}

func (p *recoveryBranchProvider) Inspect(ctx context.Context, c githubapp.Configuration, key []byte, installation int64) githubapp.Diagnostics {
	p.inspected = key
	if p.health != "healthy" {
		return githubapp.Diagnostics{Code: p.health}
	}
	return p.ownedBranchProvider.Inspect(ctx, c, key, installation)
}
func (p *recoveryBranchProvider) ObserveBranch(ctx context.Context, c githubapp.Configuration, key []byte, installation int64, name string, repository int64, branch string) (githubapp.BranchObservation, error) {
	if p.outage {
		return githubapp.BranchObservation{}, context.DeadlineExceeded
	}
	return p.ownedBranchProvider.ObserveBranch(ctx, c, key, installation, name, repository, branch)
}
func TestRecoveryFaultsUseCurrentEncryptedAppAndNeverAutoVerifyLaterObservation(t *testing.T) {
	pool := testdb.New(t, true)
	for _, fault := range []string{"installation_removed", "rate_limited", "missing_permissions", "branch_outage"} {
		t.Run(fault, func(t *testing.T) {
			f := newFixture(t, pool)
			f.submit("repository")
			keys, err := secretbox.Parse(`{"active":"test","keys":{"test":"` + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32)) + `"}}`)
			if err != nil {
				t.Fatal(err)
			}
			provider := &recoveryBranchProvider{ownedBranchProvider: ownedBranchProvider{expected: []byte("SYNTHETIC-RECOVERY-APP-KEY")}, health: "healthy"}
			app := githubapp.Service{Pool: pool, Keys: keys, Provider: provider}
			row, err := f.q.Event(t.Context(), f.event)
			if err != nil {
				t.Fatal(err)
			}
			if err = app.Configure(t.Context(), f.owner, f.event, row.Revision, githubapp.Command{Action: "configure", Configuration: githubapp.Configuration{AppID: 42, AppSlug: "synthetic-app", OrganizationID: 99, Organization: "synthetic-org", TemplateRepository: "template"}, PrivateKey: string(provider.expected)}); err != nil {
				t.Fatal(err)
			}
			f.exec("UPDATE github_app_integrations SET installation_id=55 WHERE event_id=$1", f.event)
			f.service.App = app
			if fault == "branch_outage" {
				provider.outage = true
			} else {
				provider.health = fault
			}
			f.now = f.cfg.SubmissionCutoff
			operation := f.operation()
			if err = f.jobs.Execute(t.Context(), operation); !errors.Is(err, jobs.ErrRetry) {
				t.Fatal("fault must retain retryable capture", err)
			}
			failed := f.read(f.owner)
			if failed.Status != "unresolved" || failed.Current != nil || failed.NativeOutcome != "frozen" || len(failed.Observations) != 1 || failed.Observations[0].Status != "provider_unavailable" || provider.calls != 0 {
				t.Fatalf("fault falsely verified or bypassed current App: %+v", failed)
			}
			if !bytes.Equal(provider.inspected, make([]byte, len(provider.inspected))) {
				t.Fatal("borrowed App credential retained after failure")
			}
			source, _ := json.Marshal(failed.Source)
			provider.health = "healthy"
			provider.outage = false
			f.now = f.now.Add(time.Hour)
			if err = f.jobs.Execute(t.Context(), operation); err != nil {
				t.Fatal(err)
			}
			observed := f.read(f.owner)
			if observed.Status != "unresolved" || observed.Current != nil || len(observed.Observations) != 2 || provider.calls != 1 || !observed.Observations[1].ObservedAt.Equal(f.now) {
				t.Fatal("later recovery invented historical certainty", observed)
			}
			after, _ := json.Marshal(observed.Source)
			if !bytes.Equal(source, after) {
				t.Fatal("recovery replaced original cutoff source")
			}
			command := f.decision(observed, "reconcile")
			receipt, err := f.service.Change(t.Context(), f.owner, f.event, f.team, command)
			if err != nil {
				t.Fatal(err)
			}
			original, _ := json.Marshal(receipt)
			f.now = f.now.Add(time.Hour)
			f.capture()
			saved, err := f.service.OriginalReceipt(t.Context(), f.owner, f.event, f.team, command.RequestID)
			if err != nil || saved.Receipt == nil {
				t.Fatal(saved, err)
			}
			retained, _ := json.Marshal(saved.Receipt)
			if !bytes.Equal(original, retained) || provider.calls != 1 || f.read(f.owner).Current.SHA != strings.Repeat("a", 40) {
				t.Fatal("completed receipt changed or provider replayed")
			}
		})
	}
}
