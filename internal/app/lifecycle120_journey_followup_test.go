package app

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/inbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
	"github.com/google/uuid"
)

type lifecycleOwnedReviewReceipt struct {
	OutcomeID      uuid.UUID `json:"outcome_id"`
	OwnerID        uuid.UUID `json:"owner_id"`
	ReviewID       uuid.UUID `json:"review_id"`
	Days           int       `json:"days"`
	Version        int64     `json:"plan_version"`
	DueAt          time.Time `json:"due_at"`
	NotificationID uuid.UUID `json:"notification_id"`
}

// Approval is explicit while completed; the native archive transition must
// preserve only the exact retained source/owner/composition/timing grants.
func (r *lifecycleJourney) prepareRetainedReviews(t *testing.T) {
	t.Helper()
	location, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	for i := range r.events {
		item := &r.events[i]
		if len(item.outcomes) < 2 {
			t.Fatal("retained reviews lack distinct team outcome owners")
		}
		// Archive's audited progress exercise uses index zero. These plans remain
		// bound to a separate unchanged source rather than transferring approval.
		source := item.outcomes[1]
		for _, days := range []int{30, 60, 90} {
			id := uuid.New()
			due := r.timeline.Completed.In(location).AddDate(0, 0, days)
			path := "/api/v1/outcomes/" + source.ID.String() + "/reviews/" + id.String()
			var plan outcomes.PlanView
			command := outcomes.PlanCommand{ID: uuid.New(), Action: "save", SourceVersion: source.Version.Version, Reason: "Explicit retained synthetic owner review and response window", Schedule: &outcomes.ReviewSchedule{Days: days, WallTime: due.Format("2006-01-02T15:04:05"), Timezone: "Europe/London", Channels: []string{"in_app"}}, Message: &outcomes.ReviewMessage{Subject: fmt.Sprintf("Owned day-%d review", days), Body: "Hello {{recipient.name}}; review {{outcome.url}} at {{review.date}}."}}
			if err := r.owner.call(t.Context(), http.MethodPut, path, 0, command, &plan, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			command = outcomes.PlanCommand{ID: uuid.New(), Action: "approve", SourceVersion: source.Version.Version, Reason: "Approve exact owned source, private composition and retained timing"}
			if err := r.owner.call(t.Context(), http.MethodPut, path, plan.Version, command, &plan, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if plan.State != "approved" || plan.CommandID != command.ID || plan.OwnerID != *source.OwnerID || !plan.DueAt.Equal(due) || plan.SourceVersion != source.Version.Version {
				t.Fatal("native review approval lost exact owned schedule/source")
			}
			item.reviews = append(item.reviews, lifecycleOwnedReviewReceipt{OutcomeID: source.ID, OwnerID: *source.OwnerID, ReviewID: id, Days: days, Version: plan.Version, DueAt: due.UTC()})
		}
	}
}

func (r *lifecycleJourney) deliverRetainedReviews(t *testing.T) {
	t.Helper()
	// Domain eligibility advances monotonically. River leases, scheduling and
	// HTTP authentication keep real wall time; no jobs or receipts are seeded.
	for phase, days := range []int{30, 60, 90} {
		due := r.events[0].reviews[phase].DueAt
		if err := r.control.advance(t.Context(), due); err != nil {
			t.Fatal(err)
		}
		for i := range r.events {
			item := &r.events[i]
			owner := r.participant(t, item.reviews[phase].OwnerID)
			path := "/api/v1/inbox?event_id=" + item.view.ID.String() + "&limit=100"
			observed := map[uuid.UUID]uuid.UUID{}
			opened := map[uuid.UUID]bool{}
			deadline := time.Now().Add(75 * time.Second)
			for time.Now().Before(deadline) {
				var page inbox.Page
				if err := owner.call(t.Context(), http.MethodGet, path, -1, nil, &page, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				if page.NextCursor != nil {
					t.Fatal("review inbox census requires pagination rather than silently omitting evidence")
				}
				for _, entry := range page.Items {
					if opened[entry.ID] {
						continue
					}
					var detail inbox.Detail
					if err := owner.call(t.Context(), http.MethodGet, "/api/v1/inbox/"+entry.ID.String(), -1, nil, &detail, http.StatusOK); err != nil {
						t.Fatal(err)
					}
					opened[entry.ID] = true
					if detail.Destination == nil || detail.Destination.Kind != "outcome_review" {
						continue
					}
					matched := false
					for _, review := range item.reviews {
						if review.ReviewID != detail.Destination.ResourceID {
							continue
						}
						if review.Days > days || detail.Subject != fmt.Sprintf("Owned day-%d review", review.Days) || entry.EventName != "" {
							t.Fatal("retained review sent early or disclosed private event context")
						}
						if previous, exists := observed[review.ReviewID]; exists && previous != entry.ID {
							t.Fatal("owned review created duplicate native notifications")
						}
						observed[review.ReviewID] = entry.ID
						matched = true
					}
					if !matched {
						t.Fatal("retained inbox reminder does not reference an approved native plan")
					}
				}
				if len(observed) == phase+1 {
					break
				}
				time.Sleep(250 * time.Millisecond)
			}
			if len(observed) != phase+1 {
				t.Fatalf("day-%d retained queued review census did not settle", days)
			}
			item.reviews[phase].NotificationID = observed[item.reviews[phase].ReviewID]
			if item.reviews[phase].NotificationID == uuid.Nil {
				t.Fatal("current owned review was absent from actual native inbox")
			}
			if !reflect.DeepEqual(item.competitionBefore, lifecycleCompetitionSnapshot(t, item.view.ID, len(item.review.FinalHuddles))) {
				t.Fatal("retained queued follow-up mutated frozen competition evidence")
			}
		}
	}
}
