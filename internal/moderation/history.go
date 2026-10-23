package moderation

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
)

func historyBatch(
	ctx context.Context,
	tx pgx.Tx,
	eventID, teamID, id uuid.UUID,
	before *int64,
) ([]Huddle, error) {
	rows, err := tx.Query(
		ctx,
		`SELECT document FROM moderation_huddle_versions WHERE event_id=$1 AND team_id=$2 AND id=$3 AND ($4::bigint IS NULL OR version<$4) ORDER BY version DESC LIMIT 100`,
		eventID,
		teamID,
		id,
		before,
	)
	if err != nil {
		return nil, fmt.Errorf("page huddle snapshot history: %w", err)
	}
	defer rows.Close()
	out := []Huddle{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("read historical huddle snapshot: %w", err)
		}
		var h Huddle
		if err = json.Unmarshal(raw, &h); err != nil {
			return nil, fmt.Errorf("decode historical huddle snapshot: %w", err)
		}
		out = append(out, h)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("page historical huddle snapshots: %w", err)
	}
	return out, nil
}

func (s Service) authorizeHistoricalSnapshot(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	h Huddle,
) error {
	if !slices.Contains(h.PanelIDs, actor) {
		return access.ErrForbidden
	}
	// The latest snapshot already establishes current event/team/panel authority.
	// Historical membership additionally limits the original private audience.
	if h.Review.Card != nil {
		if err := s.Cards.AuthorizeSubmittedHistoryInTransaction(
			ctx,
			tx,
			eventID,
			h.TeamID,
			h.Review.ReviewerID,
			*h.Review.Card,
		); err != nil {
			return fmt.Errorf("authorize historical second review: %w", err)
		}
	}
	// Open snapshots bind exactly these two independent submitted authors.
	for i, ref := range h.Cards {
		if i > 1 {
			return access.ErrConflict
		}
		author := h.PrimaryID
		if i == 1 {
			author = h.Review.ReviewerID
		}
		if err := s.Cards.AuthorizeSubmittedHistoryInTransaction(
			ctx,
			tx,
			eventID,
			h.TeamID,
			author,
			ref,
		); err != nil {
			return fmt.Errorf("authorize historical independent review: %w", err)
		}
	}
	if err := s.authorizeDecisionSources(ctx, tx, actor, eventID, h, true); err != nil {
		return fmt.Errorf("authorize historical decision sources: %w", err)
	}
	return nil
}
