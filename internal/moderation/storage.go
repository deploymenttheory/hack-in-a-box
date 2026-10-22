package moderation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func load(ctx context.Context, tx pgx.Tx, eventID, teamID, id uuid.UUID) (Huddle, error) {
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT document FROM moderation_huddle_versions WHERE event_id=$1 AND team_id=$2 AND id=$3 ORDER BY version DESC LIMIT 1`, eventID, teamID, id).
		Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return Huddle{}, access.ErrNotFound
	}
	if err != nil {
		return Huddle{}, fmt.Errorf("read huddle: %w", err)
	}
	var h Huddle
	if err = json.Unmarshal(raw, &h); err != nil {
		return Huddle{}, fmt.Errorf("decode huddle: %w", err)
	}
	if h.Definition.ID != id || h.TeamID != teamID || h.Version < 1 {
		return Huddle{}, access.ErrConflict
	}
	return h, nil
}

func (s Service) retain(
	ctx context.Context,
	tx pgx.Tx,
	eventID, actor uuid.UUID,
	previous int64,
	h *Huddle,
	action, reason string,
) error {
	if strings.TrimSpace(reason) == "" || len(reason) > 2000 {
		return access.ErrInvalid
	}
	h.Version++
	h.ActorID = actor
	h.Action = action
	h.Reason = reason
	h.ChangedAt = s.now()
	raw, err := json.Marshal(h)
	if err != nil {
		return fmt.Errorf("encode huddle decision: %w", err)
	}
	_, err = tx.Exec(
		ctx,
		`INSERT INTO moderation_huddle_versions(event_id,team_id,id,version,run_id,document,actor_id,reason,changed_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		eventID,
		h.TeamID,
		h.Definition.ID,
		h.Version,
		h.Checkpoint.Definition.RunID,
		raw,
		actor,
		reason,
		h.ChangedAt,
	)
	if err != nil {
		return fmt.Errorf("retain huddle decision: %w", err)
	}
	for _, card := range h.Cards {
		if _, err = tx.Exec(
			ctx,
			`INSERT INTO moderation_huddle_cards(event_id,huddle_id,huddle_version,card_id,card_version)VALUES($1,$2,$3,$4,$5)`,
			eventID,
			h.Definition.ID,
			h.Version,
			card.ID,
			card.Version,
		); err != nil {
			return fmt.Errorf("retain exact huddle card: %w", err)
		}
	}
	q := dbgen.New(tx)
	row, err := q.BumpEventRevision(ctx, eventID)
	if err != nil {
		return fmt.Errorf("advance moderation revision: %w", err)
	}
	err = audit.Append(
		ctx,
		q,
		audit.Record{
			ActorID:  &actor,
			EventID:  &eventID,
			TargetID: h.Definition.ID,
			Action:   audit.ModerationChanged,
			Reason:   audit.UserRequest,
			Metadata: audit.Metadata{PreviousRevision: &previous, Revision: &row.Revision},
		},
	)
	if err != nil {
		return fmt.Errorf("audit moderation metadata: %w", err)
	}
	return nil
}
