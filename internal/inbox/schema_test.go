package inbox

import (
	"bytes"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestInboxUpgradeFromSchema29PreservesRubricContactsAndSourceHistory(t *testing.T) {
	pool := testdb.New(t, false)
	ctx := t.Context()
	if err := database.Migrate(ctx, pool, 29); err != nil {
		t.Fatal(err)
	}
	owner := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO person_email_contacts(person_id,mailbox,provenance) VALUES($1,'synthetic-private@example.invalid','oidc_verified')",
		owner.ID,
	); err != nil {
		t.Fatal(err)
	}
	q := dbgen.New(pool)
	comm := communications.Service{Pool: pool}
	old, err := comm.Read(ctx, owner.ID, eventID)
	if err != nil {
		t.Fatal(err)
	}
	source := communications.Template{
		ID:      uuid.New(),
		Name:    "Existing synthetic source",
		Purpose: "reminder",
		Format:  "text",
		Subject: "Retained subject",
		Body:    "Retained body",
	}
	if _, err = comm.Change(
		ctx,
		owner.ID,
		eventID,
		old.Revision,
		communications.Command{
			Action:   "save_template",
			Template: &source,
			Reason:   "Preserve source history",
		},
	); err != nil {
		t.Fatal(err)
	}
	rubrics := rubric.Service{Pool: pool}
	rubricBefore, err := rubrics.Configuration(ctx, owner.ID, eventID, 0)
	if err != nil {
		t.Fatal(err)
	}
	definition := rubricBefore.Definition
	definition.PrivateJudgeGuidance = "Synthetic private judge instructions retained across inbox upgrade"
	rubricBefore, err = rubrics.Change(ctx, owner.ID, eventID, 0, rubric.Command{Action: "save", Definition: &definition, Reason: "Explicit synthetic rubric"})
	if err != nil {
		t.Fatal(err)
	}
	rubricBefore, err = rubrics.Change(ctx, owner.ID, eventID, rubricBefore.Revision, rubric.Command{Action: "approve", Version: rubricBefore.Reference.Version, Reason: "Explicit synthetic approval"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := q.Event(ctx, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if err = database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	if err = database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
	page, err := (Service{Pool: pool, Communications: &comm}).List(ctx, owner.ID, nil, nil, 20)
	if err != nil || len(page.Items) != 0 || page.Unread != 0 {
		t.Fatalf("upgrade invented inbox messages: %+v %v", page, err)
	}
	var mailbox, provenance string
	if err = pool.QueryRow(ctx, "SELECT mailbox,provenance FROM person_email_contacts WHERE person_id=$1", owner.ID).
		Scan(&mailbox, &provenance); err != nil ||
		mailbox != "synthetic-private@example.invalid" ||
		provenance != "oidc_verified" {
		t.Fatal("upgrade changed private verified mailbox")
	}
	current, err := comm.Read(ctx, owner.ID, eventID)
	if err != nil || len(current.Templates) != 1 || current.Templates[0].Body != source.Body ||
		current.Templates[0].Version != 1 {
		t.Fatal("upgrade changed existing source history")
	}
	rubricAfter, err := rubrics.Configuration(ctx, owner.ID, eventID, 0)
	if err != nil || rubricAfter.Reference != rubricBefore.Reference || !rubricAfter.Approved || rubricAfter.Definition.PrivateJudgeGuidance != definition.PrivateJudgeGuidance || rubricAfter.PublishedVersion != nil {
		t.Fatalf("upgrade changed approved unpublished rubric: %+v %v", rubricAfter, err)
	}
	after, err := q.Event(ctx, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Revision != after.Revision ||
		!bytes.Equal(before.Configuration, after.Configuration) {
		t.Fatal("upgrade changed event state")
	}
}

func TestInboxSchemaEmptyRollbackAndRetainedHistoryGate(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS("../database/migrations"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(ctx, 29); err != nil {
		t.Fatal("empty inbox rollback", err)
	}
	if err = database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	owner := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	if _, err = pool.Exec(ctx, "INSERT INTO inbox_notifications(id,event_id,recipient_id,source_kind,source_id,source_version,subject,body,format) VALUES($1,$2,$3,'test_self',$4,1,'Synthetic','Retained private history','text')", uuid.New(), eventID, owner.ID, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(ctx, 29); err == nil {
		t.Fatal("rollback discarded private notification history")
	}
	var count int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM inbox_notifications WHERE event_id=$1", eventID).Scan(&count); err != nil || count != 1 {
		t.Fatal("rollback changed delivered history", err)
	}
	if err = database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
}
