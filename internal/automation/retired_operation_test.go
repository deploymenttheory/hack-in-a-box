package automation

import (
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
)

func TestRetiredDispatchCannotMutateReviewedCancelledOrQueuedOccurrence(t *testing.T) {
	for _, state := range []string{"pending", "cancelled", "queued", "partially_queued"} {
		t.Run(state, func(t *testing.T) {
			f := newFixture(t)
			f.campaign("reminder", "Current approved reminder", true)
			f.bind()
			occurrence := f.maintain().Occurrences[0]
			q := dbgen.New(f.pool)
			read := func() dbgen.CommunicationOccurrence {
				row, err := q.AutomationOccurrence(
					t.Context(),
					dbgen.AutomationOccurrenceParams{EventID: f.id, ID: occurrence.ID},
				)
				if err != nil {
					t.Fatal(err)
				}
				return row
			}
			original := read()
			retired := jobs.Delivery{
				ID:       *original.OperationID,
				EventID:  f.id,
				TargetID: occurrence.ID,
			}
			f.now = f.now.Add(3 * time.Minute)
			f.maintain()
			f.change(
				Command{
					Action:       "review_occurrence",
					OccurrenceID: occurrence.ID,
					Reason:       "Explicitly review current generation",
				},
			)
			current := read()
			if *current.OperationID == retired.ID ||
				current.ReviewRevision <= original.ReviewRevision {
				t.Fatal("review did not replace generation")
			}
			if state != "pending" {
				f.exec(
					`UPDATE communication_occurrences SET state=$3 WHERE event_id=$1 AND id=$2`,
					f.id,
					occurrence.ID,
					state,
				)
			}
			before := read()
			var historyBefore, historyAfter int
			if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM communication_automation_history WHERE event_id=$1`, f.id).
				Scan(&historyBefore); err != nil {
				t.Fatal(err)
			}
			f.s.Deliver(t.Context(), retired)
			if state != "pending" {
				f.s.Deliver(
					t.Context(),
					jobs.Delivery{ID: *before.OperationID, EventID: f.id, TargetID: occurrence.ID},
				)
			}
			after := read()
			if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM communication_automation_history WHERE event_id=$1`, f.id).
				Scan(&historyAfter); err != nil {
				t.Fatal(err)
			}
			if after.State != before.State || *after.OperationID != *before.OperationID ||
				after.ReviewRevision != before.ReviewRevision ||
				after.Reason != before.Reason ||
				!after.UpdatedAt.Equal(before.UpdatedAt) ||
				historyAfter != historyBefore {
				t.Fatalf(
					"retired dispatch changed current %s generation: before%+v after%+v",
					state,
					before,
					after,
				)
			}
			if state == "pending" {
				f.execute(occurrence.ID)
				f.execute(occurrence.ID)
				if f.count() != 1 {
					t.Fatal("current reviewed generation did not execute exactly once")
				}
			}
		})
	}
}
