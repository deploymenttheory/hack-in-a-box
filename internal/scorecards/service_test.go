package scorecards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type fixture struct {
	t                                     *testing.T
	pool                                  *pgxpool.Pool
	s                                     Service
	owner, primary, second, member, other dbgen.Person
	event, team, entry                    uuid.UUID
	now                                   time.Time
	config                                event.Configuration
	rubric                                rubric.Service
	criteria                              rubric.Configuration
	checkpoint                            Checkpoint
}

func setup(t *testing.T) *fixture {
	t.Helper()
	pool := testdb.New(t, true)
	f := &fixture{
		t:       t,
		pool:    pool,
		owner:   testdb.Person(t, pool),
		primary: testdb.Person(t, pool),
		second:  testdb.Person(t, pool),
		member:  testdb.Person(t, pool),
		other:   testdb.Person(t, pool),
		team:    uuid.New(),
		entry:   uuid.New(),
	}
	f.event = testdb.Event(t, pool, f.owner.ID)
	f.config = event.DefaultConfiguration()
	f.config.Timezone = "Europe/London"
	f.config.MinimumAttendance = 2
	f.config.LiveStart = time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	f.config.LiveEnd = f.config.LiveStart.Add(8 * time.Hour)
	f.config.SubmissionCutoff = f.config.LiveEnd.Add(-time.Hour)
	due, err := schedule.ChallengePublication(
		f.config.LiveStart,
		f.config.Timezone,
		f.config.DSTChoice,
	)
	if err != nil {
		t.Fatal(err)
	}
	f.config.TeamFreeze = due.Add(-time.Hour)
	f.config.AdmissionCutoff = f.config.TeamFreeze.Add(-time.Hour)
	f.now = due.Add(-time.Minute)
	config, _ := json.Marshal(f.config)
	f.exec(
		"UPDATE events SET stage='preparation',configuration=$2,team_frozen_at=$3 WHERE id=$1",
		f.event,
		config,
		f.config.TeamFreeze,
	)
	f.exec(
		"INSERT INTO event_gate_facts(event_id,attendance_ready,teams_ready,total_teams,frozen_teams,assigned_attendees) VALUES($1,true,true,1,1,2)",
		f.event,
	)
	f.exec(
		"INSERT INTO hack_teams(event_id,id,name,state,frozen) VALUES($1,$2,'Synthetic scorecard team','published',true)",
		f.event,
		f.team,
	)
	f.exec(
		"INSERT INTO competition_entries(id,event_id,team_id) VALUES($1,$2,$3)",
		f.entry,
		f.event,
		f.team,
	)
	for _, p := range []dbgen.Person{f.member, f.other} {
		f.exec(
			"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
			f.event,
			p.ID,
		)
		f.exec(
			"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
			f.event,
			f.team,
			p.ID,
		)
	}
	for _, p := range []dbgen.Person{f.primary, f.second} {
		f.exec(
			"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mentor')",
			f.event,
			p.ID,
		)
	}
	f.exec(
		"INSERT INTO policy_resources(event_id,id,kind) VALUES($1,$2,'team'),($1,$3,'assessment')",
		f.event,
		f.team,
		f.entry,
	)
	f.exec(
		"INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at) VALUES($1,$2,$3,'primary','published',$5,$6),($1,$2,$4,'second','published',$5,$6)",
		f.event,
		f.team,
		f.primary.ID,
		f.second.ID,
		f.config.LiveStart,
		f.config.LiveEnd,
	)
	f.s = Service{Pool: pool, Now: func() time.Time { return f.now }}
	f.rubric = rubric.Service{Pool: pool, Clock: func() time.Time { return f.now }}
	definition := rubric.Default(rubric.MentorQualification, f.event)
	definition.PrivateJudgeGuidance = "PRIVATE-JUDGE-CANARY"
	f.rubricChange(rubric.Command{Action: "save", Definition: &definition})
	f.rubricChange(rubric.Command{Action: "approve", Version: f.criteria.Reference.Version})
	c := content.Service{Pool: pool, Clock: func() time.Time { return f.now }}
	page, err := c.Save(
		t.Context(),
		f.owner.ID,
		f.event,
		uuid.New(),
		0,
		content.Input{
			Kind: "challenge",
			Document: content.Document{
				Title:    "Actual guarded challenge",
				Markdown: "Synthetic approved competition output",
				Rubric:   &f.criteria.Reference,
				Audience: access.ContentAudience{Confirmed: true},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"review", "approve"} {
		page, err = c.Action(
			t.Context(),
			f.owner.ID,
			f.event,
			page.ID,
			page.Revision,
			action,
			"Synthetic explicit common challenge",
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	f.now = due
	if err = c.Dispatch(t.Context(), f.event, page.ID, page.Revision); err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE events SET stage='live' WHERE id=$1", f.event)
	f.now = f.config.LiveStart
	f.checkpoint = f.configure("checkpoint")
	return f
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.t.Context(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) revision() int64 {
	f.t.Helper()
	row, err := dbgen.New(f.pool).Event(f.t.Context(), f.event)
	if err != nil {
		f.t.Fatal(err)
	}
	return row.Revision
}

func (f *fixture) rubricChange(c rubric.Command) {
	f.t.Helper()
	latest, err := f.rubric.Configuration(f.t.Context(), f.owner.ID, f.event, 0)
	if err != nil {
		f.t.Fatal(err)
	}
	f.criteria.Revision = latest.Revision
	c.Reason = "Synthetic reviewed criteria"
	out, err := f.rubric.Change(f.t.Context(), f.owner.ID, f.event, f.criteria.Revision, c)
	if err != nil {
		f.t.Fatal(err)
	}
	f.criteria = out
}

func (f *fixture) configure(purpose string) Checkpoint {
	f.t.Helper()
	f.rubricChange(
		rubric.Command{
			Action:  "create_run",
			Version: f.criteria.Reference.Version,
			Purpose: purpose,
		},
	)
	var run uuid.UUID
	for _, r := range f.criteria.Runs {
		if r.Purpose == purpose {
			run = r.ID
		}
	}
	zone, _ := time.LoadLocation(f.config.Timezone)
	opens, closes := f.config.LiveStart, f.config.SubmissionCutoff
	if purpose == "mentor_final" {
		opens, closes = f.config.SubmissionCutoff, f.config.LiveEnd.Add(time.Hour)
	}
	out, err := f.s.Configure(
		f.t.Context(),
		f.owner.ID,
		f.event,
		f.revision(),
		CheckpointCommand{
			Definition: CheckpointDefinition{
				ID:             uuid.New(),
				Name:           "Synthetic " + purpose,
				RunID:          run,
				OpensWallTime:  opens.In(zone).Format("2006-01-02T15:04:05"),
				ClosesWallTime: closes.In(zone).Format("2006-01-02T15:04:05"),
			},
			Reason: "Explicit configured checkpoint",
		},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	return out
}

func (f *fixture) command(cp Checkpoint, action string, complete bool) CardCommand {
	scores := []CategoryScore{}
	if complete {
		for _, category := range cp.Criteria.Categories {
			v := category.MinScore
			scores = append(
				scores,
				CategoryScore{
					CategoryID: category.ID,
					Value:      &v,
					Evidence: []EvidenceReference{
						{Kind: "note", Note: "PRIVATE-SCORE-EVIDENCE-CANARY"},
					},
				},
			)
		}
	}
	return CardCommand{
		Action:            action,
		CheckpointID:      cp.Definition.ID,
		CheckpointVersion: cp.Version,
		Scores:            scores,
	}
}

func (f *fixture) freeze() {
	f.t.Helper()
	f.exec("INSERT INTO submission_states(event_id,team_id) VALUES($1,$2)", f.event, f.team)
	f.exec(
		"INSERT INTO submission_revisions(event_id,team_id,revision,actor_id,submitted_at,late,document) VALUES($1,$2,1,$3,$4,false,'{\"definitions\":[],\"evidence\":[]}')",
		f.event,
		f.team,
		f.member.ID,
		f.config.SubmissionCutoff.Add(-time.Minute),
	)
	f.exec(
		"UPDATE submission_states SET active_revision=1,outcome='frozen',frozen_at=$3 WHERE event_id=$1 AND team_id=$2",
		f.event,
		f.team,
		f.config.SubmissionCutoff,
	)
}

func TestIndependentCheckpointLifecycleUsesNativeRubricAndKeepsPendingPrivate(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	command := f.command(f.checkpoint, "save", false)
	f.now = f.config.LiveStart.Add(-time.Nanosecond)
	if _, err := f.s.Change(ctx, f.primary.ID, f.event, f.team, f.revision(), command); err == nil {
		t.Fatal("pre-live card accepted")
	}
	f.now = f.config.LiveStart
	card, err := f.s.Change(ctx, f.primary.ID, f.event, f.team, f.revision(), command)
	if err != nil {
		t.Fatal(err)
	}
	if len(card.Scores) != 5 || !card.Scores[0].Pending || card.Scores[0].Value != nil ||
		card.Rubric != f.criteria.Reference ||
		card.Purpose != "checkpoint" ||
		card.SubmissionRevision != nil {
		t.Fatal("pending/provisional/source identity lost")
	}
	for _, p := range []dbgen.Person{f.member, f.owner} {
		if _, err = f.s.Read(
			ctx,
			p.ID,
			f.event,
			f.team,
			uuid.Nil,
			0,
		); !errors.Is(
			err,
			access.ErrForbidden,
		) {
			t.Fatal("unassigned privilege read private scores", err)
		}
	}
	own, err := f.s.Read(ctx, f.second.ID, f.event, f.team, uuid.Nil, 0)
	if err != nil || len(own.Items) != 0 {
		t.Fatal("second mentor saw independent first-author card", err)
	}
	if _, err = f.s.History(
		ctx,
		f.second.ID,
		f.event,
		f.team,
		card.Reference.ID,
		nil,
		0,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("history exposed another independent author", err)
	}
	if _, err = f.s.Panel(
		ctx,
		f.owner.ID,
		f.event,
		f.team,
		uuid.New(),
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("absent huddle owner granted context", err)
	}
	command = f.command(f.checkpoint, "submit", true)
	command.Version = 1
	card, err = f.s.Change(ctx, f.primary.ID, f.event, f.team, f.revision(), command)
	if err != nil {
		t.Fatal(err)
	}
	history, err := f.s.History(ctx, f.primary.ID, f.event, f.team, card.Reference.ID, nil, 0)
	if err != nil || len(history.Items) != 2 || !history.Items[1].Scores[0].Pending ||
		history.Items[0].AuthorID != history.Items[1].AuthorID {
		t.Fatal("author/history changed", err)
	}
	f.now = f.config.SubmissionCutoff
	if _, err = f.s.Change(ctx, f.primary.ID, f.event, f.team, f.revision(), command); err == nil {
		t.Fatal("late checkpoint became final")
	}
	final := f.configure("mentor_final")
	cmd := f.command(final, "save", false)
	if _, err = f.s.Change(ctx, f.primary.ID, f.event, f.team, f.revision(), cmd); err == nil {
		t.Fatal("unfrozen final output accepted")
	}
	f.freeze()
	fresh, err := f.s.Change(ctx, f.primary.ID, f.event, f.team, f.revision(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Reference.ID == card.Reference.ID || fresh.SubmissionRevision == nil ||
		*fresh.SubmissionRevision != 1 ||
		!fresh.Scores[0].Pending ||
		fresh.Scores[0].Value != nil {
		t.Fatal("checkpoint totals carried into final")
	}
}

func TestCardConcurrentRevisionsAndNativeFreezeAreAtomic(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	command := f.command(f.checkpoint, "save", true)
	revision := f.revision()
	f.exec(
		"CREATE FUNCTION reject_card_fixture() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic immutable-card fault'; END $$",
	)
	f.exec(
		"CREATE TRIGGER card_fixture_failure BEFORE INSERT ON mentor_scorecard_versions FOR EACH ROW EXECUTE FUNCTION reject_card_fixture()",
	)
	if _, err := f.s.Change(ctx, f.primary.ID, f.event, f.team, revision, command); err == nil {
		t.Fatal("failed card write succeeded")
	}
	run, err := dbgen.New(f.pool).
		RubricRun(ctx, dbgen.RubricRunParams{EventID: f.event, ID: f.checkpoint.Definition.RunID})
	if err != nil || run.StartedAt.Valid {
		t.Fatal("failed card left native rubric frozen", err)
	}
	f.exec("DROP TRIGGER card_fixture_failure ON mentor_scorecard_versions")
	card, err := f.s.Change(ctx, f.primary.ID, f.event, f.team, revision, command)
	if err != nil {
		t.Fatal(err)
	}
	command.Version = card.Reference.Version
	revision = f.revision()
	results := make(chan error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			<-start
			_, err := f.s.Change(ctx, f.primary.ID, f.event, f.team, revision, command)
			results <- err
		})
	}
	close(start)
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
		t.Fatal("concurrent score edits lost history", success, conflict)
	}
}

// Future104 contract fixture: this is a typed adapter reading a real persisted
// synthetic context row, NOT an implemented huddle/moderation workflow.
type recordedOwner struct{}

func (recordedOwner) ResolveScorecardContextInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID, id uuid.UUID,
) (ModerationContext, error) {
	var raw []byte
	err := tx.QueryRow(ctx, "SELECT document FROM synthetic_moderation_contexts WHERE event_id=$1 AND team_id=$2 AND id=$3 AND actor_id=$4 AND active", eventID, teamID, id, actor).
		Scan(&raw)
	if err != nil {
		return ModerationContext{}, fmt.Errorf("resolve synthetic owning source: %w", err)
	}
	var out ModerationContext
	if err = json.Unmarshal(raw, &out); err != nil {
		return out, err
	}
	return out, nil
}

func TestFinalSignoffNeedsCompleteExactFrozenCardsAndCurrentRecordedOwner(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	final := f.configure("mentor_final")
	f.now = f.config.SubmissionCutoff
	f.freeze()
	cmd := f.command(final, "submit", true)
	first, err := f.s.Change(ctx, f.primary.ID, f.event, f.team, f.revision(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	pending := f.command(final, "submit", false)
	second, err := f.s.Change(ctx, f.second.ID, f.event, f.team, f.revision(), pending)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	refs := []CardReference{first.Reference, second.Reference}
	proof := ModerationContext{
		ID:                 id,
		Version:            1,
		Binding:            "synthetic-owner-current-v1",
		TeamID:             f.team,
		RunID:              final.Definition.RunID,
		SubmissionRevision: 1,
		RequiredAuthorIDs:  []uuid.UUID{f.primary.ID, f.second.ID},
		Cards:              refs,
	}
	f.exec(
		"CREATE TABLE synthetic_moderation_contexts(event_id uuid,team_id uuid,id uuid,actor_id uuid,active boolean,document jsonb)",
	)
	raw, _ := json.Marshal(proof)
	f.exec(
		"INSERT INTO synthetic_moderation_contexts VALUES($1,$2,$3,$4,true,$5)",
		f.event,
		f.team,
		id,
		f.owner.ID,
		raw,
	)
	sign := func() error {
		return database.Within(ctx, f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
			_, err := f.s.SignOffInTransaction(ctx, tx, f.owner.ID, f.event, f.team, id, refs)
			return err
		})
	}
	if err = sign(); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("nil owner granted finalisation", err)
	}
	f.s.Moderation = recordedOwner{}
	if err = sign(); !errors.Is(err, access.ErrConflict) {
		t.Fatal("pending categories finalised", err)
	}
	cmd.Version = 1
	second, err = f.s.Change(ctx, f.second.ID, f.event, f.team, f.revision(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	refs[1] = second.Reference
	proof.Cards = refs
	raw, _ = json.Marshal(proof)
	f.exec("UPDATE synthetic_moderation_contexts SET document=$1", raw)

	for _, failure := range []string{"unresolved_gaps", "missing_review", "wrong_frozen_source", "revoked_moderator"} {
		broken := proof
		switch failure {
		case "unresolved_gaps":
			broken.UnresolvedGaps = 1
		case "missing_review":
			broken.RequiredAuthorIDs = append(
				append([]uuid.UUID{}, proof.RequiredAuthorIDs...),
				f.member.ID,
			)
		case "wrong_frozen_source":
			broken.SubmissionRevision = 2
		case "revoked_moderator":
			f.exec(
				"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='organiser'",
				f.event,
				f.owner.ID,
			)
		}
		raw, _ = json.Marshal(broken)
		f.exec("UPDATE synthetic_moderation_contexts SET document=$1", raw)
		if err = sign(); err == nil {
			t.Fatal("finalisation accepted incomplete/current-source failure", failure)
		}
		f.exec(
			"UPDATE event_memberships SET active=true WHERE event_id=$1 AND person_id=$2 AND role='organiser'",
			f.event,
			f.owner.ID,
		)
	}
	raw, _ = json.Marshal(proof)
	f.exec("UPDATE synthetic_moderation_contexts SET document=$1", raw)
	rollback := errors.New("synthetic owning decision rollback")
	err = database.Within(ctx, f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		if _, err := f.s.SignOffInTransaction(
			ctx,
			tx,
			f.owner.ID,
			f.event,
			f.team,
			id,
			refs,
		); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	var count int
	if err = f.pool.QueryRow(ctx, "SELECT count(*) FROM mentor_scorecard_signoffs").
		Scan(&count); err != nil ||
		count != 0 {
		t.Fatal("failed owning transaction retained signoff", err)
	}
	if err = sign(); err != nil {
		t.Fatal(err)
	}
	if err = sign(); err != nil {
		t.Fatal("idempotent exact signoff failed", err)
	}
	if err = f.pool.QueryRow(ctx, "SELECT count(*) FROM mentor_scorecard_signoffs").
		Scan(&count); err != nil ||
		count != 1 {
		t.Fatal("signoff replay created records", err)
	}

	for _, query := range []string{"UPDATE mentor_scorecard_versions SET document='{}' WHERE event_id=$1", "DELETE FROM mentor_scorecard_versions WHERE event_id=$1", "UPDATE mentor_scorecard_signoffs SET proof='{}' WHERE event_id=$1", "DELETE FROM mentor_scorecard_signoff_cards WHERE event_id=$1"} {
		if _, err = f.pool.Exec(ctx, query, f.event); err == nil {
			t.Fatal("immutable scored/source proof was rewritten", query)
		}
	}
	if _, err = f.pool.Exec(ctx, "TRUNCATE mentor_scorecard_signoffs CASCADE"); err == nil {
		t.Fatal("signed source history truncated")
	}
	changed := proof
	changed.Binding = "changed-source-without-new-version"
	raw, _ = json.Marshal(changed)
	f.exec("UPDATE synthetic_moderation_contexts SET document=$1", raw)
	if err = sign(); !errors.Is(err, access.ErrConflict) {
		t.Fatal("same source version silently changed retained signoff proof", err)
	}
	raw, _ = json.Marshal(proof)
	f.exec("UPDATE synthetic_moderation_contexts SET document=$1", raw)
	f.exec(
		"UPDATE submission_states SET outcome='withdrawn' WHERE event_id=$1 AND team_id=$2",
		f.event,
		f.team,
	)
	if err = sign(); err == nil {
		t.Fatal("withdrawn frozen source retained finalisation authority")
	}
	f.exec(
		"UPDATE submission_states SET outcome='frozen' WHERE event_id=$1 AND team_id=$2",
		f.event,
		f.team,
	)
	f.exec("UPDATE synthetic_moderation_contexts SET active=false")
	if err = sign(); err == nil {
		t.Fatal("stale owning context retained signing authority")
	}
}
