package smtpintegration

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
)

func TestReviewedSMTPResendConsumesOneGrantWithoutRewritingPriorReceipt(t *testing.T) {
	for _, prior := range []maildelivery.State{maildelivery.Accepted, maildelivery.Ambiguous} {
		t.Run(string(prior), func(t *testing.T) {
			f := setup(t)
			f.approveProduction()
			d := f.queueProduction()
			h := CampaignHandler{Service: f.service}
			f.transport.state = prior
			if got := h.Deliver(t.Context(), d); got != outcome(string(prior)) {
				t.Fatal(got)
			}
			if f.transport.count() != 1 {
				t.Fatal("initial provider call missing")
			}
			if got := h.Deliver(
				t.Context(),
				d,
			); got != outcome(string(prior)) ||
				f.transport.count() != 1 {
				t.Fatal("automatic call replayed original evidence")
			}
			state := "delivered"
			if prior == maildelivery.Ambiguous {
				state = "ambiguous"
			}
			f.exec("UPDATE operations SET state=$2 WHERE id=$1", d.ID, state)
			q := dbgen.New(f.pool)
			op, err := q.Operation(t.Context(), d.ID)
			if err != nil {
				t.Fatal(err)
			}
			original, err := q.LatestSMTPSubmissionAttempt(t.Context(), d.ID)
			if err != nil {
				t.Fatal(err)
			}
			reviewID := uuid.New()
			err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, q *dbgen.Queries) error {
				if err := q.RecordDeliveryReview(
					t.Context(),
					dbgen.RecordDeliveryReviewParams{
						ID:               reviewID,
						EventID:          f.event,
						OperationID:      d.ID,
						ActorID:          f.owner.ID,
						ExpectedRevision: op.Revision,
						Action:           "resend_acknowledged",
						Reason:           "Synthetic reviewed possible duplicate",
						PriorAttemptID:   &original.ID,
					},
				); err != nil {
					return err
				}
				_, err := q.ReviewRetryOperation(
					t.Context(),
					dbgen.ReviewRetryOperationParams{ID: d.ID, Revision: op.Revision},
				)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			f.transport.state = maildelivery.Accepted
			if got := h.Deliver(t.Context(), d); got != jobs.Delivered {
				t.Fatalf("reviewed resend=%s", got)
			}
			if f.transport.count() != 2 {
				t.Fatalf("reviewed resend physical calls=%d want2", f.transport.count())
			}
			if got := h.Deliver(t.Context(), d); got != jobs.Delivered || f.transport.count() != 2 {
				t.Fatal("spent authorisation replayed")
			}
			var attempts, receipts, uses int
			if err = f.pool.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM smtp_submission_attempts WHERE operation_id=$1),(SELECT count(*) FROM smtp_submission_receipts r JOIN smtp_submission_attempts a ON a.id=r.attempt_id WHERE a.operation_id=$1),(SELECT count(*) FROM communication_delivery_resend_uses WHERE review_id=$2)", d.ID, reviewID).
				Scan(&attempts, &receipts, &uses); err != nil ||
				attempts != 2 ||
				receipts != 2 ||
				uses != 1 {
				t.Fatalf("evidence=%d/%d uses%d error%v", attempts, receipts, uses, err)
			}
			var retained string
			if err = f.pool.QueryRow(t.Context(), "SELECT outcome FROM smtp_submission_receipts WHERE attempt_id=$1", original.ID).
				Scan(&retained); err != nil ||
				retained != string(prior) {
				t.Fatal("original immutable outcome changed", err)
			}
			if f.transport.messages[0].ID != f.transport.messages[1].ID ||
				f.transport.messages[0].SentAt != f.transport.messages[1].SentAt ||
				f.transport.messages[0].Calendar.Sequence != f.transport.messages[1].Calendar.Sequence {
				t.Fatal("resend rewrote immutable message/calendar")
			}
		})
	}
}
