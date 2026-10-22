package smtpintegration

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestSMTPUpgradeFromSchema26PreservesPeopleCampaignsAndOperations(t *testing.T) {
	pool := testdb.New(t, false)
	if err := database.Migrate(t.Context(), pool, 26); err != nil {
		t.Fatal(err)
	}
	person := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, person.ID)
	campaignID := uuid.New()
	document := []byte(`{"synthetic":"retained reviewed campaign"}`)
	if _, err := pool.Exec(
		t.Context(),
		"INSERT INTO communication_campaign_versions(event_id,id,version,document,actor_id,reason) VALUES($1,$2,1,$3,$4,'Synthetic upgrade preservation')",
		eventID,
		campaignID,
		document,
		person.ID,
	); err != nil {
		t.Fatal(err)
	}
	q := dbgen.New(pool)
	// Explicit prior26 columns avoid using a current query with new35 fields.
	operation := dbgen.Operation{
		ID:           uuid.New(),
		EventID:      eventID,
		TargetID:     campaignID,
		Kind:         "synthetic_existing_operation",
		OperationKey: "synthetic_existing_operation",
		DueAt: time.Now().
			UTC().
			Truncate(time.Microsecond),
		Approved: true,
		State:    "pending",
		Revision: 1,
	}
	err := pool.QueryRow(t.Context(), `INSERT INTO operations(id,event_id,target_id,kind,operation_key,due_at,approved) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING due_at,created_at,updated_at`,
		operation.ID, operation.EventID, operation.TargetID, operation.Kind, operation.OperationKey, operation.DueAt, operation.Approved).
		Scan(&operation.DueAt, &operation.CreatedAt, &operation.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	beforeEvent, err := q.Event(t.Context(), eventID)
	if err != nil {
		t.Fatal(err)
	}
	if err = database.Migrate(t.Context(), pool, 0); err != nil {
		t.Fatal(err)
	}
	if err = database.CheckSchema(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	afterPerson, err := q.Person(t.Context(), person.ID)
	if err != nil || afterPerson != person {
		t.Fatal("SMTP upgrade changed existing identity")
	}
	afterEvent, err := q.Event(t.Context(), eventID)
	beforeJSON, _ := json.Marshal(beforeEvent)
	afterJSON, _ := json.Marshal(afterEvent)
	if err != nil || string(beforeJSON) != string(afterJSON) {
		t.Fatal("SMTP upgrade changed event authority or configuration")
	}
	retainedOperation, err := q.Operation(t.Context(), operation.ID)
	if err != nil || retainedOperation != operation {
		t.Fatal("SMTP upgrade changed existing durable operation")
	}
	var retainedDocument []byte
	if err = pool.QueryRow(t.Context(), "SELECT document FROM communication_campaign_versions WHERE event_id=$1 AND id=$2 AND version=1", eventID, campaignID).
		Scan(&retainedDocument); err != nil {
		t.Fatal(err)
	}
	var retained, expected map[string]string
	if json.Unmarshal(retainedDocument, &retained) != nil ||
		json.Unmarshal(document, &expected) != nil ||
		retained["synthetic"] != expected["synthetic"] {
		t.Fatal("SMTP upgrade changed reviewed campaign document")
	}
	for _, table := range []string{"person_email_contacts", "email_verifications", "smtp_message_snapshots", "smtp_campaign_messages", "email_calendar_sequences", "smtp_submission_attempts", "smtp_submission_receipts", "smtp_campaign_omissions"} {
		var count int
		if err = pool.QueryRow(t.Context(), "SELECT count(*) FROM "+table).
			Scan(&count); err != nil ||
			count != 0 {
			t.Fatal("SMTP upgrade inferred private mailboxes or created external-effect evidence")
		}
	}
}
