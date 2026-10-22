package clarification_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestClarificationUpgradePreservesPopulatedScorecardAndPrivateHelpHistory(t *testing.T) {
	const predecessor int64 = 33
	ctx := t.Context()
	pool := testdb.New(t, false)
	if err := database.Migrate(ctx, pool, predecessor); err != nil {
		t.Fatal(err)
	}
	f := setupPool(t, pool)
	f.release()
	owner, eventID, teamID := f.owner, f.eventID, f.team
	mentor := testdb.Person(t, pool)
	helpID, noteID := uuid.New(), uuid.New()
	row, err := dbgen.New(pool).Event(ctx, eventID)
	if err != nil {
		t.Fatal(err)
	}
	var dates event.Configuration
	if err = json.Unmarshal(row.Configuration, &dates); err != nil {
		t.Fatal(err)
	}
	f.now = dates.LiveStart
	f.exec("UPDATE events SET stage='live' WHERE id=$1", eventID)
	f.exec(
		"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mentor')",
		eventID,
		mentor.ID,
	)
	f.exec(
		"INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at) VALUES($1,$2,$3,'primary','published',$4,$5)",
		eventID,
		teamID,
		mentor.ID,
		dates.LiveStart,
		dates.LiveEnd,
	)
	f.exec(
		"INSERT INTO policy_resources(event_id,id,kind) VALUES($1,$2,'team'),($1,$3,'help')",
		eventID,
		teamID,
		helpID,
	)
	f.exec(
		"INSERT INTO help_requests(event_id,id,team_id,requester_id,revision,category,description,location,meeting_url,status,waiting_cycle,requested_at,waiting_since,updated_at) VALUES($1,$2,$3,$4,1,'logistics','PRIVATE-PRE34-HELP','PRIVATE-PRE34-LOCATION','','waiting',1,$5,$5,$5)",
		eventID,
		helpID,
		teamID,
		f.first,
		f.now,
	)
	f.exec(
		"INSERT INTO help_transitions(event_id,request_id,revision,action,status,actor_id,occurred_at) VALUES($1,$2,1,'create','waiting',$3,$4)",
		eventID,
		helpID,
		f.first,
		f.now,
	)
	f.exec(
		"INSERT INTO mentor_observation_versions(event_id,team_id,id,version,author_id,observed_at,changed_at,category,evidence,outcome) VALUES($1,$2,$3,1,$4,$5,$5,'technical','PRIVATE-PRE34-OBSERVATION','Retained outcome')",
		eventID,
		teamID,
		noteID,
		mentor.ID,
		f.now,
	)
	f.exec(
		"INSERT INTO inbox_notifications(id,event_id,recipient_id,source_kind,source_id,source_version,subject,body,format) VALUES($1,$2,$3,'test_self',$4,1,'Retained inbox','PRIVATE-PRE34-INBOX','text')",
		uuid.New(),
		eventID,
		owner,
		uuid.New(),
	)
	criteria := rubric.Service{Pool: pool, Clock: func() time.Time { return f.now }}
	config, err := criteria.Configuration(ctx, owner, eventID, 0)
	if err != nil {
		t.Fatal(err)
	}
	config, err = criteria.Change(
		ctx,
		owner,
		eventID,
		config.Revision,
		rubric.Command{
			Action:  "create_run",
			Version: config.Reference.Version,
			Purpose: "checkpoint",
			Reason:  "Retained actual assessment run",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	var runID uuid.UUID
	for _, run := range config.Runs {
		if run.Purpose == "checkpoint" {
			runID = run.ID
		}
	}
	revision := func() int64 {
		t.Helper()
		current, readErr := dbgen.New(pool).Event(ctx, eventID)
		if readErr != nil {
			t.Fatal(readErr)
		}
		return current.Revision
	}
	cards := scorecards.Service{Pool: pool, Now: func() time.Time { return f.now }}
	zone, err := time.LoadLocation(dates.Timezone)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := cards.Configure(
		ctx,
		owner,
		eventID,
		revision(),
		scorecards.CheckpointCommand{
			Definition: scorecards.CheckpointDefinition{
				ID:             uuid.New(),
				Name:           "Actual retained checkpoint",
				RunID:          runID,
				OpensWallTime:  dates.LiveStart.In(zone).Format("2006-01-02T15:04:05"),
				ClosesWallTime: dates.SubmissionCutoff.In(zone).Format("2006-01-02T15:04:05"),
			},
			Reason: "Retained explicit configuration",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	scores := []scorecards.CategoryScore{}
	for _, category := range checkpoint.Criteria.Categories {
		value := category.MinScore
		scores = append(
			scores,
			scorecards.CategoryScore{
				CategoryID: category.ID,
				Value:      &value,
				Evidence: []scorecards.EvidenceReference{
					{Kind: "note", Note: "PRIVATE-PRE34-SCORECARD"},
				},
			},
		)
	}
	card, err := cards.Change(
		ctx,
		mentor.ID,
		eventID,
		teamID,
		revision(),
		scorecards.CardCommand{
			Action:            "save",
			CheckpointID:      checkpoint.Definition.ID,
			CheckpointVersion: checkpoint.Version,
			Scores:            scores,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if card.Reference.ID == uuid.Nil || card.Reference.Version != 1 {
		t.Fatal("fixture did not persist an actual schema33 card")
	}
	retainedCard, err := json.Marshal(card)
	if err != nil {
		t.Fatal(err)
	}
	const snapshot = `SELECT jsonb_build_object(
 'event',(SELECT to_jsonb(e) FROM events e WHERE id=$1),
 'team',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM hack_teams t WHERE event_id=$1),
 'memberships',(SELECT jsonb_agg(to_jsonb(m) ORDER BY person_id,role) FROM event_memberships m WHERE event_id=$1),
 'help',(SELECT jsonb_agg(to_jsonb(h) ORDER BY id) FROM help_requests h WHERE event_id=$1),
 'help_history',(SELECT jsonb_agg(to_jsonb(h) ORDER BY request_id,revision) FROM help_transitions h WHERE event_id=$1),
 'observations',(SELECT jsonb_agg(to_jsonb(o) ORDER BY id,version) FROM mentor_observation_versions o WHERE event_id=$1),
 'rubric',(SELECT jsonb_agg(to_jsonb(r) ORDER BY rubric_id,version) FROM rubric_versions r WHERE event_id=$1),
 'runs',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM rubric_assessment_runs r WHERE event_id=$1),
 'checkpoints',(SELECT jsonb_agg(to_jsonb(c) ORDER BY id,version) FROM scorecard_checkpoint_versions c WHERE event_id=$1),
 'cards',(SELECT jsonb_agg(to_jsonb(c) ORDER BY id,version) FROM mentor_scorecard_versions c WHERE event_id=$1),
 'inbox',(SELECT jsonb_agg(to_jsonb(n) ORDER BY id) FROM inbox_notifications n WHERE event_id=$1),
 'audit',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM audit_events a WHERE event_id=$1)
 )::text`
	var before string
	if err := pool.QueryRow(ctx, snapshot, eventID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	assertPreserved := func() {
		t.Helper()
		var after string
		if err := pool.QueryRow(ctx, snapshot, eventID).
			Scan(&after); err != nil ||
			before != after {
			t.Fatal("clarification upgrade/empty rollback changed retained private state", err)
		}
		page, readErr := cards.Read(ctx, mentor.ID, eventID, teamID, uuid.Nil, 10)
		if readErr != nil || len(page.Items) != 1 {
			t.Fatal("retained native scorecard became unreadable", readErr)
		}
		actualCard, encodeErr := json.Marshal(page.Items[0])
		if encodeErr != nil || string(actualCard) != string(retainedCard) {
			t.Fatal("native scorecard projection changed across clarification migration", encodeErr)
		}
	}
	if err := database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	if err := database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
	assertPreserved()
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM clarification_questions").
		Scan(&count); err != nil ||
		count != 0 {
		t.Fatal("upgrade invented private questions", count, err)
	}
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()
	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		os.DirFS("../database/migrations"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(ctx, predecessor); err != nil {
		t.Fatal("empty clarification rollback", err)
	}
	assertPreserved()
	if err = database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	assertPreserved()
}

func TestClarificationPopulatedRollbackRetainsPrivateQuestion(t *testing.T) {
	f := setup(t)
	f.release()
	question, err := f.service.Ask(t.Context(), f.first, f.eventID, clarification.QuestionInput{
		TeamID:    f.team,
		CommandID: uuid.New(),
		Topic:     "challenge",
		Body:      "PRIVATE-ROLLBACK-QUESTION",
	})
	if err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDBFromPool(f.pool)
	defer func() { _ = db.Close() }()
	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		os.DirFS("../database/migrations"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(t.Context(), 33); err == nil {
		t.Fatal("rollback discarded private question history")
	}
	retained, err := f.service.Question(t.Context(), f.first, f.eventID, question.ID)
	if err != nil || retained.Body != question.Body || retained.Source != question.Source {
		t.Fatal("guarded rollback changed private question", err)
	}
	if err = database.CheckSchema(t.Context(), f.pool); err != nil {
		t.Fatal(err)
	}
}
