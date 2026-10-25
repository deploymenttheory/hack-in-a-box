// Package database owns schema compatibility and transaction boundaries.
package database

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

//go:embed migrations/*.sql
var migrations embed.FS

const (
	SchemaVersion int64 = 52
	migrationLock int64 = 7261938401
)

var (
	ErrSchema = errors.New(
		"incompatible database schema; run the matching migrate command before starting services",
	)
	ErrDatabase = errors.New("database unavailable; check connectivity and credentials")
)

func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("HACKBOX_DATABASE_URL is required: %w", ErrDatabase)
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, ErrDatabase
	}
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	cfg.MaxConns = 12
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, ErrDatabase
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, ErrDatabase
	}
	return pool, nil
}

// Migrate serializes the complete application/River upgrade using a dedicated
// session lock. Closing the connection releases the lock even on cancellation.
// Services never migrate on startup. A failed migration prevents deployment.
func Migrate(ctx context.Context, pool *pgxpool.Pool, target int64) error {
	conn, err := pgx.ConnectConfig(ctx, pool.Config().ConnConfig.Copy())
	if err != nil {
		return ErrDatabase
	}
	defer func() { _ = conn.Close(context.WithoutCancel(ctx)) }()
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLock); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()
	source, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("migration sources: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, source)
	if err != nil {
		return fmt.Errorf("migration provider: %w", err)
	}
	if target == 0 {
		target = SchemaVersion
	}
	if _, err := provider.UpTo(ctx, target); err != nil {
		return fmt.Errorf("apply application migrations: %w", err)
	}
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("river migrator: %w", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("apply River migrations: %w", err)
	}
	return nil
}

func CheckSchema(ctx context.Context, pool *pgxpool.Pool) error {
	var version int64
	if err := pool.QueryRow(ctx, "SELECT COALESCE(max(version_id),0) FROM goose_db_version WHERE is_applied").
		Scan(&version); err != nil ||
		version != SchemaVersion {
		return ErrSchema
	}
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return ErrSchema
	}
	existing, err := migrator.ExistingVersions(ctx)
	if err != nil || len(existing) != len(migrator.AllVersions()) {
		return ErrSchema
	}
	for i, migration := range existing {
		if migration.Version != migrator.AllVersions()[i].Version {
			return ErrSchema
		}
	}
	return nil
}

// Within shares one pgx transaction with sqlc repositories and River InsertTx.
// A callback must not commit; any error rolls back all its database effects.
func Within(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx, *dbgen.Queries) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if err := fn(tx, dbgen.New(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
