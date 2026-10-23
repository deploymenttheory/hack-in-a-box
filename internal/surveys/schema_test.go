package surveys

import (
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestSurvey40EmptyRollbackAndOrderedReupgrade(t *testing.T) {
	pool := testdb.New(t, false)
	ctx := t.Context()
	if err := database.Migrate(ctx, pool, 39); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, pool, 40); err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	migrator, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		os.DirFS("../database/migrations"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = migrator.DownTo(ctx, 39); err != nil {
		t.Fatal(err)
	}
	if version, e := migrator.GetDBVersion(ctx); e != nil || version != 39 {
		t.Fatal(version, e)
	}
	if err = database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	if err = database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
}

func TestSurvey40PopulatedRollbackRetainsImmutableDefinition(t *testing.T) {
	f := setup(t)
	cfg := f.publish()
	ctx := t.Context()
	db := stdlib.OpenDBFromPool(f.pool)
	defer db.Close()
	migrator, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		os.DirFS("../database/migrations"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = migrator.DownTo(
		ctx,
		39,
	); err == nil ||
		!strings.Contains(err.Error(), "Survey definitions") {
		t.Fatal("populated rollback discarded survey", err)
	}
	if version, e := migrator.GetDBVersion(ctx); e != nil || version != 40 {
		t.Fatal("protected40 not retained", version, e)
	}
	retained, err := f.s.Configuration(ctx, f.owner.ID, f.event, f.id)
	if err != nil || retained.Version != cfg.Version {
		t.Fatal("retained definition changed", err)
	}
	if err = database.Migrate(ctx, f.pool, 0); err != nil {
		t.Fatal(err)
	}
	if err = database.CheckSchema(ctx, f.pool); err != nil {
		t.Fatal(err)
	}
}
