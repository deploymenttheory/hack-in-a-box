package qualification

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func originalReceipt(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, runID, requestID uuid.UUID,
) (*Receipt, error) {
	var owner, run uuid.UUID
	var raw []byte
	err := tx.QueryRow(ctx, "SELECT actor_id,run_id,receipt FROM mentor_qualification_commands WHERE event_id=$1 AND request_id=$2", eventID, requestID).
		Scan(&owner, &run, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // No original acknowledgement is known; a fresh read never settles it.
	}
	if err != nil {
		return nil, failure("read original request", err)
	}
	if actor != owner || runID != run {
		return nil, access.ErrForbidden
	}
	var out Receipt
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil, failure("decode original request", err)
	}
	return &out, nil
}

func retainReceipt(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	receipt Receipt,
) error {
	raw, err := json.Marshal(receipt)
	if err != nil {
		return failure("encode original request", err)
	}
	_, err = tx.Exec(
		ctx,
		"INSERT INTO mentor_qualification_commands(event_id,request_id,actor_id,run_id,version,receipt)VALUES($1,$2,$3,$4,$5,$6)",
		eventID,
		receipt.RequestID,
		actor,
		receipt.RunID,
		receipt.Version,
		raw,
	)
	return failure("retain original request", err)
}

// OriginalReceipt returns only immutable original request metadata under current
// native event/actor/panel authority. It contains no card, note, code or draft.
func (s Service) OriginalReceipt(
	ctx context.Context,
	actor, eventID, runID, requestID uuid.UUID,
) (Recovery, error) {
	out := Recovery{}
	if requestID == uuid.Nil {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		if _, err := s.guard(ctx, tx, actor, eventID, runID, historicalRead); err != nil {
			return err
		}
		var err error
		out.Receipt, err = originalReceipt(ctx, tx, actor, eventID, runID, requestID)
		return err
	})
	return out, failure("recover original request", err)
}
