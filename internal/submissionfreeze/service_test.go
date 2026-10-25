package submissionfreeze

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type recordedApp struct {
	entered chan struct{}
	release chan struct{}
	mu      sync.Mutex
	value   githubapp.BranchObservation
	err     error
	calls   int
	repo    int64
	branch  string
}

func (p *recordedApp) ObserveBranch(
	_ context.Context,
	_ *dbgen.Queries,
	_ uuid.UUID,
	_ githubapp.RepositoryRequest,
	repo int64,
	branch string,
) (githubapp.BranchObservation, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.entered != nil {
		close(p.entered)
		<-p.release
	}
	p.calls++
	p.repo = repo
	p.branch = branch
	return p.value, p.err
}

type fixture struct {
	t                                                          *testing.T
	pool                                                       *pgxpool.Pool
	q                                                          *dbgen.Queries
	service                                                    *Service
	jobs                                                       *jobs.Service
	provider                                                   *recordedApp
	owner, member, other, event, team, deliverable, provenance uuid.UUID
	repository                                                 int64
	now                                                        time.Time
	cfg                                                        event.Configuration
}

func newFixture(t *testing.T, pool *pgxpool.Pool, managed ...bool) *fixture {
	t.Helper()
	f := &fixture{
		t:           t,
		pool:        pool,
		q:           dbgen.New(pool),
		owner:       testdb.Person(t, pool).ID,
		member:      testdb.Person(t, pool).ID,
		other:       testdb.Person(t, pool).ID,
		team:        uuid.New(),
		deliverable: uuid.New(),
		provenance:  uuid.New(),
		repository:  time.Now().UnixNano(),
	}
	f.event = testdb.Event(t, pool, f.owner)
	f.cfg = event.DefaultConfiguration()
	f.cfg.LiveStart = time.Date(2026, 6, 20, 9, 0, 0, 0, time.UTC)
	f.cfg.LiveEnd = f.cfg.LiveStart.Add(8 * time.Hour)
	f.cfg.SubmissionCutoff = f.cfg.LiveStart.Add(7 * time.Hour)
	f.cfg.TeamFreeze = f.cfg.LiveStart.Add(-8 * 24 * time.Hour)
	f.now = f.cfg.SubmissionCutoff.Add(-time.Minute)
	c, _ := json.Marshal(f.cfg)
	f.exec(
		"UPDATE events SET configuration=$2,stage='preparation',published_at=$3,team_frozen_at=$4 WHERE id=$1",
		f.event,
		c,
		f.cfg.LiveStart.Add(-7*24*time.Hour),
		f.cfg.TeamFreeze,
	)
	f.exec(
		"INSERT INTO hack_teams(id,event_id,name,state,frozen,captain_id,published_captain_id) VALUES($1,$2,'Synthetic freeze team','published',true,$3,$3)",
		f.team,
		f.event,
		f.member,
	)
	f.exec("INSERT INTO competition_entries(event_id,team_id) VALUES($1,$2)", f.event, f.team)
	f.exec(
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
		f.event,
		f.team,
		f.member,
	)
	if err := f.q.SetCompetitionState(
		t.Context(),
		dbgen.SetCompetitionStateParams{EventID: f.event, PersonID: f.member, State: "confirmed"},
	); err != nil {
		t.Fatal(err)
	}
	defs := templates.EmptyDefinitions()
	defs.Deliverables = []templates.Deliverable{
		{
			ID:            f.deliverable,
			Title:         "Build",
			Required:      true,
			EvidenceKinds: []string{"repository", "url"},
		},
	}
	d, _ := json.Marshal(defs)
	if err := f.q.SaveEventDefinitions(
		t.Context(),
		dbgen.SaveEventDefinitionsParams{EventID: f.event, Definitions: d},
	); err != nil {
		t.Fatal(err)
	}
	f.exec(
		"INSERT INTO github_app_integrations(event_id,app_id,app_slug,organization_id,organization,template_repository,credential_version,key_id,private_key_encrypted,installation_id) VALUES($1,42,'synthetic-app',99,'synthetic-org','template',1,'synthetic',decode('00','hex'),55)",
		f.event,
	)
	if len(managed) == 0 || managed[0] {
		f.exec(
			"INSERT INTO team_repositories(event_id,team_id,organization_id,organization,name,app_id,installation_id,template_repository,requested_by,provenance,github_repository_id,state) VALUES($1,$2,99,'synthetic-org',$3,42,55,'template',$4,$5,$6,'ready')",
			f.event,
			f.team,
			"freeze-"+f.team.String(),
			f.owner,
			f.provenance,
			f.repository,
		)
	}
	f.provider = &recordedApp{
		value: githubapp.BranchObservation{SHA: strings.Repeat("a", 40), Status: "current_ref"},
	}
	f.service = &Service{Pool: pool, App: f.provider, Now: func() time.Time { return f.now }}
	var err error
	f.jobs, err = jobs.New(
		pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{HandlerKind: f.service},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.service.Jobs = f.jobs
	return f
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.t.Context(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) submit(kind string) {
	f.t.Helper()
	s := submissions.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	p, err := s.Read(f.t.Context(), f.member, f.event, f.team)
	if err != nil {
		f.t.Fatal(err)
	}
	_, err = s.Submit(
		f.t.Context(),
		f.member,
		f.event,
		f.team,
		p.Revision,
		submissions.Command{
			Evidence: []submissions.Evidence{
				{
					DeliverableID: f.deliverable,
					Kind:          kind,
					Label:         "Synthetic build",
					URL:           "https://github.com/synthetic-org/freeze-" + f.team.String(),
				},
			},
		},
	)
	if err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) operation() uuid.UUID {
	f.t.Helper()
	if err := f.service.Schedule(f.t.Context(), f.event); err != nil {
		f.t.Fatal(err)
	}
	var id uuid.UUID
	if err := f.pool.QueryRow(f.t.Context(), "SELECT id FROM operations WHERE event_id=$1 AND kind=$2 ORDER BY due_at DESC LIMIT 1", f.event, HandlerKind).
		Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	return id
}

func (f *fixture) capture() {
	f.t.Helper()
	id := f.operation()
	if err := f.jobs.Execute(f.t.Context(), id); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) read(actor uuid.UUID) Page {
	f.t.Helper()
	p, err := f.service.Read(f.t.Context(), actor, f.event, f.team)
	if err != nil {
		f.t.Fatal(err)
	}
	return p
}

func (f *fixture) decision(p Page, action string) Command {
	f.t.Helper()
	c := Command{
		SubmissionRevision: *p.CurrentSubmissionRevision, NativeRevision: p.NativeRevision,
		RequestID:        uuid.New(),
		Action:           action,
		ExpectedRevision: p.Revision,
		SourceID:         p.Source.ID,
		SHA:              strings.Repeat("a", 40),
		Reason:           "Reviewed outage evidence against cutoff; explicit organiser decision",
		ExternalEvidence: "Independent observed cutoff build reviewed; private decision evidence",
	}
	if len(p.Observations) > 0 && p.Observations[0].SHA == c.SHA {
		c.ObservationIDs = []uuid.UUID{p.Observations[0].ID}
	}
	if p.Current != nil {
		id := p.Current.ID
		c.PriorDecisionID = &id
	}
	return c
}

func TestNativeCutoffEvidence(t *testing.T) {
	pool := testdb.New(t, true)
	t.Run("before at after cutoff and immutable reviewed receipt", func(t *testing.T) {
		f := newFixture(t, pool)
		f.submit("repository")
		c := Command{
			RequestID:        uuid.New(),
			Action:           "configure",
			Branch:           "release/build",
			ExpectedRevision: 0,
		}
		ack, err := f.service.Change(t.Context(), f.member, f.event, f.team, c)
		if err != nil {
			t.Fatal(err)
		}
		if ack.Revision != 1 {
			t.Fatal(ack)
		}
		id := f.operation()
		if err = f.jobs.Execute(t.Context(), id); err != nil {
			t.Fatal(err)
		}
		p := f.read(f.member)
		if p.Source != nil || f.provider.calls != 0 {
			t.Fatal("captured before cutoff", p)
		}
		f.now = f.cfg.SubmissionCutoff
		if err = f.jobs.Execute(t.Context(), id); err != nil {
			t.Fatal(err)
		}
		p = f.read(f.owner)
		if p.Status != "unresolved" || p.NativeOutcome != "frozen" || !p.BindingCurrent ||
			p.Source.SubmissionRevision == nil ||
			len(p.Observations) != 1 ||
			!p.Observations[0].ObservedAt.Equal(f.now) ||
			p.Source.Branch != "release/build" ||
			f.provider.repo != f.repository {
			t.Fatalf("wrong cutoff source %+v", p)
		}
		if _, err = f.service.Change(
			t.Context(),
			f.member,
			f.event,
			f.team,
			Command{
				RequestID:        uuid.New(),
				Action:           "configure",
				Branch:           "other",
				ExpectedRevision: p.Revision,
			},
		); !errors.Is(
			err,
			access.ErrForbidden,
		) {
			t.Fatal(err)
		}
		c = f.decision(p, "reconcile")
		ack, err = f.service.Change(t.Context(), f.owner, f.event, f.team, c)
		if err != nil {
			t.Fatal(err)
		}
		replay, err := f.service.Change(t.Context(), f.owner, f.event, f.team, c)
		if err != nil || replay.Fingerprint != ack.Fingerprint ||
			replay.Decision.ID != ack.Decision.ID {
			t.Fatal("lost original ACK", replay, err)
		}
		original, err := f.service.OriginalReceipt(
			t.Context(),
			f.owner,
			f.event,
			f.team,
			c.RequestID,
		)
		if err != nil || original.Receipt == nil ||
			original.Receipt.Decision.ID != ack.Decision.ID {
			t.Fatal(original, err)
		}
		f.now = f.now.Add(time.Hour)
		f.provider.value.SHA = strings.Repeat("b", 40)
		f.capture()
		p = f.read(f.owner)
		if p.Status != "completed" || p.Current.SHA != strings.Repeat("a", 40) ||
			f.provider.calls != 1 {
			t.Fatal("later commits mutated receipt", p)
		}
		member := f.read(f.member)
		if member.Current.Reason != "" || member.Current.ExternalEvidence != "" {
			t.Fatal("private review prose leaked", member.Current)
		}
		c = f.decision(p, "amend")
		c.SHA = strings.Repeat("b", 40)
		c.ObservationIDs = nil
		amend, err := f.service.Change(t.Context(), f.owner, f.event, f.team, c)
		if err != nil {
			t.Fatal(err)
		}
		p = f.read(f.owner)
		if len(p.Decisions) != 2 || p.Decisions[0].ID != ack.Decision.ID ||
			p.Current.ID != amend.Decision.ID ||
			p.Source.SubmissionRevision == nil {
			t.Fatal("amendment erased history", p)
		}
		f.exec("UPDATE events SET writes_allowed=false,stage='archived' WHERE id=$1", f.event)
		c = f.decision(p, "amend")
		if _, err = f.service.Change(
			t.Context(),
			f.owner,
			f.event,
			f.team,
			c,
		); !errors.Is(
			err,
			access.ErrForbidden,
		) {
			t.Fatal("archive amendment allowed", err)
		}
	})
	t.Run("late successful observation remains unresolved", func(t *testing.T) {
		f := newFixture(t, pool)
		f.submit("repository")
		f.now = f.cfg.SubmissionCutoff.Add(10 * time.Minute)
		f.capture()
		p := f.read(f.owner)
		if p.Status != "unresolved" || p.Current != nil ||
			!p.Source.Cutoff.Equal(f.cfg.SubmissionCutoff) ||
			!p.Observations[0].ObservedAt.Equal(f.now) {
			t.Fatal(p)
		}
	})
	t.Run("outage retry preserves attempt history and no historical guarantee", func(t *testing.T) {
		f := newFixture(t, pool)
		f.submit("repository")
		f.now = f.cfg.SubmissionCutoff
		f.provider.err = context.DeadlineExceeded
		id := f.operation()
		if err := f.jobs.Execute(t.Context(), id); !errors.Is(err, jobs.ErrRetry) {
			t.Fatal(err)
		}
		f.now = f.now.Add(time.Hour)
		f.provider.err = nil
		if err := f.jobs.Execute(t.Context(), id); err != nil {
			t.Fatal(err)
		}
		p := f.read(f.owner)
		if p.Status != "unresolved" || len(p.Observations) != 2 ||
			p.Observations[0].Status != "provider_unavailable" ||
			p.Observations[1].SHA == "" {
			t.Fatal(p)
		}
	})
	for _, status := range []string{"branch_missing", "empty_repository", "repository_unavailable"} {
		t.Run(status, func(t *testing.T) {
			f := newFixture(t, pool)
			f.submit("repository")
			f.provider.value = githubapp.BranchObservation{Status: status}
			f.now = f.cfg.SubmissionCutoff
			f.capture()
			p := f.read(f.owner)
			if p.Status != "unresolved" || len(p.Observations) != 1 ||
				p.Observations[0].Status != status ||
				p.Current != nil {
				t.Fatal(p)
			}
		})
	}
	for _, outcome := range []string{"not_submitted", "withdrawn", "disbanded", "not_applicable", "managed_repository_missing", "authority_lost"} {
		t.Run(outcome, func(t *testing.T) {
			f := newFixture(t, pool, outcome != "managed_repository_missing")
			if outcome != "not_submitted" {
				kind := "repository"
				if outcome == "not_applicable" {
					kind = "url"
				}
				f.submit(kind)
			}
			switch outcome {
			case "withdrawn":
				f.exec("UPDATE competition_entries SET active=false WHERE event_id=$1", f.event)
			case "disbanded":
				f.exec("UPDATE hack_teams SET state='disbanded' WHERE event_id=$1", f.event)

			case "authority_lost":
				f.exec(
					"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2",
					f.event,
					f.owner,
				)
			case "configuration_lost":
				f.exec(
					"UPDATE github_app_integrations SET credential_version=credential_version+1 WHERE event_id=$1",
					f.event,
				)
			}
			f.now = f.cfg.SubmissionCutoff
			f.capture()
			if outcome == "authority_lost" {
				var count int
				if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM submission_freeze_observations o JOIN submission_freeze_sources s ON s.id=o.source_id WHERE s.event_id=$1", f.event).
					Scan(&count); err != nil ||
					count != 1 ||
					f.provider.calls != 0 {
					t.Fatal(count, err, f.provider.calls)
				}
				return
			}
			p := f.read(f.owner)
			switch outcome {
			case "not_submitted", "not_applicable":
				if p.Status != outcome || f.provider.calls != 0 {
					t.Fatal(p)
				}
			case "withdrawn", "disbanded":
				if p.NativeOutcome != "withdrawn" || p.Source.NativeOutcome != "withdrawn" ||
					f.provider.calls != 0 {
					t.Fatal(p)
				}
			case "managed_repository_missing":
				if p.Status != "managed_repository_missing" || len(p.Observations) != 0 ||
					f.provider.calls != 0 {
					t.Fatal(p)
				}
			case "configuration_lost":
				if p.Status != "unresolved" {
					t.Fatal(p)
				}
			}
		})
	}
	t.Run("concurrent jobs and competing organiser decisions", func(t *testing.T) {
		f := newFixture(t, pool)
		f.submit("repository")
		f.now = f.cfg.SubmissionCutoff
		id := f.operation()
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for range 2 {
			wg.Go(func() { errs <- f.jobs.Execute(t.Context(), id) })
		}
		wg.Wait()
		close(errs)
		p := f.read(f.owner)
		if len(p.Observations) != 1 || f.provider.calls != 1 {
			t.Fatal(p)
		}
		c1, c2 := f.decision(p, "reconcile"), f.decision(p, "reconcile")
		results := make(chan error, 2)
		for _, c := range []Command{c1, c2} {
			wg.Go(
				func() { _, err := f.service.Change(t.Context(), f.owner, f.event, f.team, c); results <- err },
			)
		}
		wg.Wait()
		close(results)
		success, conflict := 0, 0
		for err := range results {
			if err == nil {
				success++
			} else if errors.Is(err, access.ErrConflict) {
				conflict++
			} else {
				t.Fatal(err)
			}
		}
		if success != 1 || conflict != 1 {
			t.Fatal(success, conflict)
		}
	})
}
