package results

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func (s Service) retain(
	ctx context.Context,
	tx pgx.Tx,
	eventID, actor uuid.UUID,
	previous int64,
	round *Round,
	input Command,
) error {
	round.Version++
	round.ActorID = actor
	round.Action = input.Action
	round.Reason = input.Reason
	round.ChangedAt = s.now()
	raw, err := json.Marshal(round)
	if err != nil {
		return failure("encode decision", err)
	}
	var judgeRun, approval, qualRun *uuid.UUID
	var qualVersion *int64
	if round.Source.JudgeRunID != uuid.Nil {
		judgeRun = &round.Source.JudgeRunID
	}
	if round.Confirmation != nil {
		approval = &round.Confirmation.JudgeApprovalID
	}
	if round.Source.Selection != nil {
		qualRun = &round.Source.Selection.RunID
		qualVersion = &round.Source.Selection.RoundVersion
	}
	_, err = tx.Exec(
		ctx,
		"INSERT INTO result_versions(event_id,version,judge_run_id,judge_approval_id,qualification_run_id,qualification_version,actor_id,action,reason,document,changed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)",
		eventID,
		round.Version,
		judgeRun,
		approval,
		qualRun,
		qualVersion,
		actor,
		input.Action,
		input.Reason,
		raw,
		round.ChangedAt,
	)
	if err != nil {
		return failure("retain decision", err)
	}
	q := dbgen.New(tx)
	row, err := q.BumpEventRevision(ctx, eventID)
	if err != nil {
		return failure("advance event revision", err)
	}
	return failure(
		"audit decision",
		audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &eventID,
				TargetID: eventID,
				Action:   audit.ModerationChanged,
				Reason:   audit.UserRequest,
				Metadata: audit.Metadata{PreviousRevision: &previous, Revision: &row.Revision},
			},
		),
	)
}
