package judging

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

type Rounds struct {
	Items []Round `json:"items"`
}

// Rounds discovers the owning current judge_final run without requiring an
// organiser rubric endpoint or inferring a judge grant from an event role.
func (s Service) Rounds(ctx context.Context, actor, eventID uuid.UUID) (Rounds, error) {
	out := Rounds{Items: []Round{}}
	ids := []uuid.UUID{}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := s.guard(ctx, tx, actor, eventID, false); err != nil {
			return err
		}
		runs, err := q.CurrentRubricRuns(ctx, eventID)
		if err != nil {
			return failure("discover current runs", err)
		}
		for _, run := range runs {
			if run.Purpose == "judge_final" {
				ids = append(ids, run.ID)
			}
		}
		return nil
	})
	if err != nil {
		return out, failure("discover rounds", err)
	}
	for _, id := range ids {
		r, err := s.Round(ctx, actor, eventID, id)
		if err != nil {
			return Rounds{}, err
		}
		out.Items = append(out.Items, r)
	}
	return out, nil
}

type Recovery struct {
	Receipt *Receipt `json:"receipt"`
}

func (s Service) OriginalReceipt(
	ctx context.Context,
	actor, eventID, requestID uuid.UUID,
) (Recovery, error) {
	var out Recovery
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		a, err := s.guard(ctx, tx, actor, eventID, false)
		if err != nil {
			return err
		}
		var raw []byte
		err = tx.QueryRow(ctx, "SELECT result FROM judge_commands WHERE event_id=$1 AND request_id=$2 AND actor_id=$3", eventID, requestID, actor).
			Scan(&raw)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return failure("receipt store", err)
		}
		var r Receipt
		if err = json.Unmarshal(raw, &r); err != nil {
			return failure("decode original acknowledgement", err)
		}
		if r.RunID != uuid.Nil && !a.organiser {
			ids, err := assigned(ctx, tx, eventID, r.TeamID)
			if err != nil {
				return err
			}
			allowed := false
			for _, id := range ids {
				if id == actor {
					allowed = true
				}
			}
			if !allowed {
				return access.ErrForbidden
			}
			ok, err := eligibleJudge(ctx, tx, eventID, r.TeamID, actor)
			if err != nil {
				return err
			}
			if !ok {
				return access.ErrForbidden
			}
		}
		out.Receipt = &r
		return nil
	})
	return out, failure("original receipt", err)
}
