package githubwebhooks

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestOrderedWebhookUpgradePreservesHistoricalNativeRecords(t *testing.T) {
	pool := testdb.New(t, false)
	ctx := t.Context()
	if err := database.Migrate(ctx, pool, 43); err != nil {
		t.Fatal(err)
	}
	owner := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	teamID := uuid.New()
	operationID := uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	ciphertext := []byte("SYNTHETIC-PRE-EXISTING-PROTECTED-CREDENTIAL")
	exec(
		"INSERT INTO github_app_integrations(event_id,app_id,app_slug,organization_id,organization,template_repository,credential_version,key_id,private_key_encrypted,installation_id) VALUES($1,42,'synthetic-app',99,'synthetic-org','synthetic-template',3,'old-key',$2,7)",
		eventID,
		ciphertext,
	)
	exec(
		"INSERT INTO hack_teams(event_id,id,name,state) VALUES($1,$2,'Historical team','disbanded')",
		eventID,
		teamID,
	)
	exec(
		"INSERT INTO competition_entries(event_id,team_id,active) VALUES($1,$2,false)",
		eventID,
		teamID,
	)
	exec(
		"INSERT INTO submission_states(event_id,team_id,outcome) VALUES($1,$2,'withdrawn')",
		eventID,
		teamID,
	)
	exec(
		"INSERT INTO operations(id,event_id,operation_key,kind,target_id,due_at,approved,state) VALUES($1,$2,'historical-operation','historical-kind',$3,now(),true,'delivered')",
		operationID,
		eventID,
		teamID,
	)
	exec("UPDATE events SET writes_allowed=false WHERE id=$1", eventID)
	if err := database.Migrate(ctx, pool, 44); err != nil {
		t.Fatal(err)
	}
	exec(
		"INSERT INTO team_repositories(event_id,team_id,organization_id,organization,name,app_id,installation_id,template_repository,github_repository_id,state,requested_by,attempted,template_repository_id) VALUES($1,$2,99,'synthetic-org','historical-repo',42,7,'synthetic-template',123,'ready',$3,true,700)",
		eventID,
		teamID,
		owner.ID,
	)
	beforeRepository, err := dbgen.New(pool).
		TeamRepository(ctx, dbgen.TeamRepositoryParams{EventID: eventID, TeamID: teamID})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, pool, 45); err != nil {
		t.Fatal(err)
	}
	before46, err := dbgen.New(pool).
		TeamRepository(ctx, dbgen.TeamRepositoryParams{EventID: eventID, TeamID: teamID})
	if err != nil || !reflect.DeepEqual(beforeRepository, before46) {
		t.Fatalf("released44→actual45 changed retained repository provenance: %v", err)
	}
	if err := database.Migrate(ctx, pool, 46); err != nil {
		t.Fatal(err)
	}
	afterRepository, err := dbgen.New(pool).
		TeamRepository(ctx, dbgen.TeamRepositoryParams{EventID: eventID, TeamID: teamID})
	if err != nil || !reflect.DeepEqual(before46, afterRepository) {
		t.Fatalf(
			"actual45→webhook46 changed retained repository provenance: %v",
			err,
		)
	}
	if err := database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
	q := dbgen.New(pool)
	app, err := q.GitHubAppIntegration(ctx, eventID)
	if err != nil || app.CredentialVersion != 3 || app.InstallationID != 7 ||
		!bytes.Equal(app.PrivateKeyEncrypted, ciphertext) {
		t.Fatalf("App identity/credential lost: %+v %v", app, err)
	}
	event, err := q.Event(ctx, eventID)
	if err != nil || event.WritesAllowed {
		t.Fatal("historical writes policy changed")
	}
	operation, err := q.Operation(ctx, operationID)
	if err != nil || operation.State != "delivered" || operation.Kind != "historical-kind" {
		t.Fatal("historical durable work changed")
	}
	var repository int64
	var active bool
	var outcome string
	if err := pool.QueryRow(ctx, "SELECT r.github_repository_id,e.active,s.outcome FROM team_repositories r JOIN competition_entries e USING(event_id,team_id) JOIN submission_states s USING(event_id,team_id) WHERE r.event_id=$1 AND r.team_id=$2", eventID, teamID).
		Scan(&repository, &active, &outcome); err != nil {
		t.Fatal(err)
	}
	if repository != 123 || active || outcome != "withdrawn" {
		t.Fatal("retained repository/entry/submission evidence changed")
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM github_webhook_secrets").
		Scan(&count); err != nil ||
		count != 0 {
		t.Fatal("migration fabricated webhook credentials")
	}
}
