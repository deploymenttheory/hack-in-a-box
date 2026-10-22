package presentation

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/credentials"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestPresentationUpgradeFromSchema23PreservesCredentialAndCommunicationState(t *testing.T) {
	pool := testdb.New(t, false)
	ctx := t.Context()
	if err := database.Migrate(ctx, pool, 23); err != nil {
		t.Fatal(err)
	}
	owner, mc := testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	q := dbgen.New(pool)
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mc')",
		eventID,
		mc.ID,
	); err != nil {
		t.Fatal(err)
	}
	// A synthetic key and supplied value exercise the existing domain instead of
	// inserting an invalid pretend ciphertext. Neither is printed in failures.
	raw, _ := json.Marshal(
		map[string]any{
			"active": "synthetic-upgrade",
			"keys": map[string]string{
				"synthetic-upgrade": base64.StdEncoding.EncodeToString(
					bytes.Repeat([]byte{31}, 32),
				),
			},
		},
	)
	keys, err := secretbox.Parse(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	credentialService := credentials.Service{Pool: pool, Keys: keys}
	row, err := q.Event(ctx, eventID)
	if err != nil {
		t.Fatal(err)
	}
	page, err := credentialService.Change(
		ctx,
		owner.ID,
		eventID,
		row.Revision,
		credentials.Command{
			Action: "create",
			Label:  "Synthetic retained credential",
			Value:  "Synthetic upgrade-only supplied value",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatal("missing pre-upgrade credential")
	}
	before, err := q.TeamCredential(
		ctx,
		dbgen.TeamCredentialParams{EventID: eventID, ID: page.Items[0].ID},
	)
	if err != nil {
		t.Fatal(err)
	}
	comm := communications.Service{Pool: pool}
	state, err := comm.Read(ctx, owner.ID, eventID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = comm.Change(
		ctx,
		owner.ID,
		eventID,
		state.Revision,
		communications.Command{
			Action: "save_template",
			Template: &communications.Template{
				ID:      uuid.New(),
				Name:    "Synthetic existing notice",
				Purpose: "reminder",
				Format:  "text",
				Subject: "Existing event notice",
				Body:    "Synthetic unchanged body",
			},
			Reason: "Preserve existing communications",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	eventBefore, err := q.Event(ctx, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if err = database.CheckSchema(ctx, pool); !errors.Is(err, database.ErrSchema) {
		t.Fatalf("old schema unexpectedly accepted by schema24 app: %v", err)
	}
	// Normal ordered Goose migration: no out-of-order or allow-missing policy.
	if err = database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	if err = database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
	after, err := q.TeamCredential(ctx, dbgen.TeamCredentialParams{EventID: eventID, ID: before.ID})
	if err != nil {
		t.Fatal(err)
	}
	if before.Version != after.Version || before.KeyID != after.KeyID ||
		!bytes.Equal(before.Ciphertext, after.Ciphertext) ||
		before.Active != after.Active {
		t.Fatal("schema24 altered the existing supplied credential")
	}
	messages, err := comm.Read(ctx, owner.ID, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages.Templates) != 1 || messages.Templates[0].Version != 1 ||
		messages.Templates[0].Body != "Synthetic unchanged body" {
		t.Fatal("schema24 altered existing communication history")
	}
	service := Service{Pool: pool}
	program, err := service.Read(ctx, mc.ID, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if program.Revision != 0 || len(program.Draft) != 0 || len(program.Published) != 0 ||
		len(program.Pairings) != 0 {
		t.Fatal("upgrade invented room cues or display capabilities")
	}
	paired, err := service.Pair(
		ctx,
		mc.ID,
		eventID,
		program.Revision,
		PairInput{Label: "Post-upgrade room", Minutes: 60, Reason: "Verify post-upgrade pairing"},
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.Snapshot(ctx, eventID, paired.Token)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.EventName != eventBefore.Name || snapshot.Current != nil || snapshot.Next != nil {
		t.Fatal("post-upgrade capability inherited unrelated application data")
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
