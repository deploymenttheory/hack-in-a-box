package results

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func emptyRound() Round {
	return Round{Source: emptyPreview(), Votes: []Vote{}}
}

func load(ctx context.Context, tx pgx.Tx, eventID uuid.UUID) (Round, error) {
	out := emptyRound()
	var raw []byte
	err := tx.QueryRow(ctx, "SELECT document FROM result_versions WHERE event_id=$1 ORDER BY version DESC LIMIT 1", eventID).
		Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, failure("read decision", err)
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return out, failure("decode decision", err)
	}
	return out, nil
}
