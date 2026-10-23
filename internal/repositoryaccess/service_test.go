package repositoryaccess

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/repositories"
	"github.com/deploymenttheory/hack-in-a-box/internal/teams"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

// The provider is deterministic and entirely in-process. All state/authority,
// provisioning, approval and worker execution use their production PG owners.
var nextAccessRepository atomic.Int64

type accessProvider struct {
	mu         sync.Mutex
	repository *githubapp.Repository
	snapshot   githubapp.AccessSnapshot
	writes     int
	fault      string
}

func (p *accessProvider) LookupRepository(
	_ context.Context,
	_ *dbgen.Queries,
	_ uuid.UUID,
	r githubapp.RepositoryRequest,
) (githubapp.Repository, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if r.Name == r.TemplateRepository {
		return githubapp.Repository{ID: 700, OrganizationID: 99}, nil
	}
	if p.repository == nil {
		return githubapp.Repository{}, githubapp.ErrRepositoryAbsent
	}
	return *p.repository, nil
}

func (p *accessProvider) CreateRepository(
	_ context.Context,
	_ *dbgen.Queries,
	_ uuid.UUID,
	r githubapp.RepositoryRequest,
) (githubapp.Repository, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	repo := githubapp.Repository{
		ID:             800 + nextAccessRepository.Add(1),
		OrganizationID: 99,
		Name:           r.Name,
		Description:    r.Marker,
		Private:        true,
		TemplateID:     700,
	}
	p.repository = &repo
	return repo, nil
}

func (p *accessProvider) InspectCollaborators(
	context.Context,
	*dbgen.Queries,
	uuid.UUID,
	githubapp.AccessRequest,
) (githubapp.AccessSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fault == "read" {
		return githubapp.AccessSnapshot{}, context.DeadlineExceeded
	}
	return githubapp.AccessSnapshot{
		Collaborators: slices.Clone(p.snapshot.Collaborators),
		Invitations:   slices.Clone(p.snapshot.Invitations),
	}, nil
}

func (p *accessProvider) ChangeCollaborator(
	_ context.Context,
	_ *dbgen.Queries,
	_ uuid.UUID,
	_ githubapp.AccessRequest,
	c githubapp.AccessChange,
) (githubapp.AccessResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writes++
	if p.fault == "policy" {
		return githubapp.AccessResult{}, githubapp.ErrAccessRejected
	}
	if p.fault == "unsent" {
		return githubapp.AccessResult{}, githubapp.ErrAccessNotSent
	}
	if c.Remove {
		p.snapshot.Collaborators = slices.DeleteFunc(
			p.snapshot.Collaborators,
			func(v githubapp.Collaborator) bool { return v.ID == c.AccountID },
		)
		p.snapshot.Invitations = slices.DeleteFunc(
			p.snapshot.Invitations,
			func(v githubapp.Invitation) bool { return v.AccountID == c.AccountID },
		)
		return githubapp.AccessResult{}, nil
	}
	if p.fault == "accepted" {
		p.snapshot.Collaborators = append(
			p.snapshot.Collaborators,
			githubapp.Collaborator{ID: c.AccountID, Permission: permissionName(c.Permission)},
		)
		return githubapp.AccessResult{Accepted: true}, nil
	}
	invite := githubapp.Invitation{
		ID:         int64(1000 + p.writes),
		AccountID:  c.AccountID,
		Permission: permissionName(c.Permission),
		CreatedAt:  time.Now().UTC(),
	}
	p.snapshot.Invitations = append(p.snapshot.Invitations, invite)
	if p.fault == "lost" {
		return githubapp.AccessResult{}, context.DeadlineExceeded
	}
	return githubapp.AccessResult{Invitation: &invite}, nil
}

type accessFixture struct {
	t                          *testing.T
	pool                       *pgxpool.Pool
	event, team, owner, member uuid.UUID
	provider                   *accessProvider
	service                    Service
	jobs                       *jobs.Service
}

func setupAccess(t *testing.T, pool *pgxpool.Pool) *accessFixture {
	t.Helper()
	owner, member := testdb.Person(t, pool), testdb.Person(t, pool)
	f := &accessFixture{
		t:        t,
		pool:     pool,
		event:    testdb.Event(t, pool, owner.ID),
		team:     uuid.New(),
		owner:    owner.ID,
		member:   member.ID,
		provider: &accessProvider{},
	}
	f.exec(
		"INSERT INTO hack_teams(id,event_id,name,state)VALUES($1,$2,'Synthetic published team','published')",
		f.team,
		f.event,
	)
	f.exec("INSERT INTO competition_entries(event_id,team_id)VALUES($1,$2)", f.event, f.team)
	f.exec(
		"INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')",
		f.event,
		f.member,
	)
	f.exec(
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot)VALUES($1,$2,$3,'published')",
		f.event,
		f.team,
		f.member,
	)
	// github_id is globally unique across the shared database fixture.
	var account int64
	if err := pool.QueryRow(t.Context(), "SELECT COALESCE(MAX(github_id),100)+1 FROM github_accounts").
		Scan(&account); err != nil {
		t.Fatal(err)
	}
	f.exec(
		"INSERT INTO github_accounts(person_id,github_id,login,token_encrypted)VALUES($1,$2,'synthetic-current',decode('00','hex'))",
		f.member,
		account,
	)
	f.exec("INSERT INTO github_link_states(person_id,revision)VALUES($1,7)", f.member)
	f.exec(
		"INSERT INTO github_app_integrations(event_id,app_id,app_slug,organization_id,organization,template_repository,credential_version,key_id,private_key_encrypted,installation_id)VALUES($1,42,'synthetic-app',99,'synthetic-org','template',1,'synthetic',decode('00','hex'),55)",
		f.event,
	)
	f.service = Service{Pool: pool, App: f.provider}
	repoOwner := repositories.Service{Pool: pool, App: f.provider}
	var err error
	f.jobs, err = jobs.New(
		pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{
			repositories.HandlerKind: repoOwner,
			HandlerKind:              &f.service,
			PollKind:                 PollHandler{Service: &f.service},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	event, _ := dbgen.New(pool).Event(t.Context(), f.event)
	if err = repoOwner.Queue(t.Context(), f.owner, f.event, event.Revision); err != nil {
		t.Fatal(err)
	}
	var op uuid.UUID
	if err = pool.QueryRow(t.Context(), "SELECT id FROM operations WHERE event_id=$1 AND kind=$2", f.event, repositories.HandlerKind).
		Scan(&op); err != nil {
		t.Fatal(err)
	}
	if err = f.jobs.Execute(t.Context(), op); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *accessFixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.t.Context(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *accessFixture) publish() {
	f.t.Helper()
	f.exec("UPDATE hack_teams SET frozen=true WHERE id=$1", f.team)
	f.exec(
		"UPDATE events SET team_frozen_at=now(),published_at=now()-interval '1 second' WHERE id=$1",
		f.event,
	)
}

func (f *accessFixture) command(command Command) Result {
	f.t.Helper()
	event, err := dbgen.New(f.pool).Event(f.t.Context(), f.event)
	if err != nil {
		f.t.Fatal(err)
	}
	result, err := f.service.Change(
		f.t.Context(),
		f.owner,
		f.event,
		f.team,
		event.Revision,
		command,
	)
	if err != nil {
		f.t.Fatal(err)
	}
	return result
}

func (f *accessFixture) review() Plan {
	f.t.Helper()
	r := f.command(
		Command{Action: "review", PlanID: uuid.New(), Reason: "Review current native roster"},
	)
	if r.Receipt == nil {
		f.t.Fatal("missing exact receipt")
	}
	return *r.Receipt
}

func (f *accessFixture) approve(p Plan) {
	f.t.Helper()
	f.command(
		Command{
			Action:      "approve",
			PlanID:      p.ID,
			PlanVersion: p.Version,
			Reason:      "Approve exact reviewed access",
		},
	)
}

func (f *accessFixture) execute(action Action) {
	f.t.Helper()
	var op uuid.UUID
	if err := f.pool.QueryRow(f.t.Context(), "SELECT id FROM operations WHERE event_id=$1 AND kind=$2 AND target_id=$3", f.event, HandlerKind, action.ID).
		Scan(&op); err != nil {
		f.t.Fatal(err)
	}
	if err := f.jobs.Execute(f.t.Context(), op); err != nil {
		f.t.Fatal(err)
	}
}

func (f *accessFixture) view() View {
	f.t.Helper()
	v, err := f.service.Read(f.t.Context(), f.owner, f.event, f.team)
	if err != nil {
		f.t.Fatal(err)
	}
	return v
}

func (f *accessFixture) refresh() {
	f.t.Helper()
	f.command(Command{Action: "refresh", Reason: "Inspect provider current state"})
}

func TestReviewedRepositoryAccessRealPostgres(t *testing.T) {
	pool := testdb.New(t, true)
	t.Run(
		"approval never grants before freeze and actual challenge publication",
		func(t *testing.T) {
			f := setupAccess(t, pool)
			p := f.review()
			if len(p.Actions) != 1 {
				t.Fatal(p)
			}
			f.approve(p)
			var count int
			if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM operations WHERE event_id=$1 AND kind=$2", f.event, HandlerKind).
				Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 || f.provider.writes != 0 {
				t.Fatal("prepublication grant was queued")
			}
			f.exec("UPDATE hack_teams SET frozen=true WHERE id=$1", f.team)
			f.exec("UPDATE events SET team_frozen_at=now() WHERE id=$1", f.event)
			nativeEvent, nativeErr := dbgen.New(pool).Event(t.Context(), f.event)
			if nativeErr != nil {
				t.Fatal(nativeErr)
			}
			if _, nativeErr = (teams.Service{Pool: pool}).Act(
				t.Context(),
				f.owner,
				f.event,
				nativeEvent.Revision,
				teams.Command{
					Action:   "move",
					TeamID:   uuid.New(),
					PersonID: f.member,
					Reason:   "Attempt frozen reassignment",
				},
			); !errors.Is(
				nativeErr,
				teams.ErrFrozen,
			) {
				t.Fatal("native frozen reassignment was permitted", nativeErr)
			}
			if err := f.service.Maintain(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM operations WHERE event_id=$1 AND kind=$2", f.event, HandlerKind).
				Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatal("scheduled freeze treated as T7 publication")
			}
			f.publish()
			if err := f.service.Maintain(t.Context()); err != nil {
				t.Fatal(err)
			}
			f.execute(p.Actions[0])
			v := f.view()
			if f.provider.writes != 1 || len(v.Grants) != 1 || v.Grants[0].State != "pending" ||
				!v.Grants[0].Managed {
				t.Fatal("owned invitation not retained", v)
			}
			f.refresh()
			again := f.review()
			if len(again.Actions) != 0 {
				t.Fatal("pending duplicate", again)
			}
			// Accepted state is a fresh provider observation; receipt remains the original review.
			f.provider.snapshot.Collaborators = []githubapp.Collaborator{
				{ID: p.Members[0].GitHubID, Permission: "write"},
			}
			f.provider.snapshot.Invitations = nil
			f.refresh()
			if f.view().Grants[0].State != "accepted" {
				t.Fatal("accepted state not reconciled")
			}
			own, err := f.service.Own(t.Context(), f.member, f.event, f.team)
			if err != nil || len(own.Grants) != 1 {
				t.Fatal(own, err)
			}
			f.exec(
				"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
				f.event,
				f.member,
			)
			if _, err = f.service.Own(
				t.Context(),
				f.member,
				f.event,
				f.team,
			); !errors.Is(
				err,
				access.ErrForbidden,
			) {
				t.Fatal("withdrawn own reader", err)
			}
			if err = f.service.Maintain(t.Context()); err != nil {
				t.Fatal(err)
			}
			automatic := f.view().Plan
			if automatic == nil || automatic.ApprovedAt != nil || len(automatic.Actions) != 1 ||
				automatic.Actions[0].Kind != "remove" ||
				f.provider.writes != 1 {
				t.Fatal("native roster change did not create an unapproved review plan", automatic)
			}
			removal := f.review()
			if len(removal.Actions) != 1 || removal.Actions[0].Kind != "remove" {
				t.Fatal("withdrawal did not propose owned removal", removal)
			}
			f.approve(removal)
			f.execute(removal.Actions[0])
			if f.provider.writes != 2 || f.view().Grants[0].State != "revoked" {
				t.Fatal("withdrawal not revoked")
			}
			var retained int
			if err = pool.QueryRow(t.Context(), "SELECT count(*) FROM hack_team_members WHERE event_id=$1 AND team_id=$2 AND person_id=$3 AND snapshot='published'", f.event, f.team, f.member).
				Scan(&retained); err != nil ||
				retained != 1 {
				t.Fatal("frozen assignment changed", err)
			}
		},
	)
	t.Run("lost provider response stays uncertain and cannot resend", func(t *testing.T) {
		f := setupAccess(t, pool)
		f.publish()
		p := f.review()
		f.approve(p)
		f.provider.fault = "lost"
		f.execute(p.Actions[0])
		f.provider.fault = ""
		f.refresh()
		v := f.view()
		if len(v.Grants) != 1 || !v.Grants[0].Uncertain || v.Grants[0].Managed {
			t.Fatal("uncertain ownership fabricated", v)
		}
		again := f.review()
		if len(again.Actions) != 0 {
			t.Fatal("uncertain effect generated another mutation", again)
		}
		for range 3 {
			f.service.Deliver(
				t.Context(),
				jobs.Delivery{EventID: f.event, TargetID: p.Actions[0].ID},
			)
		}
		if f.provider.writes != 1 {
			t.Fatal("uncertain provider operation resent", f.provider.writes)
		}
		for _, sql := range []string{"UPDATE repository_access_effects SET attempted=false,dispatch_id=NULL,state='planned' WHERE id=$1", "UPDATE repository_access_effects SET dispatch_id=gen_random_uuid() WHERE id=$1", "DELETE FROM repository_access_effects WHERE id=$1", "DELETE FROM repository_access_grants WHERE id=$1"} {
			if _, err := pool.Exec(t.Context(), sql, p.Actions[0].ID); err == nil {
				t.Fatal("durable barrier erased", sql)
			}
		}
	})
	t.Run("receipt commit failure retains barrier and cannot resend", func(t *testing.T) {
		f := setupAccess(t, pool)
		f.publish()
		p := f.review()
		f.approve(p)
		f.exec(
			"CREATE FUNCTION fixture_reject_access_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic receipt commit failure'; END $$",
		)
		f.exec(
			"CREATE CONSTRAINT TRIGGER fixture_access_receipt AFTER UPDATE ON repository_access_grants DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN (NEW.app_managed) EXECUTE FUNCTION fixture_reject_access_receipt()",
		)
		f.execute(p.Actions[0])
		f.exec("DROP TRIGGER fixture_access_receipt ON repository_access_grants")
		f.exec("DROP FUNCTION fixture_reject_access_receipt()")
		f.refresh()
		v := f.view()
		if f.provider.writes != 1 || len(v.Grants) != 1 || !v.Grants[0].Uncertain ||
			v.Grants[0].Managed {
			t.Fatal("rolled-back receipt lost barrier", v)
		}
		f.service.Reconcile(t.Context(), jobs.Delivery{EventID: f.event, TargetID: p.Actions[0].ID})
		if f.provider.writes != 1 || len(f.review().Actions) != 0 {
			t.Fatal("commit failure repeated external invitation")
		}
	})
	t.Run("concurrent native workers dispatch once", func(t *testing.T) {
		f := setupAccess(t, pool)
		f.publish()
		p := f.review()
		f.approve(p)
		var operationID uuid.UUID
		if err := pool.QueryRow(t.Context(), "SELECT id FROM operations WHERE event_id=$1 AND target_id=$2", f.event, p.Actions[0].ID).
			Scan(&operationID); err != nil {
			t.Fatal(err)
		}
		var workers sync.WaitGroup
		errs := make(chan error, 2)
		for range 2 {
			workers.Go(func() { errs <- f.jobs.Execute(t.Context(), operationID) })
		}
		workers.Wait()
		close(errs)
		for err := range errs {
			var snooze *river.JobSnoozeError
			if err != nil && !errors.As(err, &snooze) {
				t.Fatal(err)
			}
		}
		if f.provider.writes != 1 {
			t.Fatal("concurrent workers duplicated provider write", f.provider.writes)
		}
	})
	t.Run("stale link revision and withdrawn claim stop queued grants", func(t *testing.T) {
		f := setupAccess(t, pool)
		f.publish()
		p := f.review()
		f.approve(p)
		f.exec("UPDATE github_link_states SET revision=revision+1 WHERE person_id=$1", f.member)
		f.execute(p.Actions[0])
		if f.provider.writes != 0 {
			t.Fatal("stale identity granted")
		}
		next := f.review()
		f.approve(next)
		f.exec(
			"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
			f.event,
			f.member,
		)
		f.execute(next.Actions[0])
		if f.provider.writes != 0 {
			t.Fatal("withdrawn identity granted")
		}
	})
	t.Run(
		"published mentor coverage expires without changing frozen assignments",
		func(t *testing.T) {
			f := setupAccess(t, pool)
			f.publish()
			now := time.Now().UTC().Add(-2 * time.Hour)
			start, end := now.Add(-time.Hour), now.Add(time.Hour)
			config := event.DefaultConfiguration()
			config.LiveStart = start
			config.LiveEnd = end
			config.SubmissionCutoff = end.Add(-10 * time.Minute)
			encoded, _ := json.Marshal(config)
			f.exec(
				"UPDATE events SET configuration=$2,published_at=$3 WHERE id=$1",
				f.event,
				encoded,
				start.Add(-time.Hour),
			)
			primary, second := testdb.Person(t, pool), testdb.Person(t, pool)
			availability, _ := json.Marshal([]map[string]time.Time{{"start": start, "end": end}})
			for i, person := range []uuid.UUID{primary.ID, second.ID} {
				slot := "primary"
				if i == 1 {
					slot = "second"
				}
				f.exec(
					"INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'mentor')",
					f.event,
					person,
				)
				f.exec(
					"INSERT INTO mentor_profiles(event_id,person_id,availability)VALUES($1,$2,$3)",
					f.event,
					person,
					availability,
				)
				f.exec(
					"INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at)VALUES($1,$2,$3,$4,'published',$5,$6)",
					f.event,
					f.team,
					person,
					slot,
					start,
					end,
				)
				var numericID int64
				if err := pool.QueryRow(t.Context(), "SELECT COALESCE(MAX(github_id),100)+1 FROM github_accounts").
					Scan(&numericID); err != nil {
					t.Fatal(err)
				}
				f.exec(
					"INSERT INTO github_accounts(person_id,github_id,login,token_encrypted)VALUES($1,$2,'synthetic-mentor',decode('00','hex'))",
					person,
					numericID,
				)
				f.exec("INSERT INTO github_link_states(person_id,revision)VALUES($1,1)", person)
			}
			f.service.Now = func() time.Time { return now }
			p := f.review()
			if len(p.Actions) != 3 {
				t.Fatal("native coverage missing", p)
			}
			f.approve(p)
			for _, action := range p.Actions {
				if action.Member.Role == "mentor" &&
					(action.Member.Permission != "pull" || action.Member.CoverageEnd == nil) {
					t.Fatal("mentor scope widened", action)
				}
				f.execute(action)
			}
			own, err := f.service.Own(t.Context(), primary.ID, f.event, f.team)
			if err != nil || len(own.Grants) != 1 || own.Grants[0].Member.CoverageEnd == nil {
				t.Fatal("mentor own scoped history", own, err)
			}
			f.service.Now = func() time.Time { return end.Add(time.Second) }
			if _, err = f.service.Own(
				t.Context(),
				primary.ID,
				f.event,
				f.team,
			); !errors.Is(
				err,
				access.ErrForbidden,
			) {
				t.Fatal("expired mentor retained invitation access", err)
			}
			next := f.review()
			if len(next.Actions) != 2 {
				t.Fatal("expired coverage did not propose removal", next)
			}
			f.approve(next)
			for _, action := range next.Actions {
				if action.Kind != "remove" {
					t.Fatal(action)
				}
				f.execute(action)
			}
			if f.provider.writes != 5 {
				t.Fatal("expired mentor removals were not executed", f.provider.writes)
			}
			var retained int
			if err = pool.QueryRow(t.Context(), "SELECT count(*) FROM mentor_assignments WHERE event_id=$1 AND snapshot='published'", f.event).
				Scan(&retained); err != nil ||
				retained != 2 {
				t.Fatal("frozen mentor history changed", err)
			}
		},
	)
	t.Run("expired owned invitation is cancelled before replacement", func(t *testing.T) {
		f := setupAccess(t, pool)
		f.publish()
		p := f.review()
		f.approve(p)
		f.execute(p.Actions[0])
		f.provider.snapshot.Invitations[0].Expired = true
		f.refresh()
		next := f.review()
		if len(next.Actions) != 2 || next.Actions[0].Kind != "remove" ||
			next.Actions[1].Kind != "invite" {
			t.Fatal(next)
		}
		f.approve(next)
		var count int
		if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM operations WHERE event_id=$1 AND target_id=$2", f.event, next.Actions[1].ID).
			Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatal("replacement queued before cancellation")
		}
		f.execute(next.Actions[0])
		if err := f.service.Maintain(t.Context()); err != nil {
			t.Fatal(err)
		}
		f.execute(next.Actions[1])
		if f.provider.writes != 3 || len(f.provider.snapshot.Invitations) != 1 ||
			f.provider.snapshot.Invitations[0].Expired {
			t.Fatal("expired recovery did not converge")
		}
	})
	t.Run("policy failure is explicit and only a new reviewed plan can retry", func(t *testing.T) {
		f := setupAccess(t, pool)
		f.publish()
		p := f.review()
		f.approve(p)
		f.provider.fault = "policy"
		f.execute(p.Actions[0])
		v := f.view()
		if len(v.Grants) != 1 || v.Grants[0].State != "failed" ||
			v.Grants[0].Diagnostic != "organization_policy_restricted" {
			t.Fatal("policy failure missing", v)
		}
		f.provider.fault = ""
		next := f.review()
		if len(next.Actions) != 1 {
			t.Fatal(next)
		}
		f.approve(next)
		f.execute(next.Actions[0])
		if f.provider.writes != 2 {
			t.Fatal("reviewed recovery did not run")
		}
	})
	t.Run(
		"direct existing access remains unmanaged and unknown collaborators are flagged",
		func(t *testing.T) {
			f := setupAccess(t, pool)
			f.publish()
			f.provider.snapshot.Collaborators = []githubapp.Collaborator{
				{ID: 987654, Permission: "admin"},
			}
			p := f.review()
			f.approve(p)
			f.provider.fault = "accepted"
			f.execute(p.Actions[0])
			v := f.view()
			if len(v.Grants) != 1 || v.Grants[0].Managed {
				t.Fatal("204 response adopted ownership", v)
			}
			f.exec(
				"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
				f.event,
				f.member,
			)
			next := f.review()
			if len(next.Actions) != 0 {
				t.Fatal("unmanaged collaborator removal", next)
			}
			if !slices.ContainsFunc(
				next.Flags,
				func(flag Flag) bool { return flag.Code == "unknown_external_collaborator" },
			) {
				t.Fatal("unknown collaborators hidden", next)
			}
		},
	)
}
