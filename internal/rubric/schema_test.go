package rubric_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

const beforeRubricSchema, versionedRubricSchema = 28, 29

func TestRubricUpgradePreservesPriorChallengeDefinitionsCiphertextAndAudit(t *testing.T) {
	pool := testdb.New(t, false)
	ctx := t.Context()
	if err := database.Migrate(ctx, pool, beforeRubricSchema); err != nil {
		t.Fatal(err)
	}
	owner, member := testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	q := dbgen.New(pool)
	execute := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	execute(
		"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
		eventID,
		member.ID,
	)
	execute("UPDATE events SET stage='preparation',published_at=$2 WHERE id=$1", eventID, now)
	challengeID := uuid.New()
	// Use only pre-rubric columns and JSON fields. The new approval service must
	// not be used to manufacture a historical publication on an older schema.
	document, _ := json.Marshal(
		map[string]any{
			"title":    "Retained common challenge",
			"markdown": "Synthetic retained task",
			"audience": map[string]any{
				"confirmed": true,
				"roles":     []string{},
				"teams":     []uuid.UUID{},
			},
			"attachments":             []uuid.UUID{},
			"scheduled_at":            nil,
			"require_acknowledgement": false,
		},
	)
	execute(
		"INSERT INTO content_items(event_id,id,kind,revision,version,state,approved_by,approved_revision) VALUES($1,$2,'challenge',3,1,'released',$3,2)",
		eventID,
		challengeID,
		owner.ID,
	)
	execute(
		"INSERT INTO content_versions(event_id,content_id,version,document,actor_id,created_at) VALUES($1,$2,1,$3,$4,$5)",
		eventID,
		challengeID,
		document,
		owner.ID,
		now,
	)
	execute(
		"UPDATE content_items SET released_version=1,released_at=$3 WHERE event_id=$1 AND id=$2",
		eventID,
		challengeID,
		now,
	)
	execute(
		"INSERT INTO content_changes(event_id,content_id,revision,version,action,actor_id,reason,occurred_at) VALUES($1,$2,3,1,'released',$3,'Synthetic retained release',$4)",
		eventID,
		challengeID,
		owner.ID,
		now,
	)
	definitions, _ := json.Marshal(
		map[string]any{
			"rubrics": []any{
				map[string]any{
					"id":   uuid.New(),
					"name": "Retained configured draft",
					"criteria": []any{
						map[string]any{
							"id":        uuid.New(),
							"label":     "Evidence",
							"guidance":  "Retained public guidance",
							"max_score": 10,
							"weight":    1,
						},
					},
				},
			},
		},
	)
	execute(
		"INSERT INTO event_definitions(event_id,definitions) VALUES($1,$2)",
		eventID,
		definitions,
	)
	if err := audit.Append(
		ctx,
		q,
		audit.Record{
			ActorID:  &owner.ID,
			EventID:  &eventID,
			TargetID: eventID,
			Action:   audit.EventCreated,
			Reason:   audit.OrganiserDecision,
		},
	); err != nil {
		t.Fatal(err)
	}
	keyConfig, _ := json.Marshal(
		map[string]any{
			"active": "synthetic-upgrade",
			"keys": map[string]string{
				"synthetic-upgrade": base64.StdEncoding.EncodeToString(
					bytes.Repeat([]byte{37}, 32),
				),
			},
		},
	)
	keys, err := secretbox.Parse(string(keyConfig))
	if err != nil {
		t.Fatal(err)
	}
	credentialID := uuid.New()
	binding := []byte(fmt.Sprintf("hackbox:team-credential:v1:%s:%s:1", eventID, credentialID))
	const supplied = "Synthetic migration-only credential"
	keyID, ciphertext, err := keys.Seal([]byte(supplied), binding)
	if err != nil {
		t.Fatal(err)
	}
	execute(
		"INSERT INTO team_credentials(id,event_id,label,key_id,ciphertext) VALUES($1,$2,'Retained synthetic resource',$3,$4)",
		credentialID,
		eventID,
		keyID,
		ciphertext,
	)
	var oldDocument, oldDefinitions, oldAudit []byte
	for _, read := range []struct {
		query  string
		target *[]byte
	}{{"SELECT document FROM content_versions WHERE event_id=$1", &oldDocument}, {"SELECT definitions FROM event_definitions WHERE event_id=$1", &oldDefinitions}, {"SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM audit_events a WHERE event_id=$1", &oldAudit}} {
		if err = pool.QueryRow(ctx, read.query, eventID).Scan(read.target); err != nil {
			t.Fatal(err)
		}
	}
	if err = database.Migrate(ctx, pool, versionedRubricSchema); err != nil {
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
	if _, err = provider.DownTo(ctx, beforeRubricSchema); err != nil {
		t.Fatal("empty rubric schema did not support normal rollback", err)
	}
	if err = database.Migrate(ctx, pool, versionedRubricSchema); err != nil {
		t.Fatal(err)
	}

	for _, read := range []struct {
		query  string
		before []byte
	}{{"SELECT document FROM content_versions WHERE event_id=$1", oldDocument}, {"SELECT definitions FROM event_definitions WHERE event_id=$1", oldDefinitions}, {"SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM audit_events a WHERE event_id=$1", oldAudit}} {
		var after []byte
		if err = pool.QueryRow(ctx, read.query, eventID).
			Scan(&after); err != nil ||
			!bytes.Equal(after, read.before) {
			t.Fatal("rubric upgrade changed retained native content, clone or audit state")
		}
	}
	retained, err := q.TeamCredential(
		ctx,
		dbgen.TeamCredentialParams{EventID: eventID, ID: credentialID},
	)
	if err != nil || retained.KeyID != keyID || !bytes.Equal(retained.Ciphertext, ciphertext) {
		t.Fatal("rubric upgrade changed private resource ciphertext")
	}
	opened, err := keys.Open(retained.KeyID, retained.Ciphertext, binding)
	if err != nil || string(opened) != supplied {
		t.Fatal("retained resource no longer authenticates with its deployment key")
	}
	service := rubric.Service{Pool: pool}
	configuration, err := service.Configuration(ctx, owner.ID, eventID, 0)
	if err != nil || configuration.Approved || configuration.Reference.ID != uuid.Nil ||
		configuration.Definition.Name != "Retained configured draft" {
		t.Fatal("upgrade implicitly approved or replaced legacy configured criteria")
	}
	if _, err = service.Public(ctx, member.ID, eventID); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("upgrade invented a rubric publication for an old challenge")
	}
	var versions int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM rubric_versions").
		Scan(&versions); err != nil ||
		versions != 0 {
		t.Fatal("upgrade silently created a scoring version")
	}
	if _, err = pool.Exec(
		ctx,
		"UPDATE content_versions SET document='{}' WHERE event_id=$1",
		eventID,
	); err == nil {
		t.Fatal("upgrade weakened retained content immutability")
	}
	saved, err := service.Change(
		ctx,
		owner.ID,
		eventID,
		0,
		rubric.Command{
			Action:     "save",
			Definition: &configuration.Definition,
			Reason:     "Synthetic explicit retained rubric",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(ctx, beforeRubricSchema); err == nil {
		t.Fatal("rollback discarded retained rubric history")
	}
	retainedConfiguration, err := service.Configuration(
		ctx,
		owner.ID,
		eventID,
		saved.Reference.Version,
	)
	if err != nil || retainedConfiguration.Reference != saved.Reference {
		t.Fatal("rejected rollback damaged retained rubric version")
	}
	if err = database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
}
