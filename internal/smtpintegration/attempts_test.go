package smtpintegration

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
)

func history(t *testing.T, f *fixture, operation uuid.UUID) []dbgen.SMTPSubmissionHistoryRow {
	t.Helper()
	rows, err := dbgen.New(f.pool).SMTPSubmissionHistory(t.Context(), operation)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestSMTPAttemptHistoryRetainsTemporaryRejectionThenAcceptance(t *testing.T) {
	for _, source := range []string{"test", "campaign"} {
		t.Run(source, func(t *testing.T) {
			f := setup(t)
			var d jobs.Delivery
			var handler jobs.Handler = f.service
			if source == "campaign" {
				f.approveProduction()
				d = f.queueProduction()
				handler = CampaignHandler{Service: f.service}
			} else {
				d = f.queue()
			}
			f.transport.state = maildelivery.Retryable
			if handler.Deliver(t.Context(), d) != jobs.Retryable {
				t.Fatal("temporary rejection not retained")
			}
			first := history(t, f, d.ID)
			if len(first) != 1 || first[0].Outcome.String != "retryable" ||
				first[0].ReceiptCode.String != "relay_temporary_rejection" {
				t.Fatal("first bounded SMTP receipt missing")
			}
			f.transport.state = maildelivery.Accepted
			if handler.Deliver(t.Context(), d) != jobs.Delivered {
				t.Fatal("safe retry was not accepted")
			}
			rows := history(t, f, d.ID)
			if len(rows) != 2 || rows[0] != first[0] || rows[0].ID == rows[1].ID ||
				rows[1].Outcome.String != "accepted" ||
				rows[1].ReceiptCode.String != "relay_accepted" {
				t.Fatal("later acceptance overwrote earlier attempt evidence")
			}
			for index, row := range rows {
				if row.OperationID != d.ID || row.SourceKind != source ||
					row.AttemptNumber != int32(index+1) ||
					!row.RecordedAt.Valid ||
					row.RecordedAt.Time.Before(row.PreparedAt) {
					t.Fatal("prepared/receipt correlation or ordering lost")
				}
			}
			encoded, err := json.Marshal(rows)
			if err != nil || strings.Contains(string(encoded), "organiser@example.test") ||
				strings.Contains(string(encoded), f.transport.messages[0].PlainText) {
				t.Fatal("bounded evidence contains a private payload")
			}
			for _, statement := range []string{
				"UPDATE smtp_submission_attempts SET source_kind=source_kind WHERE id=$1",
				"DELETE FROM smtp_submission_attempts WHERE id=$1",
				"UPDATE smtp_submission_receipts SET receipt_code=receipt_code WHERE attempt_id=$1",
				"DELETE FROM smtp_submission_receipts WHERE attempt_id=$1",
			} {
				if _, err = f.pool.Exec(t.Context(), statement, rows[0].ID); err == nil {
					t.Fatal("durable SMTP attempt/receipt evidence was mutable")
				}
			}
			if handler.Deliver(t.Context(), d) != jobs.Delivered || f.transport.count() != 2 ||
				len(history(t, f, d.ID)) != 2 {
				t.Fatal("accepted attempt created a new send/history row")
			}
		})
	}
}

func TestSMTPAttemptHistoryRetainsPreparedCrashWithoutAutomaticReplay(t *testing.T) {
	for _, source := range []string{"test", "campaign"} {
		t.Run(source, func(t *testing.T) {
			f := setup(t)
			var d jobs.Delivery
			var handler jobs.Handler = f.service
			if source == "campaign" {
				f.approveProduction()
				d = f.queueProduction()
				h := CampaignHandler{Service: f.service}
				handler = h
				if _, ready, _, err := h.prepare(t.Context(), d); err != nil || !ready {
					t.Fatal("could not prepare campaign crash barrier")
				}
			} else {
				d = f.queue()
				if _, _, ready, err := f.service.prepare(t.Context(), d); err != nil || !ready {
					t.Fatal("could not prepare test crash barrier")
				}
			}
			rows := history(t, f, d.ID)
			if len(rows) != 1 || rows[0].Outcome.Valid || rows[0].ReceiptCode.Valid ||
				rows[0].RecordedAt.Valid {
				t.Fatal("prepared crash invented an SMTP receipt")
			}
			if handler.Reconcile(t.Context(), d) != jobs.Ambiguous ||
				handler.Deliver(t.Context(), d) != jobs.Ambiguous ||
				f.transport.count() != 0 {
				t.Fatal("prepared uncertain effect replayed automatically")
			}
			after := history(t, f, d.ID)
			if len(after) != 1 || after[0] != rows[0] {
				t.Fatal("reconciliation replaced unresolved prepared evidence")
			}
		})
	}
}
