// Package recovery owns operator-only startup isolation and verified recovery.
// Deployment database access is required; it grants no application role.
package recovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const runtimeLock int64 = 7261938402

var (
	ErrBusy = errors.New(
		"application services are still running; quiesce them before recovery capture",
	)
	ErrReview = errors.New("recovery hold or pending operation review changed")
)

// Status contains safe operational metadata only, never external payloads or keys.
type Status struct {
	HoldID      *uuid.UUID `json:"hold_id"`
	HeldAt      *time.Time `json:"held_at"`
	Fingerprint string     `json:"fingerprint"`
	Unresolved  int64      `json:"unresolved"`
}

// Exclusive prevents native API/worker startup during a quiesced backup or hold.
// The caller must retain and close the connection for the complete operation.
func Exclusive(ctx context.Context, pool *pgxpool.Pool) (*pgx.Conn, error) {
	conn, err := pgx.ConnectConfig(ctx, pool.Config().ConnConfig.Copy())
	if err != nil {
		return nil, fmt.Errorf("connect recovery lease: %w", err)
	}
	var acquired bool
	if err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", runtimeLock).
		Scan(&acquired); err != nil ||
		!acquired {
		_ = conn.Close(context.WithoutCancel(ctx))
		if err != nil {
			return nil, fmt.Errorf("acquire recovery lease: %w", err)
		}
		return nil, ErrBusy
	}
	return conn, nil
}

// Runtime waits before any file cleanup, provider construction or maintenance.
// Its dedicated shared lease prevents a backup from racing a live process.
func Runtime(ctx context.Context, pool *pgxpool.Pool) (*pgx.Conn, error) {
	conn, err := pgx.ConnectConfig(ctx, pool.Config().ConnConfig.Copy())
	if err != nil {
		return nil, fmt.Errorf("connect runtime lease: %w", err)
	}
	for {
		var acquired bool
		err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock_shared($1)", runtimeLock).
			Scan(&acquired)
		if err != nil {
			break
		}
		if acquired {
			var held bool
			err = conn.QueryRow(ctx, "SELECT hold_id IS NOT NULL FROM recovery_control WHERE singleton").
				Scan(&held)
			if err != nil {
				break
			}
			if !held {
				return conn, nil
			}
			_, err = conn.Exec(ctx, "SELECT pg_advisory_unlock_shared($1)", runtimeLock)
			if err != nil {
				break
			}
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			err = ctx.Err()
		case <-timer.C:
		}
		if err != nil {
			break
		}
	}
	_ = conn.Close(context.WithoutCancel(ctx))
	return nil, fmt.Errorf("wait for reviewed recovery: %w", err)
}

// Hold requires quiesced application services and preserves all original intents.
func Hold(ctx context.Context, pool *pgxpool.Pool) (Status, error) {
	lease, err := Exclusive(ctx, pool)
	if err != nil {
		return Status{}, err
	}
	defer func() { _ = lease.Close(context.WithoutCancel(ctx)) }()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("begin recovery hold: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	id := uuid.New()
	var existing *uuid.UUID
	if err = tx.QueryRow(ctx, "SELECT hold_id FROM recovery_control WHERE singleton FOR UPDATE").
		Scan(&existing); err != nil {
		return Status{}, fmt.Errorf("lock recovery hold: %w", err)
	}
	if existing == nil {
		if _, err = tx.Exec(
			ctx,
			"UPDATE recovery_control SET hold_id=$1,held_at=clock_timestamp() WHERE singleton",
			id,
		); err != nil {
			return Status{}, fmt.Errorf("install recovery hold: %w", err)
		}
		if _, err = tx.Exec(
			ctx,
			"INSERT INTO recovery_history(hold_id,action) VALUES($1,'hold')",
			id,
		); err != nil {
			return Status{}, fmt.Errorf("record recovery hold: %w", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Status{}, fmt.Errorf("commit recovery hold: %w", err)
	}
	return Read(ctx, pool)
}

// Read fingerprints the current unresolved operation metadata, not their payloads.
func Read(ctx context.Context, pool *pgxpool.Pool) (Status, error) {
	tx, err := pool.BeginTx(
		ctx,
		pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly},
	)
	if err != nil {
		return Status{}, fmt.Errorf("begin recovery review: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	s, err := read(ctx, tx)
	if err != nil {
		return Status{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Status{}, fmt.Errorf("commit recovery review: %w", err)
	}
	return s, nil
}

func read(ctx context.Context, tx pgx.Tx) (Status, error) {
	var s Status
	if err := tx.QueryRow(ctx, "SELECT hold_id,held_at FROM recovery_control WHERE singleton").
		Scan(&s.HoldID, &s.HeldAt); err != nil {
		return s, fmt.Errorf("read recovery hold: %w", err)
	}
	var metadata []byte
	if err := tx.QueryRow(ctx, `SELECT count(*), COALESCE(jsonb_agg(jsonb_build_array(id,event_id,kind,target_id,revision,state,approved,cancelled,due_at,attempts,lease_token,lease_until) ORDER BY id),'[]'::jsonb)::text FROM operations WHERE state IN ('pending','running','ambiguous')`).
		Scan(&s.Unresolved, &metadata); err != nil {
		return s, fmt.Errorf("read recovery operation metadata: %w", err)
	}
	digest := sha256.Sum256(metadata)
	s.Fingerprint = hex.EncodeToString(digest[:])
	return s, nil
}

// Resume records the exact current review without approving, replaying or
// resolving any operation. Native approval and reconciliation remain unchanged.
func Resume(ctx context.Context, pool *pgxpool.Pool, holdID uuid.UUID, fingerprint string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin reviewed resume: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err = tx.Exec(
		ctx,
		"SELECT singleton FROM recovery_control WHERE singleton FOR UPDATE",
	); err != nil {
		return fmt.Errorf("lock reviewed resume: %w", err)
	}
	if _, err = tx.Exec(ctx, "LOCK TABLE operations IN SHARE MODE"); err != nil {
		return fmt.Errorf("lock recovery review: %w", err)
	}
	s, err := read(ctx, tx)
	if err != nil {
		return err
	}
	if s.HoldID == nil || *s.HoldID != holdID || s.Fingerprint != fingerprint {
		return ErrReview
	}
	if _, err = tx.Exec(
		ctx,
		"INSERT INTO recovery_history(hold_id,action,reviewed_fingerprint) VALUES($1,'resume',$2)",
		holdID,
		fingerprint,
	); err != nil {
		return fmt.Errorf("record reviewed resume: %w", err)
	}
	if _, err = tx.Exec(
		ctx,
		"UPDATE recovery_control SET hold_id=NULL,held_at=NULL WHERE singleton",
	); err != nil {
		return fmt.Errorf("clear reviewed recovery hold: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit reviewed resume: %w", err)
	}
	return nil
}
