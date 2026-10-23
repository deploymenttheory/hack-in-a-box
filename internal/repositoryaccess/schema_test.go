package repositoryaccess

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/repositories"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestRepositoryAccessMigrationPreservesNativeRegistryAndRetainsEffects(t *testing.T) {
	pool := testdb.New(t, false)
	ctx := t.Context()
	// Final ordinal is deliberately resolved against actual merged webhook 46.
	if err := database.Migrate(ctx, pool, 46); err != nil {
		t.Fatal(err)
	}
	f := setupAccess(t, pool)
	reader := repositories.Reader{Pool: pool}
	before, err := reader.ForTeam(ctx, f.event, f.team)
	if err != nil {
		t.Fatal(err)
	}
	if err = database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	after, err := reader.ForTeam(ctx, f.event, f.team)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("access upgrade altered native registry", err)
	}
	sqlDB := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = sqlDB.Close() })
	migrator, err := goose.NewProvider(
		goose.DialectPostgres,
		sqlDB,
		os.DirFS("../database/migrations"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = migrator.DownTo(ctx, 46); err != nil {
		t.Fatal("empty access rollback", err)
	}
	after, err = reader.ForTeam(ctx, f.event, f.team)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("empty rollback altered native registry", err)
	}
	if err = database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	f.publish()
	plan := f.review()
	f.approve(plan)
	f.provider.fault = "lost"
	f.execute(plan.Actions[0])
	for _, sql := range []string{"TRUNCATE repository_access_plans CASCADE", "TRUNCATE repository_access_effects CASCADE", "TRUNCATE repository_access_grants", "UPDATE repository_access_plans SET approved_at=NULL,approved_by=NULL", "UPDATE repository_access_plans SET fingerprint='changed'", "UPDATE repository_access_grants SET github_id=99999"} {
		if _, err = pool.Exec(ctx, sql); err == nil {
			t.Fatal("access history could be altered", sql)
		}
	}
	if _, err = migrator.DownTo(
		ctx,
		46,
	); err == nil ||
		!strings.Contains(err.Error(), "Repository access history exists") {
		t.Fatal("populated access rollback was not rejected", err)
	}
	if version, versionErr := migrator.GetDBVersion(
		ctx,
	); versionErr != nil ||
		version != database.SchemaVersion {
		t.Fatal("failed rollback changed schema", version, versionErr)
	}
	if !f.view().Grants[0].Uncertain || f.provider.writes != 1 {
		t.Fatal("failed rollback changed uncertain effect")
	}
}
