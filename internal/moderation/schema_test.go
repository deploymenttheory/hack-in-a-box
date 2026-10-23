package moderation

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func priorDataSnapshot(t *testing.T, pool *pgxpool.Pool) map[string]string {
	t.Helper()
	rows, err := pool.Query(
		t.Context(),
		`SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename NOT LIKE 'goose%' ORDER BY tablename`,
	)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, name := range names {
		var value string
		if err = pool.QueryRow(t.Context(), `SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text),'[]'::jsonb)::text FROM `+pgx.Identifier{name}.Sanitize()+` t`).
			Scan(&value); err != nil {
			t.Fatal(err)
		}
		out[name] = value
	}
	return out
}

func schemaMigrator(t *testing.T, pool *pgxpool.Pool) *goose.Provider {
	t.Helper()
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })
	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		os.DirFS("../database/migrations"),
	)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func TestModeration37PreservesAllPopulated36TablesAndGuardsCanonicalHistory(t *testing.T) {
	pool := testdb.New(t, false)
	ctx := t.Context()
	if err := database.Migrate(ctx, pool, 36); err != nil {
		t.Fatal(err)
	}
	f := setupPool(t, pool)
	f.submit(f.checkpoint, f.primary.ID, true)
	f.submit(f.checkpoint, f.second.ID, true)
	native, feedbackID, task := f.nativeAction()
	questionOwner := clarification.Service{Pool: pool, Clock: func() time.Time { return f.now }}
	question, err := questionOwner.Ask(
		ctx,
		f.member.ID,
		f.event,
		clarification.QuestionInput{
			TeamID:    f.team,
			CommandID: uuid.New(),
			Topic:     "rubric",
			Body:      "PRIVATE schema36 question retained exactly",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	helpOwner := help.Service{Pool: pool, Clock: func() time.Time { return f.now }}
	request, err := helpOwner.Create(
		ctx,
		f.member.ID,
		f.event,
		uuid.New(),
		help.Input{
			TeamID:      f.team,
			Category:    "setup",
			Description: "PRIVATE schema36 help retained exactly",
			Location:    "Synthetic meeting point",
			CommandID:   uuid.New(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(
		`INSERT INTO inbox_notifications(id,event_id,recipient_id,source_kind,source_id,source_version,subject,body,format)VALUES($1,$2,$3,'help',$4,$5,'Existing safe help update','','text')`,
		uuid.New(),
		f.event,
		f.member.ID,
		request.Request.ID,
		request.Request.Revision,
	)
	f.exec(
		`INSERT INTO communication_automation_settings(event_id,paused,actor_id,reason)VALUES($1,true,$2,'Existing explicit campaign pause')`,
		f.event,
		f.owner.ID,
	)
	before := priorDataSnapshot(t, pool)
	if err = database.Migrate(ctx, pool, 37); err != nil {
		t.Fatal(err)
	}
	if err = database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
	after := priorDataSnapshot(t, pool)
	for table, value := range before {
		if after[table] != value {
			t.Fatal("36→37 changed retained table", table)
		}
	}
	if _, err = questionOwner.Question(ctx, f.member.ID, f.event, question.ID); err != nil {
		t.Fatal("prior native question permission changed", err)
	}
	if _, err = helpOwner.Read(ctx, f.member.ID, f.event, request.Request.ID); err != nil {
		t.Fatal("prior native help permission changed", err)
	}
	if _, err = native.Read(ctx, f.member.ID, f.event, feedbackID); err != nil {
		t.Fatal("prior native feedback permission changed", err)
	}
	s := f.huddles()
	planned := f.create(s, f.checkpoint)
	migrator := schemaMigrator(t, pool)
	if _, err = migrator.DownTo(ctx, 36); err == nil {
		t.Fatal("rollback discarded planned private moderation history")
	}
	requireProtected37(t, ctx, migrator)
	if _, err = s.Read(ctx, f.owner.ID, f.event, f.team, planned.Definition.ID); err != nil {
		t.Fatal("guarded rollback lost planned huddle", err)
	}
	final := f.configure("mentor_final")
	f.freeze()
	f.now = f.config.SubmissionCutoff
	f.submit(final, f.primary.ID, true)
	f.submit(final, f.second.ID, true)
	h := f.create(s, final)
	h = f.change(s, h, f.second.ID, Command{Action: "complete_second_review"})
	h = f.change(s, h, f.owner.ID, Command{Action: "open"})
	task = f.actionStatus(native, feedbackID, task, "done")
	gap := uuid.New()
	h = f.change(
		s,
		h,
		f.owner.ID,
		Command{
			Action:        "add_gap",
			GapID:         gap,
			ActionID:      task.Action.ID,
			ActionVersion: task.Action.Version,
			Reason:        "Retain exact native feedback/action FK evidence",
		},
	)
	h = f.change(
		s,
		h,
		f.owner.ID,
		Command{
			Action:        "resolve_gap",
			GapID:         gap,
			ActionVersion: task.Action.Version,
			Evidence: []scorecards.EvidenceReference{
				{Kind: "note", Note: "Explicit current frozen output formal review"},
			},
			Reason: "Done action plus independent formal evidence decision",
		},
	)
	h = f.change(
		s,
		h,
		f.owner.ID,
		Command{
			Action: "sign_off",
			Reason: "Canonical exact final proof retained through failed rollback",
		},
	)
	if _, err = migrator.DownTo(ctx, 36); err == nil {
		t.Fatal("rollback discarded canonical proof and source links")
	}
	requireProtected37(t, ctx, migrator)
	var decisions, proofs, links int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM moderation_final_decisions WHERE event_id=$1),(SELECT count(*) FROM mentor_scorecard_signoffs WHERE event_id=$1),(SELECT count(*) FROM moderation_gap_actions WHERE event_id=$1)`, f.event).
		Scan(&decisions, &proofs, &links); err != nil ||
		decisions != 1 ||
		proofs != 1 ||
		links < 1 {
		t.Fatal("failed rollback lost canonical/source history", decisions, proofs, links, err)
	}
	for _, table := range []string{"moderation_huddle_versions", "moderation_huddle_cards", "moderation_gap_actions", "moderation_final_decisions"} {
		if _, err = pool.Exec(
			ctx,
			`DELETE FROM `+pgx.Identifier{table}.Sanitize()+` WHERE event_id=$1`,
			f.event,
		); err == nil {
			t.Fatal("private immutable history was deletable", table)
		}
	}
	if _, err = s.Read(ctx, f.owner.ID, f.event, f.team, h.Definition.ID); err != nil {
		t.Fatal("native canonical proof unreadable after guarded rollback", err)
	}
}

func requireProtected37(t *testing.T, ctx context.Context, migrator *goose.Provider) {
	t.Helper()
	version, err := migrator.GetDBVersion(ctx)
	if err != nil || version != 37 {
		t.Fatal("failed rollback crossed explicit moderation37 preservation boundary", version, err)
	}
}

func TestModeration37EmptyRollbackRestoresNormalLatest(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	migrator := schemaMigrator(t, pool)
	if _, err := migrator.DownTo(ctx, 36); err != nil {
		t.Fatal("empty37 rollback unsupported", err)
	}
	if err := database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	if err := database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
}
