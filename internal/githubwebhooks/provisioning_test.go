package githubwebhooks

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/repositories"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type nativeProvider struct {
	provider
	created githubapp.Repository
	calls   int
}

func (p *nativeProvider) LookupRepository(
	_ context.Context,
	c githubapp.Configuration,
	_ []byte,
	_ int64,
	name string,
) (githubapp.Repository, error) {
	if name == c.TemplateRepository {
		return githubapp.Repository{ID: 700, OrganizationID: c.OrganizationID}, nil
	}
	if p.created.Name == name {
		return p.created, nil
	}
	return githubapp.Repository{}, githubapp.ErrRepositoryAbsent
}

func (p *nativeProvider) CreateRepository(
	_ context.Context,
	c githubapp.Configuration,
	_ []byte,
	_ int64,
	name, marker string,
) (githubapp.Repository, error) {
	p.calls++
	p.created = githubapp.Repository{
		ID:             123,
		OrganizationID: c.OrganizationID,
		Name:           name,
		Description:    marker,
		Private:        true,
		TemplateID:     700,
	}
	return p.created, nil
}

func TestNativePublishedRepositoryProvisioningFeedsSignedActivity(t *testing.T) {
	f := setupFixture(t, false)
	ctx := t.Context()
	// Replace only fixture setup with the real92 queue/worker/App credential
	// path. The only mock is deterministic provider transport; no live GitHub call.
	member := testdb.Person(t, f.pool)
	f.exec(
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
		f.event,
		f.team,
		member.ID,
	)
	remote := &nativeProvider{}
	f.app.Provider = remote
	provisioner := repositories.Service{Pool: f.pool, App: f.app}
	worker, err := jobs.New(
		f.pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{repositories.HandlerKind: provisioner, HandlerKind: f.s},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.s.Jobs = worker
	event, err := dbgen.New(f.pool).Event(ctx, f.event)
	if err != nil {
		t.Fatal(err)
	}
	if err = provisioner.Queue(ctx, f.owner, f.event, event.Revision); err != nil {
		t.Fatal(err)
	}
	var operationID uuid.UUID
	if err = f.pool.QueryRow(ctx, "SELECT id FROM operations WHERE event_id=$1 AND kind=$2", f.event, repositories.HandlerKind).
		Scan(&operationID); err != nil {
		t.Fatal(err)
	}
	if err = worker.Execute(ctx, operationID); err != nil {
		t.Fatal(err)
	}
	mapped, err := (repositories.Reader{Pool: f.pool}).ByGitHubID(ctx, 123)
	if err != nil || mapped.State != "ready" || mapped.TeamID != f.team ||
		mapped.RequestedBy != f.owner ||
		remote.calls != 1 {
		t.Fatalf("native provenance not proven: %+v calls=%d %v", mapped, remote.calls, err)
	}
	id := uuid.New()
	raw := f.push(time.Now().Add(-time.Minute))
	if err = f.s.Receive(
		ctx,
		Headers{DeliveryID: id.String(), Event: "push", Signature: sign(raw, testSecret)},
		raw,
	); err != nil {
		t.Fatal(err)
	}
	f.execute(id)
	status, err := f.s.Status(ctx, f.owner, f.event)
	if err != nil || len(status.Activity) != 1 || status.Activity[0].Pushes != 1 {
		t.Fatalf("native registry activity: %+v %v", status, err)
	}
	// A deleted/recreated repository with the same name has no immutable mapping.
	changed := []byte(strings.Replace(string(raw), `"id":123`, `"id":124`, 1))
	if err = f.s.Receive(
		ctx,
		Headers{DeliveryID: uuid.NewString(), Event: "push", Signature: sign(changed, testSecret)},
		changed,
	); err == nil {
		t.Fatal("recreated repository inherited original identity")
	}
	current, err := (repositories.Reader{Pool: f.pool}).ForTeam(ctx, f.event, f.team)
	if err != nil || current.GithubRepositoryID.Int64 != 123 ||
		current.Provenance != mapped.Provenance ||
		current.State != "ready" {
		t.Fatal("activity changed native readiness/identity")
	}
	if remote.calls != 1 {
		t.Fatal("webhook made provider mutation")
	}
}
