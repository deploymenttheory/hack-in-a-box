package githubwebhooks

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func webhookMigrator(t *testing.T, pool *pgxpool.Pool) *goose.Provider {
	t.Helper()
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })
	migrator, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		os.DirFS("../database/migrations"),
	)
	if err != nil {
		t.Fatal(err)
	}
	return migrator
}

func rollbackSnapshot(t *testing.T, pool *pgxpool.Pool, tables []string) map[string]string {
	t.Helper()
	snapshot := make(map[string]string, len(tables))
	for _, table := range tables {
		var rows string
		query := fmt.Sprintf(
			`SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text),'[]')::text FROM %s t`,
			table,
		)
		if err := pool.QueryRow(t.Context(), query).Scan(&rows); err != nil {
			t.Fatal("snapshot owning table", table, err)
		}
		snapshot[table] = rows
	}
	return snapshot
}

func TestWebhook46EmptyRollbackPreservesActual45History(t *testing.T) {
	pool := testdb.New(t, false)
	ctx := t.Context()
	if err := database.Migrate(ctx, pool, 45); err != nil {
		t.Fatal(err)
	}
	owner := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	teamID := uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(
		`INSERT INTO hack_teams(event_id,id,name,state) VALUES($1,$2,'Retained team','disbanded')`,
		eventID,
		teamID,
	)
	exec(
		`INSERT INTO team_repositories(event_id,team_id,organization_id,organization,name,app_id,installation_id,template_repository,requested_by,attempted,template_repository_id,state) VALUES($1,$2,99,'synthetic-org','retained-target',42,7,'synthetic-template',$3,true,700,'ambiguous')`,
		eventID,
		teamID,
		owner.ID,
	)
	exec(
		`INSERT INTO github_app_integrations(event_id,app_id,app_slug,organization_id,organization,template_repository,credential_version,key_id,private_key_encrypted,installation_id) VALUES($1,42,'synthetic-app',99,'synthetic-org','synthetic-template',3,'retained-key',decode('aabb','hex'),7)`,
		eventID,
	)
	exec(
		`INSERT INTO operations(event_id,operation_key,kind,target_id,due_at,approved,state) VALUES($1,'retained-provision','repository.provision',$2,now(),true,'ambiguous')`,
		eventID,
		teamID,
	)
	exec(`UPDATE events SET writes_allowed=false WHERE id=$1`, eventID)
	tables := []string{
		"events",
		"people",
		"github_app_integrations",
		"team_repositories",
		"operations",
	}
	before := rollbackSnapshot(t, pool, tables)
	if err := database.Migrate(ctx, pool, 46); err != nil {
		t.Fatal(err)
	}
	migrator := webhookMigrator(t, pool)
	if _, err := migrator.DownTo(ctx, 45); err != nil {
		t.Fatal("empty46 rollback rejected retained45 provenance", err)
	}
	if version, err := migrator.GetDBVersion(ctx); err != nil || version != 45 {
		t.Fatal("empty46 rollback lost actual45 version", version, err)
	}
	if !reflect.DeepEqual(before, rollbackSnapshot(t, pool, tables)) {
		t.Fatal("empty46 rollback changed actual45 history")
	}
	if _, err := pool.Exec(
		ctx,
		`DELETE FROM team_repositories WHERE event_id=$1`,
		eventID,
	); err == nil {
		t.Fatal("empty46 rollback disabled actual45 retention barrier")
	}
	if err := database.Migrate(ctx, pool, 46); err != nil {
		t.Fatal(err)
	}
	if err := database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
}

func TestWebhook46PopulatedRollbackRetainsDedupSecretsAndWork(t *testing.T) {
	for _, kind := range []string{"configured-secret", "pending-delivery", "recorded-delivery", "activity-only", "operation-history-only"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			ctx := t.Context()
			switch kind {
			case "pending-delivery", "recorded-delivery":
				id := uuid.New()
				if err := f.ingest(id, f.push(time.Now().Add(-time.Minute))); err != nil {
					t.Fatal(err)
				}
				if kind == "recorded-delivery" {
					f.execute(id)
				}
			case "activity-only":
				f.exec(
					`INSERT INTO github_repository_activity(event_id,team_id,pushes,last_received_at) VALUES($1,$2,1,now())`,
					f.event,
					f.team,
				)
			case "operation-history-only":
				if err := database.Within(ctx, f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
					_, err := f.s.Jobs.Enqueue(
						ctx,
						tx,
						dbgen.InsertOperationParams{
							EventID:      f.event,
							TargetID:     uuid.New(),
							OperationKey: "retained-webhook-operation",
							Kind:         HandlerKind,
							DueAt:        time.Now(),
							Approved:     true,
						},
					)
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
			// Isolate each guard reason: history cases must remain protected even after
			// the native App disconnect removes its configured webhook secret.
			if kind != "configured-secret" {
				event, err := dbgen.New(f.pool).Event(ctx, f.event)
				if err != nil {
					t.Fatal(err)
				}
				if err = f.app.Configure(
					ctx,
					f.owner,
					f.event,
					event.Revision,
					githubapp.Command{Action: "disconnect"},
				); err != nil {
					t.Fatal(err)
				}
			}
			tables := []string{
				"github_webhook_secrets",
				"github_webhook_deliveries",
				"github_repository_activity",
				"github_app_integrations",
				"team_repositories",
				"operations",
				"delivery_attempts",
				"river_job",
				"audit_events",
			}
			before := rollbackSnapshot(t, f.pool, tables)
			migrator := webhookMigrator(t, f.pool)
			if _, err := migrator.DownTo(
				ctx,
				45,
			); err == nil ||
				!strings.Contains(err.Error(), "Webhook configuration or activity history exists") {
				t.Fatalf(
					"populated46 rollback erased %s evidence instead of rejecting: %v",
					kind,
					err,
				)
			}
			if version, err := migrator.GetDBVersion(ctx); err != nil || version != 46 {
				t.Fatal("blocked rollback changed schema version", version, err)
			}
			if !reflect.DeepEqual(before, rollbackSnapshot(t, f.pool, tables)) {
				t.Fatal(
					"blocked rollback changed GUID/hash/provenance, secrets, activity, durable work or audit",
				)
			}
			if err := database.CheckSchema(ctx, f.pool); err != nil {
				t.Fatal(err)
			}
		})
	}
}
