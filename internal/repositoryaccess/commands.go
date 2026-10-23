package repositoryaccess

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func (s Service) changeLocked(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	e dbgen.Event,
	r dbgen.TeamRepository,
	actor uuid.UUID,
	command Command,
) (*Plan, error) {
	switch command.Action {
	case "refresh":
		if err := s.refresh(ctx, tx, q, e, r); err != nil {
			return nil, err
		}
		return readPlan(ctx, tx, e.ID, r.TeamID)
	case "review":
		return s.review(ctx, tx, q, e, r, actor, command)
	case "approve":
		return s.approve(ctx, tx, q, e, r, actor, command)
	default:
		return nil, access.ErrInvalid
	}
}

func (s Service) review(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	e dbgen.Event,
	r dbgen.TeamRepository,
	actor uuid.UUID,
	command Command,
) (*Plan, error) {
	var encoded []byte
	err := tx.QueryRow(ctx, "SELECT definition FROM repository_access_plans WHERE id=$1 AND event_id=$2 AND team_id=$3", command.PlanID, e.ID, r.TeamID).
		Scan(&encoded)
	if err == nil {
		var receipt Plan
		if err = json.Unmarshal(encoded, &receipt); err != nil {
			return nil, fmt.Errorf("read exact review receipt: %w", err)
		}
		if receipt.Reason != strings.TrimSpace(command.Reason) {
			return nil, access.ErrConflict
		}
		return &receipt, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("check exact review identity: %w", err)
	}
	if err = s.refresh(ctx, tx, q, e, r); err != nil {
		return nil, err
	}
	return s.createPlan(
		ctx,
		tx,
		q,
		e,
		r,
		actor,
		command.PlanID,
		true,
		strings.TrimSpace(command.Reason),
	)
}

func (s Service) approve(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	e dbgen.Event,
	r dbgen.TeamRepository,
	actor uuid.UUID,
	command Command,
) (*Plan, error) {
	plan, err := readPlan(ctx, tx, e.ID, r.TeamID)
	if err != nil {
		return nil, err
	}
	if plan == nil || plan.ID != command.PlanID || plan.Version != command.PlanVersion {
		return nil, access.ErrConflict
	}
	members, _, err := desired(ctx, tx, q, e, r, actor, s.now())
	if err != nil {
		return nil, err
	}
	if plan.Fingerprint != fingerprint(r.GithubRepositoryID.Int64, r.Provenance, members) {
		return nil, access.ErrConflict
	}
	if plan.ApprovedAt == nil {
		at := s.now()
		if _, err = tx.Exec(
			ctx,
			"UPDATE repository_access_plans SET approved_by=$2,approved_at=$3 WHERE id=$1 AND approved_at IS NULL",
			plan.ID,
			actor,
			at,
		); err != nil {
			return nil, fmt.Errorf("approve exact repository access plan: %w", err)
		}
		plan.ApprovedAt = &at
		plan.ApprovedBy = &actor
	}
	if err = s.queue(ctx, tx, q, e, r, *plan); err != nil {
		return nil, err
	}
	return plan, nil
}
