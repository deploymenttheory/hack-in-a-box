package smtpintegration

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
)

func pendingResend(ctx context.Context, tx pgx.Tx, operationID uuid.UUID) (uuid.UUID, error) {
	a, err := dbgen.New(tx).LatestSMTPSubmissionAttempt(ctx, operationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, nil
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("read SMTP resend attempt: %w", err)
	}
	grant, err := jobs.ResendGrantInTransaction(ctx, tx, operationID, a.ID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("check SMTP resend grant: %w", err)
	}
	return grant, nil
}

func (s Service) reviewedResend(ctx context.Context, d jobs.Delivery) (bool, error) {
	valid := false
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(ctx, d.EventID); err != nil {
			return fmt.Errorf("lock SMTP resend check: %w", err)
		}
		grant, err := pendingResend(ctx, tx, d.ID)
		valid = grant != uuid.Nil
		return err
	})
	if err != nil {
		return false, fmt.Errorf("check reviewed SMTP resend: %w", err)
	}
	return valid, nil
}

// ValidateRecoveryInTransaction applies the same current source/provider/
// mailbox/snapshot policy as execution without preparing an attempt or sending.
func (s Service) ValidateRecoveryInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	d jobs.Delivery,
) error {
	if !s.configured() || s.Campaigns == nil {
		return access.ErrForbidden
	}
	return s.withCurrentCampaign(
		ctx,
		tx,
		d,
		func(_ dbgen.SmtpCampaignMessage, _ campaignSnapshot) error {
			op, err := dbgen.New(tx).Operation(ctx, d.ID)
			if err != nil {
				return fmt.Errorf("read current SMTP recovery operation: %w", err)
			}
			if !mailOperationIdentityCurrent(op, d, s.now(), "communication_campaign_smtp") {
				return access.ErrForbidden
			}
			return nil
		},
	)
}
