package app

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/repositoryaccess"
	"github.com/google/uuid"
)

type lifecycleAccessPollReceipt struct {
	TeamID             uuid.UUID  `json:"team_id"`
	PlanID             uuid.UUID  `json:"plan_id"`
	PlanVersion        int64      `json:"plan_version"`
	PreviousObservedAt *time.Time `json:"previous_observed_at"`
	CurrentObservedAt  time.Time  `json:"current_observed_at"`
	AcceptedGrants     int        `json:"accepted_grants"`
}

// No refresh command follows the live phase change. Only the actual native
// maintenance/poll worker can replace the earlier publication-time observation.
func (r *lifecycleJourney) waitCurrentAccessPoll(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	before := map[uuid.UUID]*time.Time{}
	observedAt := r.timeline.Live
	if !r.livePollAt.IsZero() {
		observedAt = r.livePollAt
	}
	deadline := time.Now().Add(135 * time.Second)
	for time.Now().Before(deadline) {
		all := true
		for index := range r.events {
			item := &r.events[index]
			current := []lifecycleAccessPollReceipt{}
			receipts := item.access
			if len(item.liveAccess) > 0 {
				receipts = item.liveAccess
			}
			for _, original := range receipts {
				path := fmt.Sprintf("/api/v1/events/%s/repositories/%s/access", item.view.ID, original.TeamID)
				var view repositoryaccess.View
				if err := r.owner.call(ctx, http.MethodGet, path, -1, nil, &view, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				if _, seen := before[original.TeamID]; !seen {
					before[original.TeamID] = view.ObservedAt
				}
				if !view.CanManage || !view.GrantsOpen || view.Plan == nil || view.Plan.ID != original.PlanID || view.Plan.Version != original.PlanVersion || view.Plan.Fingerprint != original.Fingerprint || len(view.Flags) != 0 || len(view.Grants) != original.Accepted {
					lifecycleCaptureAccessObservation(t, view, original, fmt.Errorf("current native poll authority/source changed"))
					t.Fatal("current native access poll changed approved source or authority")
				}
				for _, grant := range view.Grants {
					if grant.State != "accepted" || !grant.Managed || grant.Diagnostic != "" || grant.Uncertain {
						t.Fatal("current native poll lost verified accepted grants")
					}
				}
				if view.ObservedAt == nil || view.ObservedAt.Before(observedAt) {
					all = false
					continue
				}
				if !view.ObservedAt.Equal(observedAt) {
					t.Fatal("native access poll observation is outside the actual current domain phase")
				}
				current = append(current, lifecycleAccessPollReceipt{TeamID: original.TeamID, PlanID: original.PlanID, PlanVersion: original.PlanVersion, PreviousObservedAt: before[original.TeamID], CurrentObservedAt: *view.ObservedAt, AcceptedGrants: len(view.Grants)})
			}
			item.accessPoll = current
		}
		if all {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
	t.Fatal("actual native queued access poll did not observe the current live phase within normal reconciliation cadence")
}
