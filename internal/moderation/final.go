package moderation

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
)

type FinalDecision struct {
	TeamID         uuid.UUID `json:"team_id"`
	RunID          uuid.UUID `json:"run_id"`
	HuddleID       uuid.UUID `json:"huddle_id"`
	HuddleVersion  int64     `json:"huddle_version"`
	ContextVersion int64     `json:"context_version"`
	SignoffID      uuid.UUID `json:"signoff_id"`
}

func finalDecision(
	ctx context.Context,
	tx pgx.Tx,
	eventID, teamID, runID uuid.UUID,
) (FinalDecision, error) {
	out := FinalDecision{TeamID: teamID, RunID: runID}
	err := tx.QueryRow(ctx, `SELECT huddle_id,huddle_version,context_version,signoff_id FROM moderation_final_decisions WHERE event_id=$1 AND team_id=$2 AND run_id=$3`, eventID, teamID, runID).
		Scan(&out.HuddleID, &out.HuddleVersion, &out.ContextVersion, &out.SignoffID)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, access.ErrNotFound
	}
	if err != nil {
		return out, fmt.Errorf("read canonical final moderation decision: %w", err)
	}
	return out, nil
}

func (s Service) signOff(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	h *Huddle,
) error {
	if actor != h.Definition.ModeratorID || h.State != "open" ||
		h.Checkpoint.Purpose != "mentor_final" {
		return access.ErrForbidden
	}
	if _, err := finalDecision(
		ctx,
		tx,
		eventID,
		h.TeamID,
		h.Checkpoint.Definition.RunID,
	); err == nil {
		return access.ErrConflict
	} else if !errors.Is(
		err,
		access.ErrNotFound,
	) {
		return err
	}
	native := *s.Cards
	native.Moderation = &s
	signed, err := native.SignOffInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		h.TeamID,
		h.Definition.ID,
		h.Cards,
	)
	if err != nil {
		return fmt.Errorf("sign complete exact native final cards: %w", err)
	}
	// This owner supplies one canonical final source per team/native final run.
	// Other sessions retain history; only an explicit new native run may sign a
	// different final source. No checkpoint totals or awards are calculated.
	if _, err = tx.Exec(
		ctx,
		`INSERT INTO moderation_final_decisions(event_id,team_id,run_id,huddle_id,huddle_version,context_version,signoff_id)VALUES($1,$2,$3,$4,$5,$6,$7)`,
		eventID,
		h.TeamID,
		h.Checkpoint.Definition.RunID,
		h.Definition.ID,
		h.Version+1,
		h.Version,
		signed.ID,
	); err != nil {
		return fmt.Errorf("retain canonical final moderation decision: %w", err)
	}
	h.State = "signed"
	h.SignoffID = &signed.ID
	h.SignoffContext = &signed.Context
	return nil
}

func (s Service) signedRetry(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	h Huddle,
	input Command,
) error {
	if h.SignoffContext == nil || h.SignoffID == nil || actor != h.Definition.ModeratorID ||
		input.Reason != h.Reason ||
		(input.Version != h.Version && input.Version != h.SignoffContext.Version) {
		return access.ErrConflict
	}
	if _, err := s.current(ctx, tx, actor, eventID, h, true); err != nil {
		return err
	}
	decision, err := finalDecision(ctx, tx, eventID, h.TeamID, h.Checkpoint.Definition.RunID)
	if err != nil {
		return err
	}
	if decision.HuddleID != h.Definition.ID || decision.HuddleVersion != h.Version ||
		decision.ContextVersion != h.SignoffContext.Version ||
		decision.SignoffID != *h.SignoffID {
		return access.ErrConflict
	}
	native := *s.Cards
	native.Moderation = &s
	signed, err := native.SignOffInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		h.TeamID,
		h.Definition.ID,
		h.Cards,
	)
	if err != nil {
		return fmt.Errorf("recheck unchanged canonical final source: %w", err)
	}
	if signed.ID != decision.SignoffID {
		return access.ErrConflict
	}
	return nil
}
