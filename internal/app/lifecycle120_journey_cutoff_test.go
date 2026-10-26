package app

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissionfreeze"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
)

type lifecycleCutoffReceipt struct {
	TeamID              uuid.UUID  `json:"team_id"`
	SourceID            uuid.UUID  `json:"source_id"`
	Outcome             string     `json:"native_outcome"`
	SubmissionRevision  *int64     `json:"submission_revision"`
	Status              string     `json:"status"`
	Cutoff              time.Time  `json:"cutoff"`
	RecordedAt          time.Time  `json:"recorded_at"`
	SelectedSHA         string     `json:"selected_sha,omitempty"`
	LatestObservedSHA   string     `json:"latest_observed_sha,omitempty"`
	DecisionID          *uuid.UUID `json:"decision_id,omitempty"`
	ProviderUnavailable int        `json:"provider_unavailable_observations"`
}

func (r *lifecycleJourney) freezePage(t *testing.T, item *lifecycleJourneyEvent, teamID uuid.UUID) submissionfreeze.Page {
	t.Helper()
	var page submissionfreeze.Page
	path := fmt.Sprintf("/api/v1/events/%s/teams/%s/submission-freeze", item.view.ID, teamID)
	if err := r.owner.call(t.Context(), http.MethodGet, path, -1, nil, &page, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	return page
}

func (r *lifecycleJourney) captureAndReconcileCutoff(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	var target lifecycleWorkReceipt
	for _, submission := range r.events[0].submissions {
		if submission.FormalRevision > 0 && submission.Kind == "repository" {
			for _, work := range r.events[0].work {
				if work.TeamID == submission.TeamID {
					target = work
				}
			}
			break
		}
	}
	if target.ID == uuid.Nil {
		t.Fatal("cutoff failure case has no actual preweek external work receipt")
	}
	if err := r.control.call(ctx, http.MethodPost, "/github/branch-fault", map[string]any{"repository": target.Name, "failures": 1}, nil); err != nil {
		t.Fatal(err)
	}
	if err := r.control.advance(ctx, r.timeline.Cutoff); err != nil {
		t.Fatal(err)
	}
	failed := false
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		page := r.freezePage(t, &r.events[0], target.TeamID)
		if page.Current != nil {
			t.Fatal("native source selected a SHA without organiser reconciliation")
		}
		for _, observation := range page.Observations {
			if observation.Status == "provider_unavailable" {
				failed = true
			}
		}
		if failed {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
	if !failed {
		t.Fatal("real queued cutoff capture did not retain its actual provider failure")
	}
	if err := r.control.advance(ctx, r.timeline.Cutoff.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	var member *lifecycleClient
	for _, team := range r.events[0].teams.Teams {
		if team.ID == target.TeamID {
			member = r.participant(t, team.Published[0].ID)
		}
	}
	if member == nil {
		t.Fatal("late work case lost its current team member")
	}
	late := r.externalWork(t, member, target.TeamID, target.Name, target.SHA, "Synthetic later work deliberately after native cutoff")
	if !late.DomainObservedAt.After(r.timeline.Cutoff) || late.SHA == target.SHA {
		t.Fatal("late external work was not a separate post-cutoff transaction")
	}
	r.events[0].lateWork = &late
	// A later current ref is not a replacement submission, even while the event
	// stage remains live. The member command uses the exact current native state.
	submissionPath := fmt.Sprintf("/api/v1/events/%s/teams/%s/submission", r.events[0].view.ID, target.TeamID)
	var frozen submissions.Page
	if err := member.call(ctx, http.MethodGet, submissionPath, -1, nil, &frozen, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if frozen.Outcome != "frozen" || frozen.Final == nil || frozen.FrozenAt == nil || !frozen.FrozenAt.Equal(r.timeline.Cutoff) {
		t.Fatal("native cutoff did not freeze the actual formal entry before stage advancement")
	}
	changed := append([]submissions.Evidence{}, frozen.Final.Evidence...)
	changed[0].URL = "https://github.com/" + lifecycleGitHubOrganization + "/" + target.Name + "/commit/" + late.SHA
	var rejected struct {
		Error struct {
			Blockers []event.Issue `json:"blockers"`
		} `json:"error"`
	}
	if err := member.call(ctx, http.MethodPut, submissionPath, frozen.Revision, submissions.Command{Evidence: changed}, &rejected, http.StatusConflict); err != nil {
		t.Fatal(err)
	}
	if len(rejected.Error.Blockers) != 1 || rejected.Error.Blockers[0].Code != "locked" || rejected.Error.Blockers[0].Field != "submission_cutoff" {
		t.Fatal("late formal replacement was not rejected at the native cutoff gate")
	}
	var unchanged submissions.Page
	if err := member.call(ctx, http.MethodGet, submissionPath, -1, nil, &unchanged, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(frozen, unchanged) {
		t.Fatal("rejected late replacement changed the native frozen evidence")
	}

	// Observe natural retry through the actual queue. Read-only native pages never
	// invoke a handler or promote the late current head to historical proof.
	deadline = time.Now().Add(90 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		ready = true
		for index := range r.events {
			item := &r.events[index]
			for _, team := range item.teams.Teams {
				page := r.freezePage(t, item, team.ID)
				if page.Source == nil {
					ready = false
					continue
				}
				if !page.Source.Cutoff.Equal(r.timeline.Cutoff) || page.Source.EntryID == uuid.Nil {
					t.Fatal("queued capture lost its immutable native cutoff/entry binding")
				}
				if page.Source.NativeOutcome == "frozen" && page.CurrentReferenceStatus == "managed_reference" {
					expected := ""
					for _, work := range item.work {
						if work.TeamID == team.ID {
							expected = work.SHA
						}
					}
					if team.ID == target.TeamID {
						expected = late.SHA
					}
					observed := false
					for _, observation := range page.Observations {
						if observation.Status == "current_ref" && observation.SHA == expected {
							observed = true
						}
					}
					if !observed {
						ready = false
					}
					if page.Current != nil || page.Status == "completed" {
						t.Fatal("current-ref read silently became a cutoff decision")
					}
				}
			}
		}
		if ready {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	if !ready {
		t.Fatal("real queued cutoff sources/current observations did not settle within normal worker/retry budget")
	}
	for index := range r.events {
		item := &r.events[index]
		census := map[string]int{}
		for _, team := range item.teams.Teams {
			page := r.freezePage(t, item, team.ID)
			source := page.Source
			if source == nil {
				t.Fatal("native cutoff census contains a missing immutable source")
			}
			census[source.NativeOutcome]++
			receipt := lifecycleCutoffReceipt{TeamID: team.ID, SourceID: source.ID, Outcome: source.NativeOutcome, SubmissionRevision: source.SubmissionRevision, Status: page.Status, Cutoff: source.Cutoff, RecordedAt: source.RecordedAt}
			for _, observation := range page.Observations {
				if observation.Status == "provider_unavailable" {
					receipt.ProviderUnavailable++
				}
				if observation.Status == "current_ref" {
					receipt.LatestObservedSHA = observation.SHA
				}
			}
			if source.NativeOutcome == "frozen" && page.CurrentReferenceStatus == "managed_reference" {
				var work lifecycleWorkReceipt
				for _, candidate := range item.work {
					if candidate.TeamID == team.ID {
						work = candidate
					}
				}
				if work.ID == uuid.Nil || !work.DomainObservedAt.Before(source.Cutoff) || page.CurrentSubmissionRevision == nil || !page.CanReconcile {
					t.Fatal("review has no exact current native source or actual retained pre-cutoff work")
				}
				command := submissionfreeze.Command{RequestID: uuid.New(), Action: "reconcile", SourceID: source.ID, NativeRevision: page.NativeRevision, SubmissionRevision: *page.CurrentSubmissionRevision, SHA: work.SHA, Reason: "Explicit synthetic organiser cutoff review: retained preparation work; current ref and its observation time do not establish history", ExternalEvidence: fmt.Sprintf("Retained synthetic external work receipt %s observed in actual preparation phase %s; independently later work is excluded", work.ID, work.DomainObservedAt.Format(time.RFC3339))}
				path := fmt.Sprintf("/api/v1/events/%s/teams/%s/submission-freeze", item.view.ID, team.ID)
				var acknowledged submissionfreeze.Receipt
				if err := r.owner.call(ctx, http.MethodPost, path, page.Revision, command, &acknowledged, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				if acknowledged.Decision == nil || acknowledged.Decision.SourceID != source.ID || acknowledged.Decision.SHA != work.SHA || acknowledged.Decision.ActorID != r.owner.person {
					t.Fatal("native organiser reconciliation changed reviewed cutoff/source/SHA")
				}
				var recovery submissionfreeze.Recovery
				if err := r.owner.call(ctx, http.MethodGet, path+"/commands/"+command.RequestID.String(), -1, nil, &recovery, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				if recovery.Receipt == nil || !reflect.DeepEqual(*recovery.Receipt, acknowledged) {
					t.Fatal("native original reconciliation receipt was replaced by a current GET projection")
				}
				current := r.freezePage(t, item, team.ID)
				if !current.BindingCurrent || current.Status != "completed" || current.Current == nil || current.Current.ID != acknowledged.Decision.ID || current.Current.SHA != work.SHA || current.Source.ID != source.ID {
					t.Fatal("native current final source did not bind the exact explicit reviewed decision")
				}
				receipt.Status = current.Status
				receipt.SelectedSHA = work.SHA
				receipt.DecisionID = &acknowledged.Decision.ID
				if team.ID == target.TeamID && (receipt.ProviderUnavailable != 1 || receipt.LatestObservedSHA != late.SHA || receipt.SelectedSHA == late.SHA) {
					t.Fatal("failed/retried capture silently substituted late head for reviewed earlier work")
				}
			}
			item.cutoff = append(item.cutoff, receipt)
		}
		if census["frozen"] != 4 || census["not_submitted"] != 2 || census["withdrawn"] != 1 || len(item.cutoff) != 7 {
			t.Fatal("native cutoff census omitted/substituted formal, private-only or disbanded entries", census)
		}
		var actual event.View
		if err := r.owner.call(ctx, http.MethodGet, "/api/v1/events/"+item.view.ID.String(), -1, nil, &actual, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if actual.Stage != "live" {
			t.Fatal("cutoff proof depended on advancing to assessment")
		}
		item.view = actual
	}
}
