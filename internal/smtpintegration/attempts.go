package smtpintegration

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
)

// sendPrepared is called only after committing the current immutable attempt's
// prepared barrier. A crash or missing durable receipt leaves that attempt
// uncertain; neither reconciliation nor a later call prepares another send.
func (s Service) sendPrepared(
	ctx context.Context,
	d jobs.Delivery,
	message maildelivery.Message,
	source string,
) jobs.Outcome {
	attempt, err := dbgen.New(s.Pool).LatestSMTPSubmissionAttempt(ctx, d.ID)
	if err != nil || attempt.SourceKind != source {
		return jobs.Ambiguous
	}
	receipt := maildelivery.SafeReceipt(s.Transport.Send(ctx, message))
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	err = database.Within(cleanup, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if err := q.RecordSMTPSubmissionReceipt(
			cleanup,
			dbgen.RecordSMTPSubmissionReceiptParams{
				AttemptID:   attempt.ID,
				Outcome:     string(receipt.State),
				ReceiptCode: receipt.Code,
			},
		); err != nil {
			return fmt.Errorf("append SMTP submission receipt: %w", err)
		}
		var projectionErr error
		if source == "test" {
			projectionErr = q.RecordSMTPReceipt(
				cleanup,
				dbgen.RecordSMTPReceiptParams{
					OperationID: d.ID,
					State:       string(receipt.State),
					ReceiptCode: receipt.Code,
				},
			)
		} else {
			projectionErr = q.RecordSMTPCampaignReceipt(
				cleanup,
				dbgen.RecordSMTPCampaignReceiptParams{
					OperationID: d.ID,
					State:       string(receipt.State),
					ReceiptCode: receipt.Code,
				},
			)
		}
		if projectionErr != nil {
			return fmt.Errorf("record current SMTP receipt: %w", projectionErr)
		}
		return nil
	})
	if err != nil {
		return jobs.Ambiguous
	}
	return outcome(string(receipt.State))
}
