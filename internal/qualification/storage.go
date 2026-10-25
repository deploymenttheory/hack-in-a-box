package qualification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func load(ctx context.Context, tx pgx.Tx, eventID, runID uuid.UUID) (Round, error) {
	out := Round{RunID: runID, Cards: []FinalCard{}, Eligibility: []Eligibility{}, Votes: []Vote{}, Decisions: []TieDecision{}, Panel: Panel{MentorIDs: []uuid.UUID{}}}
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT document FROM mentor_qualification_rounds WHERE event_id=$1 AND run_id=$2 ORDER BY version DESC LIMIT 1`, eventID, runID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("read qualification round: %w", err)
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("decode qualification round: %w", err)
	}
	return out, nil
}

func (s Service) retain(ctx context.Context, tx pgx.Tx, eventID, actor uuid.UUID, previous int64, round *Round, command Command) error {
	round.Version++
	round.ActorID = actor
	round.Action = command.Action
	round.Reason = command.Reason
	round.ChangedAt = s.now()
	raw, err := json.Marshal(round)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO mentor_qualification_rounds(event_id,run_id,version,actor_id,action,reason,document,changed_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, eventID, round.RunID, round.Version, actor, command.Action, command.Reason, raw, round.ChangedAt); err != nil {
		return fmt.Errorf("retain qualification decision: %w", err)
	}
	for _, card := range round.Cards {
		if _, err = tx.Exec(ctx, `INSERT INTO mentor_qualification_sources(event_id,run_id,round_version,team_id,signoff_id)VALUES($1,$2,$3,$4,$5)`, eventID, round.RunID, round.Version, card.TeamID, card.Source.SignoffID); err != nil {
			return fmt.Errorf("retain qualification native source: %w", err)
		}
	}
	q := dbgen.New(tx)
	row, err := q.BumpEventRevision(ctx, eventID)
	if err != nil {
		return err
	}
	return audit.Append(ctx, q, audit.Record{ActorID: &actor, EventID: &eventID, TargetID: round.RunID, Action: audit.ModerationChanged, Reason: audit.UserRequest, Metadata: audit.Metadata{PreviousRevision: &previous, Revision: &row.Revision}})
}
