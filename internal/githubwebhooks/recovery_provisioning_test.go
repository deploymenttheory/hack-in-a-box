package githubwebhooks

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/repositories"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
	"github.com/google/uuid"
)

type recoveryProvisionProvider struct {
	nativeProvider
	health    string
	uncertain bool
}

func (p *recoveryProvisionProvider) Inspect(context.Context, githubapp.Configuration, []byte, int64) githubapp.Diagnostics {
	return githubapp.Diagnostics{Code: p.health}
}
func (p *recoveryProvisionProvider) CreateRepository(ctx context.Context, c githubapp.Configuration, key []byte, installation int64, name, marker string) (githubapp.Repository, error) {
	repo, err := p.nativeProvider.CreateRepository(ctx, c, key, installation, name, marker)
	if err == nil && p.uncertain {
		return githubapp.Repository{}, context.DeadlineExceeded
	}
	return repo, err
}
func TestRecoveryProvisioningFaultsPreserveNativeClaimAndEncryptedAppBoundary(t *testing.T) {
	for _, fault := range []string{"installation_removed", "rate_limited", "uncertain_create", "unrelated_existing"} {
		t.Run(fault, func(t *testing.T) {
			f := setupFixture(t, false)
			member := testdb.Person(t, f.pool)
			f.exec("INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot)VALUES($1,$2,$3,'published')", f.event, f.team, member.ID)
			f.exec("INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')", f.event, member.ID)
			provider := &recoveryProvisionProvider{health: "healthy"}
			f.app.Provider = provider
			owner := repositories.Service{Pool: f.pool, App: f.app}
			queue, err := jobs.New(f.pool, slog.New(slog.NewTextHandler(io.Discard, nil)), map[string]jobs.Handler{repositories.HandlerKind: owner})
			if err != nil {
				t.Fatal(err)
			}
			if fault == "installation_removed" || fault == "rate_limited" {
				provider.health = fault
			}
			if fault == "uncertain_create" {
				provider.uncertain = true
			}
			if fault == "unrelated_existing" {
				provider.created = githubapp.Repository{ID: 123, OrganizationID: 99, Name: repositories.Name(f.event, f.team), Private: true, TemplateID: 700, Description: "unrelated-existing-resource"}
			}
			event, err := dbgen.New(f.pool).Event(t.Context(), f.event)
			if err != nil {
				t.Fatal(err)
			}
			if err = owner.Queue(t.Context(), f.owner, f.event, event.Revision); err != nil {
				t.Fatal(err)
			}
			var operation uuid.UUID
			if err = f.pool.QueryRow(t.Context(), "SELECT id FROM operations WHERE event_id=$1 AND target_id=$2 AND kind=$3", f.event, f.team, repositories.HandlerKind).Scan(&operation); err != nil {
				t.Fatal(err)
			}
			err = queue.Execute(t.Context(), operation)
			if fault == "installation_removed" || fault == "rate_limited" {
				if !errors.Is(err, jobs.ErrRetry) {
					t.Fatal("revoked/rate-limited App not retryable", err)
				}
				row, e := (repositories.Reader{Pool: f.pool}).ForTeam(t.Context(), f.event, f.team)
				if e != nil || row.Attempted || row.GithubRepositoryID.Valid || provider.calls != 0 {
					t.Fatal("unavailable App claimed or created resource", row, e)
				}
				provider.health = "healthy"
				if err = queue.Execute(t.Context(), operation); err != nil {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			row, err := (repositories.Reader{Pool: f.pool}).ForTeam(t.Context(), f.event, f.team)
			if err != nil {
				t.Fatal(err)
			}
			if fault == "unrelated_existing" {
				if row.GithubRepositoryID.Valid || row.Diagnostic != "name_collision" || provider.calls != 0 {
					t.Fatal("claimed unrelated pre-existing repository", row)
				}
				return
			}
			if fault == "uncertain_create" {
				if row.State != "ambiguous" || row.GithubRepositoryID.Valid || !row.Attempted || provider.calls != 1 {
					t.Fatal("uncertain create looked ready", row)
				}
				if err = queue.Execute(t.Context(), operation); err != nil {
					t.Fatal(err)
				}
				event, err = dbgen.New(f.pool).Event(t.Context(), f.event)
				if err != nil {
					t.Fatal(err)
				}
				if err = owner.Recover(t.Context(), f.owner, f.event, f.team, event.Revision); err != nil {
					t.Fatal(err)
				}
			}
			row, err = (repositories.Reader{Pool: f.pool}).ForTeam(t.Context(), f.event, f.team)
			if err != nil || row.State != "ready" || !row.GithubRepositoryID.Valid || provider.calls != 1 {
				t.Fatal("native recovery failed or duplicated create", row, err)
			}
			provenance := row.Provenance
			event, err = dbgen.New(f.pool).Event(t.Context(), f.event)
			if err != nil {
				t.Fatal(err)
			}
			if err = owner.Queue(t.Context(), f.owner, f.event, event.Revision); err != nil {
				t.Fatal(err)
			}
			if err = queue.Execute(t.Context(), operation); err != nil {
				t.Fatal(err)
			}
			current, err := (repositories.Reader{Pool: f.pool}).ForTeam(t.Context(), f.event, f.team)
			if err != nil || current.Provenance != provenance || provider.calls != 1 {
				t.Fatal("retry changed ownership or repeated create", current, err)
			}
		})
	}
}
