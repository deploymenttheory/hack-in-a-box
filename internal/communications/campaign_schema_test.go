package communications

import (
	"bytes"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestCampaignUpgradeFromSchema25PreservesExistingSourceHistory(t *testing.T) {
	pool := testdb.New(t, false)
	ctx := t.Context()
	if err := database.Migrate(ctx, pool, 25); err != nil {
		t.Fatal(err)
	}
	owner := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	service := Service{Pool: pool}
	before, err := service.Read(ctx, owner.ID, eventID)
	if err != nil {
		t.Fatal(err)
	}
	template := Template{
		ID:      uuid.New(),
		Name:    "Existing synthetic notice",
		Purpose: "reminder",
		Format:  "text",
		Subject: "Existing subject",
		Body:    "Unchanged {{event.name}}",
	}
	before, err = service.Change(
		ctx,
		owner.ID,
		eventID,
		before.Revision,
		Command{
			Action:   "save_template",
			Template: &template,
			Reason:   "Preserve source across upgrade",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	q := dbgen.New(pool)
	eventBefore, err := q.Event(ctx, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if err = database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	if err = database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
	after, err := service.Read(ctx, owner.ID, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Templates) != 1 || after.Templates[0].Version != 1 ||
		after.Templates[0].Body != template.Body ||
		after.Revision != before.Revision {
		t.Fatal("upgrade changed existing communications history")
	}
	campaigns, err := service.Campaigns(ctx, owner.ID, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if len(campaigns.Campaigns) != 0 {
		t.Fatal("upgrade invented campaigns or approvals")
	}
	var pending int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM operations WHERE event_id=$1", eventID).
		Scan(&pending); err != nil ||
		pending != 0 {
		t.Fatalf("upgrade invented delivery operations: %d %v", pending, err)
	}
	eventAfter, err := q.Event(ctx, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if eventBefore.Revision != eventAfter.Revision ||
		!bytes.Equal(eventBefore.Configuration, eventAfter.Configuration) {
		t.Fatal("upgrade changed existing event state")
	}
}
