package presentation

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
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
	// Seed only schema23 columns: current domain queries may require columns
	// introduced by later migrations. Keep real authenticated ciphertext and
	// event/resource/version binding; never print the synthetic supplied value.
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
	credentialID := uuid.New()
	const supplied = "Synthetic upgrade-only supplied value"
	binding := []byte(fmt.Sprintf("hackbox:team-credential:v1:%s:%s:1", eventID, credentialID))
	keyID, ciphertext, err := keys.Seal([]byte(supplied), binding)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(
		ctx,
		"INSERT INTO team_credentials(id,event_id,label,key_id,ciphertext) VALUES($1,$2,'Synthetic retained credential',$3,$4)",
		credentialID,
		eventID,
		keyID,
		ciphertext,
	); err != nil {
		t.Fatal(err)
	}
	var before dbgen.TeamCredential
	if err = pool.QueryRow(ctx, "SELECT id,version,key_id,ciphertext,active FROM team_credentials WHERE event_id=$1 AND id=$2", eventID, credentialID).
		Scan(&before.ID, &before.Version, &before.KeyID, &before.Ciphertext, &before.Active); err != nil {
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
		t.Fatalf("old schema unexpectedly accepted by current app: %v", err)
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
		t.Fatal("upgrade altered the existing supplied credential")
	}
	decrypted, err := keys.Open(after.KeyID, after.Ciphertext, binding)
	if err != nil || string(decrypted) != supplied {
		t.Fatal("upgrade broke authenticated credential recovery")
	}
	messages, err := comm.Read(ctx, owner.ID, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages.Templates) != 1 || messages.Templates[0].Version != 1 ||
		messages.Templates[0].Body != "Synthetic unchanged body" {
		t.Fatal("upgrade altered existing communication history")
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
