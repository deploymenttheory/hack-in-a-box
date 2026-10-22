package testdb

import (
	"io"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// Recovery owns an isolated PostgreSQL server for an actual logical backup and
// restore. Deployment encryption keys never enter the container or its dump.
type Recovery struct {
	Pool      *pgxpool.Pool
	container *postgres.PostgresContainer
}

func NewRecovery(t *testing.T) *Recovery {
	t.Helper()
	pool, container := newInstance(t, true)
	return &Recovery{Pool: pool, container: container}
}

// Restore dumps the source database and restores into a new, empty database.
// The returned dump is evidence for assertions, never included in test logs.
func (r *Recovery) Restore(t *testing.T) (*pgxpool.Pool, []byte) {
	t.Helper()
	ctx := t.Context()
	run := func(args ...string) {
		t.Helper()
		code, _, err := r.container.Exec(ctx, args)
		if err != nil || code != 0 {
			t.Fatalf("backup/restore command failed (exit %d): %v", code, err)
		}
	}
	run(
		"pg_dump",
		"--username=test",
		"--dbname=hackbox_test",
		"--no-owner",
		"--no-privileges",
		"--file=/tmp/synthetic-recovery.sql",
	)
	file, err := r.container.CopyFileFromContainer(ctx, "/tmp/synthetic-recovery.sql")
	if err != nil {
		t.Fatal(err)
	}
	dump, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		t.Fatal("cannot read recovery evidence")
	}
	run("createdb", "--username=test", "credential_recovered")
	run(
		"psql",
		"--username=test",
		"--dbname=credential_recovered",
		"--set=ON_ERROR_STOP=1",
		"--file=/tmp/synthetic-recovery.sql",
	)
	cfg := r.Pool.Config().Copy()
	cfg.ConnConfig.Database = "credential_recovered"
	restored, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restored.Close)
	if err = restored.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	return restored, dump
}
