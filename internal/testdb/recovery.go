package testdb

import (
	"context"
	"fmt"
	"io"
	"os/exec"
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

// NewRecoveryTarget starts a genuinely separate, empty PostgreSQL server. It never
// reuses or stops any existing deployment container.
func NewRecoveryTarget(t *testing.T) *Recovery {
	t.Helper()
	pool, container := newInstance(t, false)
	return &Recovery{Pool: pool, container: container}
}

// DumpStream pipes the own fixture's pg_dump stdout directly to encryption.
func (r *Recovery) DumpStream(ctx context.Context, writer io.Writer) error {
	// #nosec G204 -- Container identity comes only from this owned Testcontainers fixture; no shell is invoked.
	command := exec.CommandContext(
		ctx,
		"docker",
		"exec",
		r.container.GetContainerID(),
		"pg_dump",
		"--username=test",
		"--dbname=hackbox_test",
		"--no-owner",
		"--no-privileges",
		"--format=plain",
	)
	command.Stdout = writer
	if err := command.Run(); err != nil {
		return fmt.Errorf("stream synthetic database: %w", err)
	}
	return nil
}

// RestoreStream pipes authenticated SQL to a single transaction on the own fresh
// fixture. No plaintext dump or deployment key is written inside the container.
func (r *Recovery) RestoreStream(ctx context.Context, reader io.Reader) error {
	// #nosec G204 -- Container identity comes only from this owned Testcontainers fixture; no shell is invoked.
	command := exec.CommandContext(
		ctx,
		"docker",
		"exec",
		"-i",
		r.container.GetContainerID(),
		"psql",
		"--username=test",
		"--dbname=hackbox_test",
		"--no-psqlrc",
		"--set=ON_ERROR_STOP=1",
		"--single-transaction",
	)
	command.Stdin = reader
	command.Stdout = io.Discard
	if err := command.Run(); err != nil {
		return fmt.Errorf("restore synthetic database: %w", err)
	}
	return nil
}
