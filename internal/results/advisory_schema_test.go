package results

import (
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestMentorAdvisoryUpgradeRetainsPopulatedParentAndCorrectionHistory(t *testing.T) {
	pool := testdb.New(t, false)
	ctx := t.Context()
	if err := database.Migrate(ctx, pool, 54); err != nil {
		t.Fatal(err)
	}
	actor := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, actor.ID)
	ref := testdb.ApprovedRubric(t, pool, actor.ID, eventID)
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO result_versions(event_id,version,actor_id,action,reason,document,changed_at)VALUES($1,1,$2,'prepare','Synthetic retained parent','{}',now())",
		eventID,
		actor.ID,
	); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, pool, 55); err != nil {
		t.Fatal(err)
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
	if _, err = provider.DownTo(ctx, 54); err != nil {
		t.Fatal("empty advisory rollback", err)
	}
	if err = database.Migrate(ctx, pool, 55); err != nil {
		t.Fatal(err)
	}
	var parents int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM result_versions WHERE event_id=$1", eventID).
		Scan(&parents); err != nil ||
		parents != 1 {
		t.Fatal("upgrade/empty rollback lost populated parent", parents, err)
	}
	teamID, runID := uuid.New(), uuid.New()
	if _, err = pool.Exec(
		ctx,
		"INSERT INTO hack_teams(event_id,id,name,state)VALUES($1,$2,'Synthetic schema team','draft')",
		eventID,
		teamID,
	); err != nil {
		t.Fatal(err)
	}
	if err = dbgen.New(pool).
		CreateCompetitionEntry(ctx, dbgen.CreateCompetitionEntryParams{EventID: eventID, TeamID: teamID}); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(
		ctx,
		"INSERT INTO rubric_assessment_runs(event_id,id,rubric_id,rubric_version,purpose,status,reason,actor_id,created_at)VALUES($1,$2,$3,$4,'judge_final','current','Synthetic schema reference',$5,now())",
		eventID,
		runID,
		ref.ID,
		ref.Version,
		actor.ID,
	); err != nil {
		t.Fatal(err)
	}
	binding := strings.Repeat("a", 64)
	for _, version := range []int64{1, 2} {
		if err = dbgen.New(pool).
			InsertResultMentorAdvisory(ctx, dbgen.InsertResultMentorAdvisoryParams{EventID: eventID, TieID: binding, TeamID: teamID, AuthorID: actor.ID, Version: version, JudgeRunID: runID, SourceBinding: binding, Document: []byte(`{}`)}); err != nil {
			t.Fatal(err)
		}
	}
	if err = dbgen.New(pool).
		InsertResultMentorAdvisoryReceipt(ctx, dbgen.InsertResultMentorAdvisoryReceiptParams{EventID: eventID, RequestID: uuid.New(), TieID: binding, TeamID: teamID, AuthorID: actor.ID, Version: 2, Receipt: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"result_mentor_advisory_versions", "result_mentor_advisory_commands"} {
		for _, statement := range []string{"UPDATE " + table + " SET event_id=event_id", "DELETE FROM " + table, "TRUNCATE " + table + " CASCADE"} {
			if _, err = pool.Exec(ctx, statement); err == nil {
				t.Fatal("retained advisory history mutable", statement)
			}
		}
	}
	foreign := testdb.Event(t, pool, actor.ID)
	if err = dbgen.New(pool).
		InsertResultMentorAdvisory(ctx, dbgen.InsertResultMentorAdvisoryParams{EventID: foreign, TieID: binding, TeamID: teamID, AuthorID: actor.ID, Version: 1, JudgeRunID: runID, SourceBinding: binding, Document: []byte(`{}`)}); err == nil {
		t.Fatal("cross-event source accepted")
	}
	if _, err = provider.DownTo(ctx, 54); err == nil {
		t.Fatal("populated advisory rollback erased history")
	}
	version, err := provider.GetDBVersion(ctx)
	if err != nil || version != 55 {
		t.Fatal("retained schema changed", version, err)
	}
	var retained int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM result_mentor_advisory_versions WHERE event_id=$1", eventID).
		Scan(&retained); err != nil ||
		retained != 2 {
		t.Fatal("correction history changed", retained, err)
	}
	if err = database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
}
