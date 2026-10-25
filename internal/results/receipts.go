package results

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
	actor, eventID, requestID uuid.UUID,
) (*Receipt, error) {
	var owner uuid.UUID
	var raw []byte
	err := tx.QueryRow(ctx, "SELECT actor_id,receipt FROM result_commands WHERE event_id=$1 AND request_id=$2", eventID, requestID).
		Scan(&owner, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // Absence is not acknowledgement; current reads never settle an original.
	}
	if err != nil {
		return nil, failure("read original receipt", err)
	}
	if owner != actor {
		return nil, access.ErrForbidden
	}
	var out Receipt
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil, failure("decode original receipt", err)
	}
	return &out, nil
}

func retainReceipt(ctx context.Context, tx pgx.Tx, eventID uuid.UUID, receipt Receipt) error {
	raw, err := json.Marshal(receipt)
	if err != nil {
		return failure("encode original receipt", err)
	}
	_, err = tx.Exec(
		ctx,
		"INSERT INTO result_commands(event_id,request_id,actor_id,result_version,receipt) VALUES($1,$2,$3,$4,$5)",
		eventID,
		receipt.RequestID,
		receipt.ActorID,
		receipt.Version,
		raw,
	)
	return failure("retain original receipt", err)
}

// OriginalReceipt returns immutable metadata for the exact actor/event/request.
// It neither reads a private result payload nor treats current state as its ACK.
func (s Service) OriginalReceipt(
	ctx context.Context,
	actor, eventID, requestID uuid.UUID,
) (Recovery, error) {
	var out Recovery
	if requestID == uuid.Nil {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		current, err := s.guard(ctx, tx, actor, eventID, false)
		if err != nil {
			return err
		}
		if !current.organiser {
			return access.ErrForbidden
		}
		out.Receipt, err = originalReceipt(ctx, tx, actor, eventID, requestID)
		return err
	})
	return out, failure("recover original metadata", err)
}
