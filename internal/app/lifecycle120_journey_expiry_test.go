package app

import (
	"net/http"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/archives"
	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
)

type lifecycleExpiryReceipt struct {
	EventID              uuid.UUID `json:"event_id"`
	CompletedAt          time.Time `json:"native_completed_at"`
	RetentionDays        int32     `json:"retention_days"`
	Deadline             time.Time `json:"exclusive_deadline"`
	OwnerReadableBefore  bool      `json:"owner_readable_one_second_before"`
	OwnerDeniedAt        bool      `json:"owner_denied_at_deadline"`
	OrganiserDeniedAt    bool      `json:"organiser_denied_at_deadline"`
	NotificationDeniedAt bool      `json:"notification_denied_at_deadline"`
	CompetitionUnchanged bool      `json:"competition_unchanged"`
}

// Expiry uses the original native completion anchor and configured policy. The
// event journey created every source; no SQL policy/anchor or job edits supply
// this boundary. Read-time denial must hold whether deletion maintenance ran.
func (r *lifecycleJourney) expireRetainedOutcomes(t *testing.T) {
	t.Helper()
	order := make([]int, 0, len(r.events))
	for i := range r.events {
		item := &r.events[i]
		base := "/api/v1/events/" + item.view.ID.String()
		var policy privacy.Settings
		if err := r.owner.call(
			t.Context(),
			http.MethodGet,
			base+"/privacy",
			-1,
			nil,
			&policy,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		var archive archives.View
		if err := r.owner.call(
			t.Context(),
			http.MethodGet,
			base+"/archive",
			-1,
			nil,
			&archive,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if policy.RetentionDays != lifecycleRetentionDays(t) || archive.CompletedAt == nil {
			t.Fatal("expiry case lost its original native completion/policy")
		}
		// Calendar addition is independently applied to the observed native anchor;
		// no implementation retention helper defines the test's expected instant.
		deadline := archive.CompletedAt.UTC().AddDate(0, 0, int(lifecycleRetentionDays(t)))
		item.expiry = lifecycleExpiryReceipt{
			EventID:       item.view.ID,
			CompletedAt:   *archive.CompletedAt,
			RetentionDays: policy.RetentionDays,
			Deadline:      deadline,
		}
		order = append(order, i)
	}
	sort.Slice(order, func(i, j int) bool {
		return r.events[order[i]].expiry.Deadline.Before(r.events[order[j]].expiry.Deadline)
	})
	for start := 0; start < len(order); {
		end := start + 1
		deadline := r.events[order[start]].expiry.Deadline
		for end < len(order) && r.events[order[end]].expiry.Deadline.Equal(deadline) {
			end++
		}
		if err := r.control.advance(t.Context(), deadline.Add(-time.Second)); err != nil {
			t.Fatal(err)
		}
		for _, i := range order[start:end] {
			item := &r.events[i]
			owner := r.participant(t, *item.outcomes[0].OwnerID)
			path := "/api/v1/outcomes/" + item.outcomes[0].ID.String()
			var before outcomes.View
			if err := owner.call(
				t.Context(),
				http.MethodGet,
				path,
				-1,
				nil,
				&before,
				http.StatusOK,
			); err != nil {
				t.Fatal(err)
			}
			if before.ID != item.outcomes[0].ID {
				t.Fatal("before-expiry read substituted the retained outcome")
			}
			item.expiry.OwnerReadableBefore = true
		}
		if err := r.control.advance(t.Context(), deadline); err != nil {
			t.Fatal(err)
		}
		for _, i := range order[start:end] {
			item := &r.events[i]
			owner := r.participant(t, *item.outcomes[0].OwnerID)
			path := "/api/v1/outcomes/" + item.outcomes[0].ID.String()
			for _, client := range []*lifecycleClient{owner, r.owner} {
				for _, suffix := range []string{"", "/history"} {
					if err := client.call(
						t.Context(),
						http.MethodGet,
						path+suffix,
						-1,
						nil,
						nil,
						http.StatusNotFound,
					); err != nil {
						t.Fatal(err)
					}
				}
			}
			item.expiry.OwnerDeniedAt = true
			item.expiry.OrganiserDeniedAt = true
			if len(item.reviews) == 0 {
				t.Fatal("expiry has no actual delivered review")
			}
			review := item.reviews[len(item.reviews)-1]
			if review.NotificationID == uuid.Nil {
				t.Fatal("expiry test lacks the actual last approved review notification")
			}
			if err := owner.call(
				t.Context(),
				http.MethodGet,
				"/api/v1/inbox/"+review.NotificationID.String(),
				-1,
				nil,
				nil,
				http.StatusNotFound,
			); err != nil {
				t.Fatal(err)
			}
			item.expiry.NotificationDeniedAt = true
			if !reflect.DeepEqual(
				item.competitionBefore,
				lifecycleCompetitionSnapshot(t, item.view.ID, len(item.review.FinalHuddles)),
			) {
				t.Fatal("read-time expiry mutated frozen competition evidence")
			}
			item.expiry.CompetitionUnchanged = true
		}
		start = end
	}
}
