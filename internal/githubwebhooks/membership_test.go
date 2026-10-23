package githubwebhooks

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/repositoryaccess"
)

type currentMembershipProvider struct{ reads int }

func (p *currentMembershipProvider) InspectCollaborators(
	context.Context,
	*dbgen.Queries,
	uuid.UUID,
	githubapp.AccessRequest,
) (githubapp.AccessSnapshot, error) {
	p.reads++
	return githubapp.AccessSnapshot{
		Collaborators: []githubapp.Collaborator{
			{ID: 77, Login: "current-user", Permission: "write"},
		},
		Invitations: []githubapp.Invitation{},
	}, nil
}

func (*currentMembershipProvider) ChangeCollaborator(
	context.Context,
	*dbgen.Queries,
	uuid.UUID,
	githubapp.AccessRequest,
	githubapp.AccessChange,
) (githubapp.AccessResult, error) {
	panic("webhook hints must never dispatch provider mutations")
}

func TestAuthenticatedMemberHintsReconcileThroughCurrentProviderOnly(t *testing.T) {
	f := setup(t)
	provider := &currentMembershipProvider{}
	owner := &repositoryaccess.Service{Pool: f.pool, App: provider}
	f.s.Membership = owner
	outbox, err := jobs.New(
		f.pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{
			HandlerKind:               f.s,
			repositoryaccess.PollKind: repositoryaccess.PollHandler{Service: owner},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.s.Jobs = outbox
	ingest := func(id uuid.UUID, action, signature string) error {
		raw := []byte(
			fmt.Sprintf(
				`{"action":%q,"member":{"id":77},"installation":{"id":7},"repository":{"id":123,"owner":{"id":99}}}`,
				action,
			),
		)
		if signature == "valid" {
			signature = sign(raw, testSecret)
		}
		return f.s.Ingest(
			t.Context(),
			f.event,
			Headers{DeliveryID: id.String(), Event: "member", Signature: signature},
			raw,
		)
	}
	count := func() int {
		t.Helper()
		var n int
		if err := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM operations WHERE event_id=$1 AND kind=$2", f.event, repositoryaccess.PollKind).
			Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if err = ingest(uuid.New(), "added", "sha256=bad"); err == nil || count() != 0 {
		t.Fatal("unauthenticated evidence queued work", err)
	}
	added := uuid.New()
	if err = ingest(added, "added", "valid"); err != nil {
		t.Fatal(err)
	}
	if count() != 0 {
		t.Fatal("unexecuted webhook affected membership")
	}
	f.execute(added)
	if count() != 1 || provider.reads != 0 {
		t.Fatal("member hint mutated membership or bypassed provider poll")
	}
	if err = ingest(added, "added", "valid"); err != nil {
		t.Fatal(err)
	}
	f.execute(added)
	if count() != 1 {
		t.Fatal("duplicate delivery duplicated poll")
	}
	removed := uuid.New()
	if err = ingest(removed, "removed", "valid"); err != nil {
		t.Fatal(err)
	}
	f.execute(removed)
	rows, err := f.pool.Query(
		t.Context(),
		"SELECT id FROM operations WHERE event_id=$1 AND kind=$2 ORDER BY id",
		f.event,
		repositoryaccess.PollKind,
	)
	if err != nil {
		t.Fatal(err)
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if err = outbox.Execute(t.Context(), id); err != nil {
			t.Fatal(err)
		}
	}
	view, err := owner.Read(t.Context(), f.owner, f.event, f.team)
	if err != nil {
		t.Fatal(err)
	}
	if provider.reads != 2 || len(view.Grants) != 0 ||
		!slices.ContainsFunc(view.Flags, func(flag repositoryaccess.Flag) bool {
			return flag.Code == "unknown_external_collaborator" && flag.GitHubID != nil &&
				*flag.GitHubID == 77
		}) {
		t.Fatal("member action order overrode current provider truth", view, provider.reads)
	}
	stale := uuid.New()
	if err = ingest(stale, "edited", "valid"); err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE github_app_integrations SET installation_id=0 WHERE event_id=$1", f.event)
	f.execute(stale)
	if count() != 2 || f.delivery(stale).Disposition != "discarded" {
		t.Fatal("stale installation queued member reconciliation")
	}
}
