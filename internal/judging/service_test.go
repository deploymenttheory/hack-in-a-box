package judging

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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/observations"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissionfreeze"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type fixture struct {
	t                             *testing.T
	pool                          *pgxpool.Pool
	s                             Service
	owner, first, second, member  dbgen.Person
	event, team, run, deliverable uuid.UUID
	hiddenDeliverable             uuid.UUID
	now                           time.Time
	dates                         event.Configuration
	criteria                      rubric.Configuration
}

func setup(t *testing.T, managed ...bool) *fixture {
	t.Helper()
	pool := testdb.New(t, true)
	f := &fixture{
		t:           t,
		pool:        pool,
		owner:       testdb.Person(t, pool),
		first:       testdb.Person(t, pool),
		second:      testdb.Person(t, pool),
		member:      testdb.Person(t, pool),
		team:        uuid.New(),
		deliverable: uuid.New(),
	}
	f.event = testdb.Event(t, pool, f.owner.ID)
	f.dates = event.DefaultConfiguration()
	f.dates.MinimumAttendance = 1
	f.dates.LiveStart = time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	f.dates.LiveEnd = f.dates.LiveStart.Add(8 * time.Hour)
	f.dates.SubmissionCutoff = f.dates.LiveStart.Add(6 * time.Hour)
	due := f.dates.LiveStart.AddDate(0, 0, -7)
	f.dates.TeamFreeze = due.Add(-time.Hour)
	f.dates.AdmissionCutoff = f.dates.TeamFreeze.Add(-time.Hour)
	f.now = due.Add(-time.Minute)
	f.s = Service{Pool: pool, Now: func() time.Time { return f.now }}
	raw, _ := json.Marshal(f.dates)
	f.exec(
		"UPDATE events SET stage='preparation',configuration=$2,team_frozen_at=$3 WHERE id=$1",
		f.event,
		raw,
		f.dates.TeamFreeze,
	)
	f.exec(
		"INSERT INTO hack_teams(event_id,id,name,state,frozen) VALUES($1,$2,'Synthetic judged build','published',true)",
		f.event,
		f.team,
	)
	f.exec("INSERT INTO competition_entries(event_id,team_id) VALUES($1,$2)", f.event, f.team)
	f.exec(
		"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
		f.event,
		f.member.ID,
	)
	f.exec(
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
		f.event,
		f.team,
		f.member.ID,
	)
	f.exec(
		"INSERT INTO event_gate_facts(event_id,attendance_ready,teams_ready,total_teams,frozen_teams,assigned_attendees)VALUES($1,true,true,1,1,1)",
		f.event,
	)
	for _, p := range []dbgen.Person{f.first, f.second} {
		f.exec(
			"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'judge')",
			f.event,
			p.ID,
		)
	}
	r := rubric.Service{Pool: pool, Clock: f.s.now}
	d := rubric.Default(rubric.AllTeams, f.event)
	d.RequiredJudgeCount = 2
	d.PrivateJudgeGuidance = "Judge-specific common instructions"
	v, err := r.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		0,
		rubric.Command{Action: "save", Definition: &d, Reason: "Synthetic criteria"},
	)
	if err != nil {
		t.Fatal(err)
	}
	v, err = r.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		v.Revision,
		rubric.Command{
			Action:  "approve",
			Version: v.Reference.Version,
			Reason:  "Synthetic explicit approval",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.criteria = v
	cs := content.Service{Pool: pool, Clock: f.s.now}
	c, err := cs.Save(
		t.Context(),
		f.owner.ID,
		f.event,
		uuid.New(),
		0,
		content.Input{
			Kind: "challenge",
			Document: content.Document{
				Title:    "Synthetic common challenge",
				Markdown: "Build a fictional prototype",
				Audience: access.ContentAudience{Confirmed: true},
				Rubric:   &v.Reference,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{"review", "approve"} {
		c, err = cs.Action(
			t.Context(),
			f.owner.ID,
			f.event,
			c.ID,
			c.Revision,
			a,
			"Synthetic reviewed release",
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	f.now = due
	if err = cs.Dispatch(t.Context(), f.event, c.ID, c.Revision); err != nil {
		t.Fatal(err)
	}
	v, err = r.Configuration(t.Context(), f.owner.ID, f.event, 0)
	if err != nil {
		t.Fatal(err)
	}
	v, err = r.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		v.Revision,
		rubric.Command{
			Action:  "create_run",
			Purpose: "judge_final",
			Version: v.Reference.Version,
			Reason:  "Synthetic independent final",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range v.Runs {
		if run.Purpose == "judge_final" {
			f.run = run.ID
		}
	}
	kind, url := "url", "https://example.test/synthetic-demo"
	if len(managed) > 0 && managed[0] {
		kind = "repository"
		url = "https://github.com/synthetic-org/judged-" + f.team.String()
		f.exec(
			"INSERT INTO team_repositories(event_id,team_id,organization_id,organization,name,app_id,installation_id,template_repository,requested_by,github_repository_id,state) VALUES($1,$2,99,'synthetic-org',$3,42,55,'template',$4,701,'ready')",
			f.event,
			f.team,
			"judged-"+f.team.String(),
			f.owner.ID,
		)
	}
	defs := templates.EmptyDefinitions()
	defs.Deliverables = []templates.Deliverable{
		{
			ID:            f.deliverable,
			Title:         "Demonstration",
			Required:      true,
			EvidenceKinds: []string{kind},
			Audience:      []string{"judge", "mentor", "organiser"},
		},
	}
	if len(managed) > 2 && managed[2] {
		f.hiddenDeliverable = uuid.New()
		defs.Deliverables = append(
			defs.Deliverables,
			templates.Deliverable{
				ID:            f.hiddenDeliverable,
				Title:         "Mentor-only artifact",
				Required:      true,
				EvidenceKinds: []string{"url"},
				Audience:      []string{"mentor", "organiser"},
			},
		)
	}
	raw, _ = json.Marshal(defs)
	f.exec(
		"INSERT INTO event_definitions(event_id,definitions) VALUES($1,$2) ON CONFLICT(event_id) DO UPDATE SET definitions=EXCLUDED.definitions",
		f.event,
		raw,
	)
	f.exec("UPDATE events SET stage='live' WHERE id=$1", f.event)
	f.now = f.dates.LiveStart
	if len(managed) > 1 && managed[1] {
		f.now = f.dates.LiveStart.Add(-time.Hour)
		f.exec("UPDATE events SET stage='preparation' WHERE id=$1", f.event)
	}
	if _, err = f.s.ChangeRound(
		t.Context(),
		f.owner.ID,
		f.event,
		f.run,
		0,
		RoundCommand{
			RequestID: uuid.New(),
			Action:    "assign",
			TeamID:    f.team,
			JudgeIDs:  []uuid.UUID{f.first.ID, f.second.ID},
			Reason:    "Assign two independent judges",
		},
	); err != nil {
		t.Fatal(err)
	}
	if len(managed) > 1 && managed[1] {
		return f
	}
	n := submissions.Service{Pool: pool, Clock: f.s.now}
	p, err := n.Read(t.Context(), f.member.ID, f.event, f.team)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = n.Submit(
		t.Context(),
		f.member.ID,
		f.event,
		f.team,
		p.Revision,
		submissions.Command{
			Reason: "Synthetic formal output",
			Evidence: append([]submissions.Evidence{
				{
					DeliverableID: f.deliverable,
					Kind:          kind,
					Label:         "Demonstration",
					URL:           url,
				},
			}, mixedFinalEvidence(f)...),
		},
	); err != nil {
		t.Fatal(err)
	}
	f.now = f.dates.SubmissionCutoff
	return f
}

func (f *fixture) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.t.Context(), query, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) scores(value int) []Score {
	out := []Score{}
	for _, category := range f.criteria.Definition.Categories {
		v := value
		out = append(
			out,
			Score{
				CategoryID: category.ID,
				Value:      &v,
				Evidence: []Evidence{
					{DeliverableID: &f.deliverable, Note: "Demonstrated in immutable final"},
				},
			},
		)
	}
	return out
}

func (f *fixture) card(
	actor uuid.UUID,
	version int64,
	action string,
	scores []Score,
) (Receipt, error) {
	return f.s.ChangeCard(
		f.t.Context(),
		actor,
		f.event,
		f.run,
		f.team,
		version,
		CardCommand{RequestID: uuid.New(), Action: action, SubmissionRevision: 1, Scores: scores},
	)
}

func TestIndependentVisibilityIncompleteLockDeliberationAndReopen(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	if _, err := f.card(f.first.ID, 0, "submit", nil); !errors.Is(err, access.ErrConflict) {
		t.Fatal("incomplete submitted", err)
	}
	if _, err := f.card(f.first.ID, 0, "save", f.scores(7)); err != nil {
		t.Fatal(err)
	}
	second, err := f.s.Brief(ctx, f.second.ID, f.event, f.run, f.team)
	if err != nil {
		t.Fatal(err)
	}
	if second.OwnCard != nil || len(second.SubmittedCards) != 0 {
		t.Fatal("independent draft visible", second)
	}
	if _, err = f.card(f.first.ID, 1, "submit", f.scores(8)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.card(f.first.ID, 2, "save", f.scores(1)); !errors.Is(err, access.ErrConflict) {
		t.Fatal("submitted card unlocked", err)
	}
	err = database.Within(ctx, f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		_, e := f.s.CompletionInTransaction(ctx, tx, f.owner.ID, f.event, f.run)
		return e
	})
	if !errors.Is(err, access.ErrConflict) {
		t.Fatal("missing judge confirmed", err)
	}
	if _, err = f.card(f.second.ID, 0, "submit", f.scores(9)); err != nil {
		t.Fatal(err)
	}
	r, err := f.s.Round(ctx, f.owner.ID, f.event, f.run)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ChangeRound(
		ctx,
		f.owner.ID,
		f.event,
		f.run,
		r.Revision,
		RoundCommand{
			RequestID: uuid.New(),
			Action:    "open_deliberation",
			Reason:    "All independent cards complete",
		},
	); err != nil {
		t.Fatal(err)
	}
	b, err := f.s.Brief(ctx, f.first.ID, f.event, f.run, f.team)
	if err != nil || len(b.SubmittedCards) != 2 {
		t.Fatal("deliberation missing complete cards", b, err)
	}
	encoded, _ := json.Marshal(b)
	for _, forbidden := range []string{"qualification_total", "rank", "private_raw_notes"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("private source in brief")
		}
	}
	approval := uuid.New()
	err = database.Within(ctx, f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		c, e := f.s.CompletionInTransaction(ctx, tx, f.owner.ID, f.event, f.run)
		if e != nil {
			return e
		}
		return f.s.RecordResultApprovalInTransaction(
			ctx,
			tx,
			f.owner.ID,
			f.event,
			f.run,
			approval,
			c.Binding,
			"Separate result owner approval",
		)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ChangeCard(
		ctx,
		f.owner.ID,
		f.event,
		f.run,
		f.team,
		2,
		CardCommand{
			RequestID:          uuid.New(),
			Action:             "reopen",
			AuthorID:           f.first.ID,
			SubmissionRevision: 1,
			Reason:             "Recorded evidence correction",
		},
	); err != nil {
		t.Fatal(err)
	}
	var invalidations int
	if err = f.pool.QueryRow(ctx, "SELECT count(*) FROM judge_result_invalidations WHERE approval_id=$1", approval).
		Scan(&invalidations); err != nil ||
		invalidations != 1 {
		t.Fatal("reopen kept approval", invalidations, err)
	}
	b, err = f.s.Brief(ctx, f.second.ID, f.event, f.run, f.team)
	if err != nil || b.Deliberation || len(b.SubmittedCards) != 0 {
		t.Fatal("reopen leaked private draft", b, err)
	}
}

func TestConcurrentVersionFenceAndDeclaredConflictRevocation(t *testing.T) {
	f := setup(t)
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() { _, err := f.card(f.first.ID, 0, "save", f.scores(5)); results <- err })
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
	r, err := f.s.Round(t.Context(), f.owner.ID, f.event, f.run)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ChangeRound(
		t.Context(),
		f.owner.ID,
		f.event,
		f.run,
		r.Revision,
		RoundCommand{
			RequestID:    uuid.New(),
			Action:       "declare_conflict",
			JudgeID:      f.first.ID,
			CompetitorID: f.member.ID,
			ConflictKind: "reporting",
			Enabled:      true,
			Reason:       "Declared reporting relationship",
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Brief(
		t.Context(),
		f.first.ID,
		f.event,
		f.run,
		f.team,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("declared conflict kept scoring access", err)
	}
	var history int
	if err = f.pool.QueryRow(t.Context(), "SELECT count(*) FROM judge_history WHERE event_id=$1 AND action='save'", f.event).
		Scan(&history); err != nil ||
		history != 1 {
		t.Fatal("authored history lost", history, err)
	}
}

func TestPracticeIsolatedAndRealFinalGate(t *testing.T) {
	f := setup(t, false, true)
	p, err := f.s.Practice(t.Context(), f.first.ID, f.event)
	if err != nil {
		t.Fatal(err)
	}
	if p.FictionalEntryID == f.team {
		t.Fatal("practice real team")
	}
	if _, err = f.s.Brief(
		t.Context(),
		f.first.ID,
		f.event,
		f.run,
		f.team,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("practice weakened final gate", err)
	}
	scores := f.scores(5)
	for i := range scores {
		scores[i].Evidence = []Evidence{{Note: "Fictional robot demonstration"}}
	}
	if _, err = f.s.ChangePractice(
		t.Context(),
		f.first.ID,
		f.event,
		0,
		PracticeCommand{RequestID: uuid.New(), Action: "submit", Scores: scores},
	); err != nil {
		t.Fatal(err)
	}
	var cards int
	if err = f.pool.QueryRow(t.Context(), "SELECT count(*) FROM judge_cards WHERE event_id=$1", f.event).
		Scan(&cards); err != nil ||
		cards != 0 {
		t.Fatal("practice entered production", cards, err)
	}
	f.now = f.dates.SubmissionCutoff
	err = database.Within(context.Background(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		_, e := f.s.CompletionInTransaction(t.Context(), tx, f.owner.ID, f.event, f.run)
		return e
	})
	if !errors.Is(err, access.ErrConflict) {
		t.Fatal("practice satisfied required judges", err)
	}
}

func (f *fixture) freeze() submissionfreeze.Service {
	f.t.Helper()
	service := submissionfreeze.Service{Pool: f.pool, Now: f.s.now}
	worker, err := jobs.New(
		f.pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{submissionfreeze.HandlerKind: &service},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	service.Jobs = worker
	if err = service.Schedule(f.t.Context(), f.event); err != nil {
		f.t.Fatal(err)
	}
	var id uuid.UUID
	if err = f.pool.QueryRow(f.t.Context(), "SELECT id FROM operations WHERE event_id=$1 AND target_id=$2 AND kind=$3", f.event, f.team, submissionfreeze.HandlerKind).
		Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	if outcome := service.Deliver(
		f.t.Context(),
		jobs.Delivery{ID: id, EventID: f.event, TargetID: f.team},
	); outcome != jobs.Delivered {
		f.t.Fatal("native cutoff source fixture", outcome)
	}
	return service
}

func TestUnresolvedManagedRepositoryCannotStartJudgeScoring(t *testing.T) {
	f := setup(t, true)
	freeze := f.freeze()
	p, err := freeze.Read(t.Context(), f.owner.ID, f.event, f.team)
	if err != nil || p.Status != "unresolved" {
		t.Fatal("unresolved real managed source fixture", p, err)
	}
	if _, err = f.card(f.first.ID, 0, "save", f.scores(5)); !errors.Is(err, access.ErrConflict) {
		t.Fatal("unresolved managed repository accepted final judge scores", err)
	}
}

func (f *fixture) decide(
	freeze submissionfreeze.Service,
	action, sha string,
) submissionfreeze.Receipt {
	f.t.Helper()
	p, err := freeze.Read(f.t.Context(), f.owner.ID, f.event, f.team)
	if err != nil {
		f.t.Fatal(err)
	}
	c := submissionfreeze.Command{
		RequestID:          uuid.New(),
		Action:             action,
		SourceID:           p.Source.ID,
		ExpectedRevision:   p.Revision,
		NativeRevision:     p.NativeRevision,
		SubmissionRevision: *p.CurrentSubmissionRevision,
		SHA:                sha,
		Reason:             "Explicit synthetic repository evidence review",
		ExternalEvidence:   "Independent reviewed historical evidence",
	}
	if p.Current != nil {
		id := p.Current.ID
		c.PriorDecisionID = &id
	}
	r, err := freeze.Change(f.t.Context(), f.owner.ID, f.event, f.team, c)
	if err != nil {
		f.t.Fatal(err)
	}
	return r
}

func (f *fixture) reviewedCard(
	actor uuid.UUID,
	version int64,
	action string,
	decision *uuid.UUID,
	author uuid.UUID,
	submissionRevision int64,
) (Receipt, error) {
	f.t.Helper()
	raw, err := json.Marshal(
		map[string]any{
			"request_id":             uuid.New(),
			"action":                 action,
			"author_id":              author,
			"submission_revision":    submissionRevision,
			"scores":                 f.scores(7),
			"repository_decision_id": decision,
			"reason":                 "Explicit corrected exact-code review",
		},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	var c CardCommand
	if err = json.Unmarshal(raw, &c); err != nil {
		f.t.Fatal(err)
	}
	if action == "reopen" {
		c.Scores = nil
	}
	return f.s.ChangeCard(f.t.Context(), actor, f.event, f.run, f.team, version, c)
}

func TestSameFinalRepositoryAmendmentCannotRetargetSubmittedJudgeCards(t *testing.T) {
	f := setup(t, true)
	freeze := f.freeze()
	first := f.decide(freeze, "reconcile", strings.Repeat("a", 40))
	decision := first.Decision.ID
	for _, judge := range []uuid.UUID{f.first.ID, f.second.ID} {
		if _, err := f.reviewedCard(judge, 0, "submit", &decision, uuid.Nil, 1); err != nil {
			t.Fatal(err)
		}
	}
	var original string
	if err := f.pool.QueryRow(t.Context(), "SELECT document::text FROM judge_cards WHERE event_id=$1 AND author_id=$2", f.event, f.first.ID).
		Scan(&original); err != nil {
		t.Fatal(err)
	}
	if err := database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		_, err := f.s.CompletionInTransaction(t.Context(), tx, f.owner.ID, f.event, f.run)
		return err
	}); err != nil {
		t.Fatal("original completed code/cards fixture", err)
	}
	roundBefore, err := f.s.Round(t.Context(), f.owner.ID, f.event, f.run)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ChangeRound(
		t.Context(),
		f.owner.ID,
		f.event,
		f.run,
		roundBefore.Revision,
		RoundCommand{
			RequestID: uuid.New(),
			Action:    "open_deliberation",
			Reason:    "Original exact code cards complete",
		},
	); err != nil {
		t.Fatal(err)
	}
	changed := f.decide(freeze, "amend", strings.Repeat("b", 40))
	if changed.Decision.SubmissionRevision != first.Decision.SubmissionRevision {
		t.Fatal("fixture changed native final rather than code receipt")
	}
	err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		_, err := f.s.CompletionInTransaction(t.Context(), tx, f.owner.ID, f.event, f.run)
		return err
	})
	if !errors.Is(err, access.ErrConflict) {
		t.Fatal("same-final code amendment silently retargeted submitted scores", err)
	}
	var after string
	if err = f.pool.QueryRow(t.Context(), "SELECT document::text FROM judge_cards WHERE event_id=$1 AND author_id=$2", f.event, f.first.ID).
		Scan(&after); err != nil ||
		after != original {
		t.Fatal("code amendment rewrote authored card", err)
	}
	current, err := f.s.Brief(t.Context(), f.first.ID, f.event, f.run, f.team)
	if err != nil || current.OwnCard == nil || current.OwnCard.State != "source_changed" ||
		len(current.OwnCard.Scores) != 0 {
		t.Fatal("current read hid change or reused old scores", current.OwnCard, err)
	}
	next := changed.Decision.ID
	if _, err = f.reviewedCard(f.owner.ID, 1, "reopen", &next, f.first.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err = f.reviewedCard(
		f.first.ID,
		2,
		"submit",
		&decision,
		uuid.Nil,
		1,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("old reviewed code decision accepted", err)
	}
	if _, err = f.reviewedCard(f.first.ID, 2, "submit", &next, uuid.Nil, 1); err != nil {
		t.Fatal("explicit current code correction denied", err)
	}
}

func TestCurrentNativeFinalReplacementScrubsOldJudgeCodeAndRequiresReviewedReopen(t *testing.T) {
	f := setup(t, true)
	freeze := f.freeze()
	first := f.decide(freeze, "reconcile", strings.Repeat("a", 40))
	decision := first.Decision.ID
	if _, err := f.reviewedCard(f.first.ID, 0, "submit", &decision, uuid.Nil, 1); err != nil {
		t.Fatal(err)
	}
	var original string
	if err := f.pool.QueryRow(t.Context(), "SELECT document::text FROM judge_cards WHERE event_id=$1 AND author_id=$2", f.event, f.first.ID).
		Scan(&original); err != nil {
		t.Fatal(err)
	}
	native := submissions.Service{Pool: f.pool, Clock: f.s.now}
	p, err := native.Read(t.Context(), f.owner.ID, f.event, f.team)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = native.Submit(
		t.Context(),
		f.owner.ID,
		f.event,
		f.team,
		p.Revision,
		submissions.Command{
			Reason: "Explicit new native final correction",
			Evidence: []submissions.Evidence{
				{
					DeliverableID: f.deliverable,
					Kind:          "repository",
					Label:         "Current frozen output",
					URL:           "https://github.com/synthetic-org/judged-" + f.team.String(),
				},
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	// A pending exact-final replacement cannot be scored before explicit freeze repair.
	if _, err = f.reviewedCard(
		f.first.ID,
		1,
		"save",
		&decision,
		uuid.Nil,
		3,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("replacement accepted unreconciled source", err)
	}
	changed := f.decide(freeze, "amend", strings.Repeat("b", 40))
	currentRevision := changed.Decision.SubmissionRevision
	brief, err := f.s.Brief(t.Context(), f.first.ID, f.event, f.run, f.team)
	if err != nil {
		t.Fatal(err)
	}
	if brief.OwnCard == nil || brief.OwnCard.State != "source_changed" ||
		brief.OwnCard.Repository.SHA != "" ||
		len(brief.OwnCard.Scores) != 0 {
		t.Fatal("selected-final read leaked old target binding", brief.OwnCard)
	}
	err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		history, e := f.s.OwnHistory(t.Context(), tx, f.first.ID, f.event, f.run, f.team)
		if e != nil {
			return e
		}
		for _, c := range history {
			if c.SubmissionRevision == 1 && (c.Repository.SHA != "" || len(c.Scores) != 0) {
				t.Fatal("historical read leaked hidden old target", c)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	next := changed.Decision.ID
	if _, err = f.reviewedCard(
		f.owner.ID,
		1,
		"reopen",
		&next,
		f.first.ID,
		currentRevision,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = f.reviewedCard(
		f.first.ID,
		2,
		"submit",
		&next,
		uuid.Nil,
		currentRevision,
	); err != nil {
		t.Fatal(err)
	}
	var oldRetained string
	if err = f.pool.QueryRow(t.Context(), "SELECT document::text FROM judge_history WHERE event_id=$1 AND actor_id=$2 AND action='submit' ORDER BY id LIMIT 1", f.event, f.first.ID).
		Scan(&oldRetained); err != nil ||
		oldRetained != original {
		t.Fatal("privacy projection rewrote original authored history", err)
	}
}

func TestCurrentAssignmentLossRevokesNativeSubmissionAndAuthoredHistoryReads(t *testing.T) {
	f := setup(t)
	if _, err := f.card(f.first.ID, 0, "save", f.scores(6)); err != nil {
		t.Fatal(err)
	}
	round, err := f.s.Round(t.Context(), f.owner.ID, f.event, f.run)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ChangeRound(
		t.Context(),
		f.owner.ID,
		f.event,
		f.run,
		round.Revision,
		RoundCommand{
			RequestID: uuid.New(),
			Action:    "assign",
			TeamID:    f.team,
			JudgeIDs:  []uuid.UUID{f.second.ID},
			Reason:    "Explicit reviewer reassignment",
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Brief(
		t.Context(),
		f.first.ID,
		f.event,
		f.run,
		f.team,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("removed judge retained scoring brief", err)
	}
	if _, err = (submissions.Service{Pool: f.pool, Clock: f.s.now}).Read(
		t.Context(),
		f.first.ID,
		f.event,
		f.team,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("old generic resource grant retained native judge access", err)
	}
	err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		_, e := f.s.OwnHistory(t.Context(), tx, f.first.ID, f.event, f.run, f.team)
		return e
	})
	if !errors.Is(err, access.ErrForbidden) {
		t.Fatal("removed assignment retained future history access", err)
	}
	var retained int
	if err = f.pool.QueryRow(t.Context(), "SELECT count(*) FROM judge_history WHERE event_id=$1 AND actor_id=$2 AND action='save'", f.event, f.first.ID).
		Scan(&retained); err != nil ||
		retained != 1 {
		t.Fatal("reassignment erased authored history", retained, err)
	}
	visible, err := f.s.Rounds(t.Context(), f.first.ID, f.event)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range visible.Items {
		if len(r.Assignments) != 0 {
			t.Fatal("discovery resurrected removed team", r)
		}
	}
}

func TestFactualBriefDoesNotImportPrivateNativeMentorEvidence(t *testing.T) {
	f := setup(t)
	mentor := testdb.Person(t, f.pool)
	f.exec(
		"INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'mentor')",
		f.event,
		mentor.ID,
	)
	f.exec(
		"INSERT INTO policy_resources(event_id,id,kind)VALUES($1,$2,'team') ON CONFLICT DO NOTHING",
		f.event,
		f.team,
	)
	f.exec(
		"INSERT INTO resource_assignments(event_id,resource_id,person_id)VALUES($1,$2,$3)",
		f.event,
		f.team,
		mentor.ID,
	)
	f.exec(
		"INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at)VALUES($1,$2,$3,'primary','published',$4,$5)",
		f.event,
		f.team,
		mentor.ID,
		f.dates.LiveStart,
		f.dates.LiveEnd,
	)
	service := observations.Service{Pool: f.pool, Now: f.s.now}
	current, err := service.Read(t.Context(), mentor.ID, f.event, f.team)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Change(
		t.Context(),
		mentor.ID,
		f.event,
		f.team,
		current.Revision,
		observations.Command{
			Action: "observation",
			ID:     uuid.New(),
			Evidence: observations.Evidence{
				Category:          "PRIVATE-QUALIFICATION-CATEGORY",
				Evidence:          "PRIVATE-RAW-MENTOR-NOTE",
				Outcome:           "PRIVATE-MENTOR-OUTCOME",
				FeedbackReference: "PRIVATE-FEEDBACK",
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	brief, err := f.s.Brief(t.Context(), f.first.ID, f.event, f.run, f.team)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(brief)
	if strings.Contains(string(raw), "PRIVATE-") ||
		strings.Contains(string(raw), "qualification_total") ||
		strings.Contains(string(raw), "rank") {
		t.Fatal("factual handoff imported private assessment data", string(raw))
	}
	if brief.Output.FrozenRevision == nil ||
		brief.SubmissionRevision != *brief.Output.FrozenRevision ||
		len(brief.Output.Artifacts) != 1 {
		t.Fatal("factual native final metadata missing", brief.Output)
	}
}

func mixedFinalEvidence(f *fixture) []submissions.Evidence {
	if f.hiddenDeliverable == uuid.Nil {
		return nil
	}
	return []submissions.Evidence{
		{
			DeliverableID: f.hiddenDeliverable,
			Kind:          "url",
			Label:         "Mentor-only artifact",
			URL:           "https://example.test/private-mentor-artifact",
		},
	}
}
