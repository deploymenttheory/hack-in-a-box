package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

// Prior34 fixtures use explicit old columns: current generated queries include
// new35 fields and must not be used to manufacture an apparent upgrade proof.
type priorNativeInbox struct{}

func (priorNativeInbox) RecordClarificationInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, answerID uuid.UUID,
	version int64,
	people []uuid.UUID,
) error {
	for _, person := range people {
		if _, err := tx.Exec(
			ctx,
			`INSERT INTO inbox_notifications(id,event_id,recipient_id,source_kind,source_id,source_version,subject,body,format)VALUES($1,$2,$3,'clarification',$4,$5,'Retained native answer','Retained safe common notification','text')`,
			uuid.New(),
			eventID,
			person,
			answerID,
			version,
		); err != nil {
			return fmt.Errorf("retain prior native inbox fixture: %w", err)
		}
	}
	return nil
}

func TestAutomation35PreservesPopulated34AndGuardsPauseOnlyRollback(t *testing.T) {
	const prior, protected int64 = 34, 35
	pool := testdb.New(t, false)
	ctx := t.Context()
	if err := database.Migrate(ctx, pool, prior); err != nil {
		t.Fatal(err)
	}
	f := newFixtureWithPool(t, pool)
	f.campaign("reminder", "Retained approved template", true)
	rubrics, content, challenge, team := prepareNativeCommonChallenge(t, f)
	if err := content.Dispatch(ctx, f.id, challenge.ID, challenge.Revision); err != nil {
		t.Fatal(err)
	}

	f.now = f.now.Add(time.Minute)
	if err := content.Dispatch(ctx, f.id, challenge.ID, challenge.Revision); err != nil {
		t.Fatal(err)
	}
	nativeHelp := help.Service{Pool: pool, Clock: func() time.Time { return f.now }}
	if _, err := nativeHelp.Create(
		ctx,
		f.person.ID,
		f.id,
		uuid.New(),
		help.Input{
			TeamID:      team,
			Category:    "setup",
			Description: "PRIVATE-HELP-MIGRATION-CANARY",
			Location:    "Retained room",
			CommandID:   uuid.New(),
		},
	); err != nil {
		t.Fatal(err)
	}
	answers := clarification.Service{
		Pool:          pool,
		Clock:         func() time.Time { return f.now },
		Notifications: priorNativeInbox{},
	}
	question, err := answers.Ask(
		ctx,
		f.person.ID,
		f.id,
		clarification.QuestionInput{
			TeamID:    team,
			CommandID: uuid.New(),
			Topic:     "challenge",
			Body:      "PRIVATE-QUESTION-MIGRATION-CANARY",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := answers.SaveAnswer(
		ctx,
		f.owner.ID,
		f.id,
		question.ID,
		0,
		clarification.AnswerDocument{
			Title: "Retained common answer",
			Body:  "Retained public clarification",
			Kind:  "clarification",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"approve", "publish"} {
		answer, err = answers.ActAnswer(
			ctx,
			f.owner.ID,
			f.id,
			answer.ID,
			answer.Revision,
			clarification.AnswerCommand{Action: action, Version: answer.Version},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	seedPriorScorecard(t, f, rubrics, team)
	seedPriorProviders(t, f)
	snapshot := func() map[string]string { return prior34Snapshot(t, f) }
	before := snapshot()
	if err = database.Migrate(ctx, pool, protected); err != nil {
		t.Fatal(err)
	}
	assertSnapshotEqual(t, before, snapshot())
	var invented int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM communication_automation_versions)+(SELECT count(*) FROM communication_occurrences)+(SELECT count(*) FROM communication_publication_approvals)`).
		Scan(&invented); err != nil ||
		invented != 0 {
		t.Fatalf("upgrade invented automation intent: %d %v", invented, err)
	}
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		os.DirFS("../database/migrations"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(ctx, prior); err != nil {
		t.Fatal("empty35 cannot roll back without changing retained34 data", err)
	}
	assertSnapshotEqual(t, before, snapshot())
	if err = database.Migrate(ctx, pool, protected); err != nil {
		t.Fatal(err)
	}
	// A pause-only event has no binding, occurrence or publication plan; its
	// durable settings and journal still make rollback a preservation decision.
	f.change(Command{Action: "pause", Reason: "Retain pause-only operational decision"})
	retained := snapshot()
	if _, err = provider.DownTo(ctx, prior); err == nil {
		t.Fatal("rollback discarded pause-only settings and journal")
	}
	version, err := provider.GetDBVersion(ctx)
	if err != nil || version != protected {
		t.Fatalf("failed rollback changed schema: %d %v", version, err)
	}
	assertSnapshotEqual(t, retained, snapshot())
	var paused bool
	var history int
	if err = pool.QueryRow(ctx, `SELECT paused,(SELECT count(*) FROM communication_automation_history WHERE event_id=$1) FROM communication_automation_settings WHERE event_id=$1`, f.id).
		Scan(&paused, &history); err != nil ||
		!paused ||
		history != 1 {
		t.Fatalf("failed rollback lost pause journal: %v %d %v", paused, history, err)
	}
	if err = database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	if err = database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
}

func seedPriorScorecard(t *testing.T, f *fixture, rs rubric.Service, team uuid.UUID) {
	t.Helper()
	ctx := t.Context()
	configuration, err := rs.Configuration(ctx, f.owner.ID, f.id, 0)
	if err != nil {
		t.Fatal(err)
	}
	configuration, err = rs.Change(
		ctx,
		f.owner.ID,
		f.id,
		configuration.Revision,
		rubric.Command{
			Action:  "create_run",
			Version: configuration.Reference.Version,
			Purpose: "checkpoint",
			Reason:  "Retained native checkpoint",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	row, err := dbgen.New(f.pool).Event(ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	var cfg event.Configuration
	if err = json.Unmarshal(row.Configuration, &cfg); err != nil {
		t.Fatal(err)
	}
	mentor := testdb.Person(t, f.pool)
	f.exec(
		`INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'mentor')`,
		f.id,
		mentor.ID,
	)
	f.exec(
		`INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at)VALUES($1,$2,$3,'primary','published',$4,$5)`,
		f.id,
		team,
		mentor.ID,
		cfg.LiveStart,
		cfg.LiveEnd,
	)
	f.now = cfg.LiveStart.Add(time.Minute)
	f.exec(`UPDATE events SET stage='live' WHERE id=$1`, f.id)
	service := scorecards.Service{Pool: f.pool, Now: func() time.Time { return f.now }}
	loc, _ := time.LoadLocation(cfg.Timezone)
	row, err = dbgen.New(f.pool).Event(ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	cp, err := service.Configure(
		ctx,
		f.owner.ID,
		f.id,
		row.Revision,
		scorecards.CheckpointCommand{
			Definition: scorecards.CheckpointDefinition{
				ID:             uuid.New(),
				Name:           "Retained live checkpoint",
				RunID:          configuration.Runs[0].ID,
				OpensWallTime:  cfg.LiveStart.In(loc).Format("2006-01-02T15:04:05"),
				ClosesWallTime: cfg.LiveStart.Add(time.Hour).In(loc).Format("2006-01-02T15:04:05"),
			},
			Reason: "Retain prior native independent score",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	scores := []scorecards.CategoryScore{}
	for _, category := range configuration.Definition.Categories {
		scores = append(
			scores,
			scorecards.CategoryScore{
				CategoryID: category.ID,
				Pending:    true,
				Evidence: []scorecards.EvidenceReference{
					{Kind: "note", Note: "PRIVATE-CARD-MIGRATION-CANARY"},
				},
			},
		)
	}
	row, err = dbgen.New(f.pool).Event(ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Change(
		ctx,
		mentor.ID,
		f.id,
		team,
		row.Revision,
		scorecards.CardCommand{
			Action:            "save",
			CheckpointID:      cp.Definition.ID,
			CheckpointVersion: cp.Version,
			Scores:            scores,
		},
	); err != nil {
		t.Fatal(err)
	}
}

func seedPriorProviders(t *testing.T, f *fixture) {
	t.Helper()
	ctx := t.Context()
	operation, mailOperation, connection, delivery, attempt, mailAttempt := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	f.exec(
		`INSERT INTO operations(id,event_id,operation_key,kind,target_id,due_at,approved,state)VALUES($1,$3,'retained-teams','teams_workflow',$4,$5,true,'delivered'),($2,$3,'retained-smtp','communication_campaign_smtp',$6,$5,true,'delivered')`,
		operation,
		mailOperation,
		f.id,
		delivery,
		f.now,
		uuid.New(),
	)
	f.exec(
		`INSERT INTO teams_workflow_connections(id,event_id,version,label,owner_label,co_owner_label,key_id,ciphertext,active,updated_at)VALUES($1,$2,1,'Retained connection','Owner','Co-owner','synthetic',decode('010203','hex'),false,$3)`,
		connection,
		f.id,
		f.now,
	)
	var preview uuid.UUID
	if err := f.pool.QueryRow(ctx, `SELECT id FROM communication_previews WHERE event_id=$1 ORDER BY created_at DESC LIMIT 1`, f.id).
		Scan(&preview); err != nil {
		t.Fatal(err)
	}
	f.exec(
		`INSERT INTO teams_workflow_deliveries(event_id,id,operation_id,kind,connection_id,connection_version,channel_revision,actor_id,campaign_id,campaign_version,preview_id,preview_fingerprint,request_id,created_at)VALUES($1,$2,$3,'broadcast',$4,1,'1',$5,$6,$7,$8,'retained',gen_random_uuid(),$9)`,
		f.id,
		delivery,
		operation,
		connection,
		f.owner.ID,
		f.campaignID,
		f.campaignVersion,
		preview,
		f.now,
	)
	f.exec(
		`INSERT INTO teams_workflow_attempts(event_id,id,delivery_id,attempt_number,connection_version,payload_fingerprint,started_at)VALUES($1,$2,$3,1,1,'retained',$4)`,
		f.id,
		attempt,
		delivery,
		f.now,
	)
	f.exec(
		`INSERT INTO teams_workflow_receipts(event_id,attempt_id,outcome,http_status,reason,recorded_at)VALUES($1,$2,'provider_accepted',202,'provider_accepted',$3)`,
		f.id,
		attempt,
		f.now,
	)
	f.exec(
		`INSERT INTO smtp_campaign_messages(operation_id,event_id,intent_id,campaign_id,campaign_version,approval_binding,occurrence_id,occurrence_binding,provider_revision,person_id,mailbox,snapshot,state,receipt_code)VALUES($1,$2,gen_random_uuid(),$3,$4,'retained','00000000-0000-0000-0000-000000000000','retained','old-relay',$5,'synthetic@example.test','{"body":"PRIVATE-SMTP-MIGRATION-CANARY"}','accepted','relay_accepted')`,
		mailOperation,
		f.id,
		f.campaignID,
		f.campaignVersion,
		f.person.ID,
	)
	f.exec(
		`INSERT INTO smtp_submission_attempts(id,operation_id,source_kind,attempt_number)VALUES($1,$2,'campaign',1)`,
		mailAttempt,
		mailOperation,
	)
	f.exec(
		`INSERT INTO smtp_submission_receipts(attempt_id,outcome,receipt_code)VALUES($1,'accepted','relay_accepted')`,
		mailAttempt,
	)
}

func prior34Snapshot(t *testing.T, f *fixture) map[string]string {
	t.Helper()
	result := map[string]string{}
	tables := []string{
		"events",
		"hack_teams",
		"competition_entries",
		"competition_claims",
		"event_memberships",
		"mentor_assignments",
		"content_items",
		"content_versions",
		"rubric_publications",
		"help_requests",
		"help_transitions",
		"mentor_scorecard_versions",
		"scorecard_checkpoint_versions",
		"clarification_questions",
		"clarification_answers",
		"clarification_versions",
		"clarification_publications",
		"communication_campaign_versions",
		"inbox_notifications",
		"operations",
		"smtp_campaign_messages",
		"smtp_submission_attempts",
		"smtp_submission_receipts",
		"teams_workflow_connections",
		"teams_workflow_deliveries",
		"teams_workflow_attempts",
		"teams_workflow_receipts",
		"audit_events",
	}
	for _, table := range tables {
		column, filter := "event_id", ""
		if table == "events" {
			column = "id"
		}
		switch table {
		case "operations":
			filter = "-'retained_occurrence_id'"
		case "smtp_campaign_messages", "inbox_notifications":
			filter = "-'scope_approval'"
		case "teams_workflow_deliveries":
			filter = "-'scope_approval'-'scope_binding'-'occurrence_id'"
		}
		where := column + "=$1"
		if table == "smtp_submission_attempts" {
			where = "operation_id IN(SELECT id FROM operations WHERE event_id=$1)"
		}
		if table == "smtp_submission_receipts" {
			where = "attempt_id IN(SELECT a.id FROM smtp_submission_attempts a JOIN operations o ON o.id=a.operation_id WHERE o.event_id=$1)"
		}
		query := fmt.Sprintf(
			"SELECT COALESCE(jsonb_agg(to_jsonb(t)%s ORDER BY to_jsonb(t)%s),'[]')::text FROM %s t WHERE %s",
			filter,
			filter,
			table,
			where,
		)
		var value string
		if err := f.pool.QueryRow(t.Context(), query, f.id).Scan(&value); err != nil {
			t.Fatal(err)
		}
		result[table] = value
	}
	return result
}

func assertSnapshotEqual(t *testing.T, before, after map[string]string) {
	t.Helper()
	for table, value := range before {
		if after[table] != value {
			t.Fatal("migration changed retained prior source", table)
		}
	}
}
