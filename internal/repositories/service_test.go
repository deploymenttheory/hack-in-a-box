package repositories

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

var nextRepositoryID atomic.Int64

type fakeApp struct {
	mu                    sync.Mutex
	repos                 map[string]githubapp.Repository
	creates               map[string]int
	fail, timeout, tamper string
	templateMissing       bool
}

func (p *fakeApp) LookupRepository(
	_ context.Context,
	_ *dbgen.Queries,
	_ uuid.UUID,
	r githubapp.RepositoryRequest,
) (githubapp.Repository, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if r.Name == r.TemplateRepository {
		if p.templateMissing {
			return githubapp.Repository{}, githubapp.ErrRepositoryAbsent
		}
		return githubapp.Repository{ID: 700, OrganizationID: 99}, nil
	}
	if repo, ok := p.repos[r.Name]; ok {
		return repo, nil
	}
	return githubapp.Repository{}, githubapp.ErrRepositoryAbsent
}

func (p *fakeApp) CreateRepository(
	_ context.Context,
	_ *dbgen.Queries,
	_ uuid.UUID,
	r githubapp.RepositoryRequest,
) (githubapp.Repository, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.creates[r.Name]++
	if r.Name == p.fail {
		return githubapp.Repository{}, githubapp.ErrRepositoryRejected
	}
	if _, ok := p.repos[r.Name]; ok {
		return githubapp.Repository{}, githubapp.ErrRepositoryRejected
	}
	repo := githubapp.Repository{
		ID:             800 + nextRepositoryID.Add(1),
		OrganizationID: 99,
		Name:           r.Name,
		Description:    r.Marker,
		Private:        true,
		TemplateID:     700,
	}
	switch p.tamper {
	case "public":
		repo.Private = false
	case "wrong-organization":
		repo.OrganizationID = 100
	case "wrong-template":
		repo.TemplateID = 701
	case "wrong-name":
		repo.Name = "unrelated"
	case "wrong-provenance":
		repo.Description = "unrelated"
	}
	p.repos[r.Name] = repo
	if r.Name == p.timeout {
		return githubapp.Repository{}, context.DeadlineExceeded
	}
	return repo, nil
}

type fixture struct {
	t                    *testing.T
	pool                 *pgxpool.Pool
	event, owner, member uuid.UUID
	s                    Service
	p                    *fakeApp
	j                    *jobs.Service
}

func setup(t *testing.T, pool *pgxpool.Pool) *fixture {
	owner, member := testdb.Person(t, pool), testdb.Person(t, pool)
	f := &fixture{
		t:      t,
		pool:   pool,
		event:  testdb.Event(t, pool, owner.ID),
		owner:  owner.ID,
		member: member.ID,
		p:      &fakeApp{repos: map[string]githubapp.Repository{}, creates: map[string]int{}},
	}
	f.s = Service{Pool: pool, App: f.p}
	f.exec(
		"INSERT INTO github_app_integrations(event_id,app_id,app_slug,organization_id,organization,template_repository,credential_version,key_id,private_key_encrypted,installation_id) VALUES($1,42,'synthetic-app',99,'synthetic-org','template',1,'synthetic',decode('00','hex'),55)",
		f.event,
	)
	var err error
	f.j, err = jobs.New(
		pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{HandlerKind: f.s},
	)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.t.Context(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) team(state string) uuid.UUID {
	f.t.Helper()
	id := uuid.New()
	f.exec(
		"INSERT INTO hack_teams(id,event_id,name,state) VALUES($1,$2,'Synthetic team',$3)",
		id,
		f.event,
		state,
	)
	f.exec("INSERT INTO competition_entries(event_id,team_id) VALUES($1,$2)", f.event, id)
	if state == "published" {
		member := testdb.Person(f.t, f.pool)
		f.member = member.ID
		f.exec(
			"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
			f.event,
			member.ID,
		)
		f.exec(
			"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
			f.event,
			id,
			member.ID,
		)
	}
	return id
}

func (f *fixture) queue() {
	f.t.Helper()
	e, err := dbgen.New(f.pool).Event(f.t.Context(), f.event)
	if err != nil {
		f.t.Fatal(err)
	}
	if err = f.s.Queue(f.t.Context(), f.owner, f.event, e.Revision); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) op(team uuid.UUID) dbgen.Operation {
	f.t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(f.t.Context(), "SELECT id FROM operations WHERE event_id=$1 AND target_id=$2 AND kind=$3", f.event, team, HandlerKind).
		Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	op, err := dbgen.New(f.pool).Operation(f.t.Context(), id)
	if err != nil {
		f.t.Fatal(err)
	}
	return op
}

func (f *fixture) row(team uuid.UUID) dbgen.TeamRepository {
	f.t.Helper()
	r, err := (Reader{Pool: f.pool}).ForTeam(f.t.Context(), f.event, team)
	if err != nil {
		f.t.Fatal(err)
	}
	return r
}

func (f *fixture) execute(team uuid.UUID) {
	f.t.Helper()
	if err := f.j.Execute(f.t.Context(), f.op(team).ID); err != nil {
		f.t.Fatal(err)
	}
}

func TestDurableProvisioningPostgres(t *testing.T) {
	pool := testdb.New(t, true)
	t.Run(
		"timeout reconciles exact identity and repeated requests never create twice",
		func(t *testing.T) {
			f := setup(t, pool)
			team := f.team("published")
			f.p.timeout = Name(f.event, team)
			f.queue()
			f.execute(team)
			if r := f.row(team); r.State != "ambiguous" || r.GithubRepositoryID.Valid {
				t.Fatalf("uncertain response claimed: %+v", r)
			}
			e, _ := dbgen.New(pool).Event(t.Context(), f.event)
			if err := f.s.Recover(t.Context(), f.owner, f.event, team, e.Revision); err != nil {
				t.Fatal(err)
			}
			r := f.row(team)
			if r.State != "ready" || !r.GithubRepositoryID.Valid ||
				f.op(team).State != "delivered" {
				t.Fatalf("did not reconcile exact resource %+v", r)
			}
			f.queue()
			f.execute(team)
			d := jobs.Delivery{EventID: f.event, TargetID: team}
			if f.s.Deliver(t.Context(), d) != jobs.Delivered || f.p.creates[r.Name] != 1 {
				t.Fatalf("duplicate create: %+v", f.p.creates)
			}
			if _, err := (Reader{Pool: pool}).ByGitHubID(
				t.Context(),
				r.GithubRepositoryID.Int64,
			); err != nil {
				t.Fatal(err)
			}
		},
	)
	t.Run(
		"collision never claims unrelated repository and partial batch retains successes",
		func(t *testing.T) {
			f := setup(t, pool)
			good, bad, collision, draft := f.team(
				"published",
			), f.team(
				"published",
			), f.team(
				"published",
			), f.team(
				"draft",
			)
			f.p.fail = Name(f.event, bad)
			name := Name(f.event, collision)
			f.p.repos[name] = githubapp.Repository{
				ID:             999,
				OrganizationID: 99,
				Name:           name,
				Private:        true,
				TemplateID:     700,
				Description:    "unrelated",
			}
			f.queue()
			f.execute(good)
			f.execute(bad)
			f.execute(collision)
			if f.row(good).State != "ready" || f.row(bad).State != "review" ||
				f.row(collision).Diagnostic != "name_collision" ||
				f.p.creates[name] != 0 {
				t.Fatal("partial batch or collision contract failed")
			}
			if _, err := (Reader{Pool: pool}).ForTeam(
				t.Context(),
				f.event,
				draft,
			); !errors.Is(
				err,
				pgx.ErrNoRows,
			) {
				t.Fatalf("draft provisioned: %v", err)
			}
			if f.row(collision).GithubRepositoryID.Valid {
				t.Fatal("claimed unrelated repo")
			}
		},
	)
	t.Run("template unavailable and revoked organiser stop before create", func(t *testing.T) {
		f := setup(t, pool)
		team := f.team("published")
		f.p.templateMissing = true
		f.queue()
		if err := f.j.Execute(t.Context(), f.op(team).ID); !errors.Is(err, jobs.ErrRetry) {
			t.Fatalf("template failure not retryable: %v", err)
		}
		if f.row(team).Attempted || len(f.p.creates) != 0 {
			t.Fatal("created without template")
		}
		f.exec("UPDATE people SET active=false WHERE id=$1", f.owner)
		if err := f.j.Execute(t.Context(), f.op(team).ID); !errors.Is(err, jobs.ErrEligibility) {
			t.Fatalf("revoked authority: %v", err)
		}
	})
	t.Run("uncertainty without proof never creates again", func(t *testing.T) {
		f := setup(t, pool)
		team := f.team("published")
		f.queue()
		f.exec(
			"UPDATE team_repositories SET attempted=true,template_repository_id=700 WHERE event_id=$1",
			f.event,
		)
		d := jobs.Delivery{EventID: f.event, TargetID: team}
		if got := f.s.Deliver(t.Context(), d); got != jobs.Ambiguous {
			t.Fatalf("unproven absence became %s", got)
		}
		if len(f.p.creates) != 0 || f.row(team).GithubRepositoryID.Valid {
			t.Fatal("unproven create repeated/claimed")
		}
		name := Name(f.event, team)
		f.p.repos[name] = githubapp.Repository{
			ID:             999,
			OrganizationID: 99,
			Name:           name,
			Private:        true,
			TemplateID:     700,
			Description:    "wrong-marker",
		}
		if f.s.Reconcile(t.Context(), d) != jobs.Permanent || f.row(team).GithubRepositoryID.Valid {
			t.Fatal("marker collision claimed")
		}
	})
	t.Run("withdrawn attendee cannot use retained frozen published membership", func(t *testing.T) {
		f := setup(t, pool)
		team := f.team("published")
		f.queue()
		f.execute(team)
		f.exec("UPDATE events SET published_at=now(),team_frozen_at=now() WHERE id=$1", f.event)
		f.exec("UPDATE hack_teams SET frozen=true WHERE id=$1", team)
		if rows, err := f.s.List(t.Context(), f.member, f.event); err != nil || len(rows) != 1 {
			t.Fatalf("confirmed member unavailable: %v %v", rows, err)
		}
		if err := dbgen.New(pool).
			SetCompetitionState(t.Context(), dbgen.SetCompetitionStateParams{EventID: f.event, PersonID: f.member, State: "withdrawn"}); err != nil {
			t.Fatal(err)
		}
		if _, err := dbgen.New(pool).
			TeamAssignment(t.Context(), dbgen.TeamAssignmentParams{EventID: f.event, PersonID: f.member, Snapshot: "published"}); err != nil {
			t.Fatalf("frozen membership was not retained: %v", err)
		}
		if rows, err := f.s.List(
			t.Context(),
			f.member,
			f.event,
		); !errors.Is(err, access.ErrForbidden) ||
			len(rows) != 0 {
			t.Fatalf("withdrawn attendee read retained repository metadata: %+v %v", rows, err)
		}
	})
	t.Run(
		"current publication gates prelive reader and disband retains unscored repo",
		func(t *testing.T) {
			f := setup(t, pool)
			team := f.team("published")
			f.queue()
			f.execute(team)
			if _, err := f.s.List(t.Context(), f.member, f.event); err == nil {
				t.Fatal("revealed repo before challenge publication")
			}
			f.exec("UPDATE events SET published_at=now(),stage='preparation' WHERE id=$1", f.event)
			views, err := f.s.List(t.Context(), f.member, f.event)
			if err != nil || len(views) != 1 || views[0].GitHubID == nil {
				t.Fatalf("prelive access missing %+v %v", views, err)
			}
			outsider := testdb.Person(t, pool)
			if _, err = f.s.List(t.Context(), outsider.ID, f.event); err == nil {
				t.Fatal("outsider repository read")
			}
			f.exec("UPDATE hack_teams SET state='disbanded' WHERE id=$1", team)
			f.exec("UPDATE competition_entries SET active=false WHERE team_id=$1", team)
			if eligible, err := f.s.Eligible(
				t.Context(),
				jobs.Delivery{EventID: f.event, TargetID: team},
			); err != nil ||
				eligible {
				t.Fatalf("disbanded eligible %v %v", eligible, err)
			}
			views, err = f.s.List(t.Context(), f.owner, f.event)
			if err != nil || !views[0].Unused || f.row(team).GithubRepositoryID.Int64 == 0 {
				t.Fatal("disband deleted/transferred mapping")
			}
			var accepted int
			if err = pool.QueryRow(t.Context(), "SELECT count(*) FROM submission_revisions WHERE event_id=$1", f.event).
				Scan(&accepted); err != nil ||
				accepted != 0 {
				t.Fatalf("provisioning accepted a submission %d %v", accepted, err)
			}
		},
	)
	t.Run(
		"concurrent workers own one create and provider identity is immutable",
		func(t *testing.T) {
			f := setup(t, pool)
			team := f.team("published")
			f.queue()
			var wg sync.WaitGroup
			id := f.op(team).ID
			for range 2 {
				wg.Go(func() { _ = f.j.Execute(t.Context(), id) })
			}
			wg.Wait()
			r := f.row(team)
			if !r.GithubRepositoryID.Valid || f.p.creates[r.Name] != 1 {
				t.Fatalf("concurrency lost identity %+v", r)
			}
			for _, query := range []string{"UPDATE team_repositories SET github_repository_id=123456 WHERE event_id=$1", "UPDATE team_repositories SET provenance=gen_random_uuid() WHERE event_id=$1", "UPDATE team_repositories SET template_repository_id=701 WHERE event_id=$1"} {
				if _, err := pool.Exec(t.Context(), query, f.event); err == nil {
					t.Fatal("mutated immutable identity")
				}
			}
		},
	)
	for _, fault := range []string{"public", "wrong-organization", "wrong-template", "wrong-name", "wrong-provenance"} {
		t.Run("provider identity rejected "+fault, func(t *testing.T) {
			f := setup(t, pool)
			team := f.team("published")
			f.p.tamper = fault
			f.queue()
			f.execute(team)
			if f.row(team).GithubRepositoryID.Valid || f.row(team).State != "ambiguous" {
				t.Fatal("unproven resource was bound")
			}
			if f.s.Reconcile(
				t.Context(),
				jobs.Delivery{EventID: f.event, TargetID: team},
			) != jobs.Permanent ||
				f.p.creates[Name(f.event, team)] != 1 {
				t.Fatal("reconciliation claimed an unrelated/nonprivate resource")
			}
		})
	}
	t.Run(
		"automatic maintenance is publication driven and catches late App setup",
		func(t *testing.T) {
			f := setup(t, pool)
			team := f.team("published")
			draft := f.team("draft")
			if err := f.s.Maintain(t.Context()); err != nil {
				t.Fatal(err)
			}
			f.execute(team)
			if f.row(team).State != "ready" || f.p.creates[Name(f.event, draft)] != 0 {
				t.Fatal("automatic publication provisioning failed")
			}
		},
	)
}
