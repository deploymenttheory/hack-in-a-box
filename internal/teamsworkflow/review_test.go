package teamsworkflow

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/deliveryhistory"
)

func TestReviewedTeamsResendRetainsOriginalReceiptAndUsesOneGrantThroughWorkers(t *testing.T) {
	for _, status := range []int32{202, 500} {
		t.Run(map[int32]string{202: "accepted", 500: "ambiguous"}[status], func(t *testing.T) {
			f := newFixture(t)
			campaignID := f.campaign("reminder", "Synthetic common notice for {{event.name}}")
			campaign, err := f.s.Composer.Campaign(t.Context(), f.owner.ID, f.id, campaignID)
			if err != nil {
				t.Fatal(err)
			}
			p, err := f.s.QueueBroadcast(
				t.Context(),
				f.owner.ID,
				f.id,
				campaignID,
				campaign.Campaign.Campaign.Version,
			)
			if err != nil {
				t.Fatal(err)
			}
			deliveryID := p.History[0].ID
			q := dbgen.New(f.pool)
			delivery, err := q.TeamsWorkflowDelivery(
				t.Context(),
				dbgen.TeamsWorkflowDeliveryParams{EventID: f.id, ID: deliveryID},
			)
			if err != nil {
				t.Fatal(err)
			}
			f.status.Store(status)
			f.execute(delivery.OperationID)
			if f.calls.Load() != 1 {
				t.Fatal("original provider effect missing")
			}
			op, err := q.Operation(t.Context(), delivery.OperationID)
			if err != nil {
				t.Fatal(err)
			}
			original, err := q.TeamsWorkflowLatestAttempt(
				t.Context(),
				dbgen.TeamsWorkflowLatestAttemptParams{EventID: f.id, DeliveryID: deliveryID},
			)
			if err != nil {
				t.Fatal(err)
			}
			f.now = f.now.Add(2 * time.Second)
			f.status.Store(202)
			history := deliveryhistory.Service{
				Pool:      f.pool,
				Jobs:      f.s.Jobs,
				Providers: map[string]deliveryhistory.RecoveryProvider{"teams": f.s},
			}
			input := deliveryhistory.Command{
				RequestID: uuid.New(),
				Action:    "resend_acknowledged",
				Reason:    "Synthetic explicit shared-channel duplicate risk",
			}
			result, err := history.Change(
				t.Context(),
				f.owner.ID,
				f.id,
				deliveryID,
				op.Revision,
				input,
			)
			if err != nil {
				t.Fatal(err)
			}
			if !result.Item.Attempted || result.Item.State == "never_attempted" {
				t.Fatal("review erased prior effect")
			}
			if err = f.s.Jobs.Client.Start(t.Context()); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = f.s.Jobs.Client.StopAndCancel(ctx)
			})
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				current, readErr := q.Operation(t.Context(), op.ID)
				if readErr != nil {
					t.Fatal(readErr)
				}
				if current.State == "delivered" {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if f.calls.Load() != 2 {
				t.Fatalf("physical shared-channel calls=%d want2", f.calls.Load())
			}
			if _, err = history.Change(
				t.Context(),
				f.owner.ID,
				f.id,
				deliveryID,
				op.Revision,
				input,
			); err != nil {
				t.Fatal(err)
			}
			f.execute(op.ID)
			if err = f.s.Jobs.Reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			if f.calls.Load() != 2 {
				t.Fatal("duplicate review or stale worker replayed")
			}
			detail, err := history.Detail(t.Context(), f.owner.ID, f.id, deliveryID, 0, nil)
			if err != nil || len(detail.Attempts) != 2 || detail.Item.State != "provider_accepted" {
				t.Fatalf("retained Teams history %+v err%v", detail, err)
			}
			var retained string
			var uses int
			if err = f.pool.QueryRow(t.Context(), "SELECT outcome FROM teams_workflow_receipts WHERE event_id=$1 AND attempt_id=$2", f.id, original.ID).
				Scan(&retained); err != nil ||
				retained != original.Outcome.String {
				t.Fatal("original receipt changed", err)
			}
			if err = f.pool.QueryRow(t.Context(), "SELECT count(*) FROM communication_delivery_resend_uses WHERE review_id=$1", input.RequestID).
				Scan(&uses); err != nil ||
				uses != 1 {
				t.Fatal("one-use grant missing", err)
			}
		})
	}
}
