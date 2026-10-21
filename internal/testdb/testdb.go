// Package testdb starts isolated real PostgreSQL fixtures for integration tests.
package testdb

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
)

const Image = "postgres:17.7-bookworm@sha256:86e0b703649d7a792bd9243ee28afc9d8f7c6b2b5638077c9d6882d4d472bbfd"

func New(t *testing.T, migrate bool) *pgxpool.Pool {
	t.Helper()
	if os.Getenv("HACKBOX_INTEGRATION") != "1" {
		t.Skip("set HACKBOX_INTEGRATION=1 to run real PostgreSQL integration tests")
	}
	ctx := t.Context()
	container, err := postgres.Run(
		ctx,
		Image,
		postgres.WithDatabase("hackbox_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("synthetic-test-only"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.WithoutCancel(ctx)); err != nil {
			t.Error(err)
		}
	})
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if migrate {
		if err := database.Migrate(ctx, pool, 0); err != nil {
			t.Fatal(err)
		}
	}
	return pool
}
