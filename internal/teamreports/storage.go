package teamreports

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
)

func failure(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("team report %s: %w", operation, err)
}

func fingerprint(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func latest(
	ctx context.Context,
	tx pgx.Tx,
	eventID, teamID uuid.UUID,
	released bool,
) (*Record, error) {
	var raw []byte
	err := tx.QueryRow(ctx, "SELECT document FROM team_feedback_versions WHERE event_id=$1 AND team_id=$2 AND (NOT $3::boolean OR state='released') ORDER BY version DESC LIMIT 1", eventID, teamID, released).
		Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // No version is not a published report.
	}
	if err != nil {
		return nil, failure("read latest", err)
	}
	var out Record
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil, failure("decode version", err)
	}
	return &out, nil
}

func original(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID, requestID uuid.UUID,
) (*Receipt, error) {
	var owner, target uuid.UUID
	var raw []byte
	err := tx.QueryRow(ctx, "SELECT actor_id,team_id,receipt FROM team_feedback_commands WHERE event_id=$1 AND request_id=$2", eventID, requestID).
		Scan(&owner, &target, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // Absence never acknowledges an original command.
	}
	if err != nil {
		return nil, failure("read original", err)
	}
	if owner != actor || target != teamID {
		return nil, access.ErrForbidden
	}
	var out Receipt
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil, failure("decode receipt", err)
	}
	return &out, nil
}

func retain(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	value Record,
	receipt Receipt,
) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return failure("encode version", err)
	}
	_, err = tx.Exec(
		ctx,
		"INSERT INTO team_feedback_versions(event_id,team_id,version,state,author_id,actor_id,judge_run_id,confirmation_id,document,changed_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)",
		eventID,
		value.TeamID,
		value.Version,
		value.State,
		value.AuthorID,
		value.ActorID,
		value.Source.JudgeRunID,
		value.Source.ConfirmationID,
		raw,
		value.ChangedAt,
	)
	if err != nil {
		return failure("retain version", err)
	}
	raw, err = json.Marshal(receipt)
	if err != nil {
		return failure("encode receipt", err)
	}
	_, err = tx.Exec(
		ctx,
		"INSERT INTO team_feedback_commands(event_id,team_id,request_id,actor_id,version,receipt)VALUES($1,$2,$3,$4,$5,$6)",
		eventID,
		value.TeamID,
		receipt.RequestID,
		receipt.ActorID,
		value.Version,
		raw,
	)
	return failure("retain original receipt", err)
}
