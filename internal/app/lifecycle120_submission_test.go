package app

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
)

type lifecycleSubmissionReceipt struct {
	TeamID         uuid.UUID  `json:"team_id"`
	DeliverableID  uuid.UUID  `json:"deliverable_id"`
	Kind           string     `json:"kind"`
	DraftRevision  int64      `json:"draft_revision"`
	DraftAt        time.Time  `json:"draft_at"`
	FormalRevision int64      `json:"formal_revision"`
	SubmittedAt    *time.Time `json:"submitted_at"`
	evidence       []submissions.Evidence
}

// Definition authoring precedes actual common release, retaining all other
// snapshot owners. Native publication then makes these requirements immutable.
func (r *lifecycleJourney) configureDeliverables(t *testing.T, item *lifecycleJourneyEvent) {
	t.Helper()
	ctx := t.Context()
	base := "/api/v1/events/" + item.view.ID.String()
	var snapshot templates.Bundle
	if err := r.owner.call(ctx, http.MethodGet, base+"/configuration-snapshot", -1, nil, &snapshot, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	item.deliverable = uuid.New()
	snapshot.Definitions.Deliverables = []templates.Deliverable{{ID: item.deliverable, Title: "Reviewed competition output", Description: "Submit either the managed repository or a non-repository demonstration reference during the live window.", Required: true, EvidenceKinds: []string{"repository", "url"}, Audience: []string{"organiser", "mentor", "judge"}}}
	var next templates.Bundle
	if err := r.owner.call(ctx, http.MethodPut, base+"/configuration-snapshot", item.view.Revision, snapshot.Definitions, &next, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(next.Definitions.Deliverables, snapshot.Definitions.Deliverables) {
		t.Fatal("native deliverable definition changed while saving")
	}
	if err := r.owner.call(ctx, http.MethodGet, base, -1, nil, &item.view, http.StatusOK); err != nil {
		t.Fatal(err)
	}
}

func (r *lifecycleJourney) preparePrivateDrafts(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	for index := range r.events {
		item := &r.events[index]
		for _, team := range item.teams.Teams {
			if team.State == "disbanded" {
				continue
			}
			member := r.participant(t, team.Published[0].ID)
			kind := "repository"
			reference := ""
			for _, repo := range item.repositories {
				if repo.TeamID == team.ID {
					reference = "https://github.com/" + repo.Organization + "/" + repo.Name
				}
			}
			if reference == "" {
				t.Fatal("private draft has no actual provisioned repository")
			}
			// One genuine non-repository entry per event; other teams retain the managed
			// repository reference. A URL is evidence metadata, never a SHA receipt.
			if len(item.submissions) == 1 {
				kind = "url"
				reference = "https://example.test/demonstrations/" + team.ID.String()
			}
			evidence := []submissions.Evidence{{DeliverableID: item.deliverable, Kind: kind, Label: "Synthetic team working draft", URL: reference}}
			path := fmt.Sprintf("/api/v1/events/%s/teams/%s/submission", item.view.ID, team.ID)
			var before submissions.Page
			if err := member.call(ctx, http.MethodGet, path, -1, nil, &before, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if !before.CanDraft || before.CanSubmit || before.CanReview || before.Final != nil || len(before.Definitions) != 1 || before.Definitions[0].ID != item.deliverable {
				t.Fatal("preweek submission capabilities or published requirements changed")
			}
			var draft submissions.Draft
			if err := member.call(ctx, http.MethodPut, path+"/draft", 0, map[string]any{"evidence": evidence}, &draft, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if draft.Revision != 1 || draft.UpdatedAt == nil || !draft.UpdatedAt.Equal(r.timeline.Publication) || !reflect.DeepEqual(draft.Evidence, evidence) {
				t.Fatal("actual preweek draft did not retain its exact evidence/time")
			}
			var rejected struct {
				Error struct {
					Blockers []event.Issue `json:"blockers"`
				} `json:"error"`
			}
			if err := member.call(ctx, http.MethodPut, path, before.Revision, submissions.Command{Evidence: evidence}, &rejected, http.StatusConflict); err != nil {
				t.Fatal(err)
			}
			if len(rejected.Error.Blockers) != 1 || rejected.Error.Blockers[0].Field != "live_start" || rejected.Error.Blockers[0].Code != "not_open" {
				t.Fatal("formal preweek submission did not fail at the native live gate")
			}
			rejected.Error.Blockers = nil
			if err := r.owner.call(ctx, http.MethodPost, path+"/review", before.Revision, map[string]any{"submission_revision": 1, "status": "accepted", "reason": "Prove formal review remains closed during preparation"}, &rejected, http.StatusConflict); err != nil {
				t.Fatal(err)
			}
			if len(rejected.Error.Blockers) != 1 || rejected.Error.Blockers[0].Field != "live_start" || rejected.Error.Blockers[0].Code != "review_not_open" {
				t.Fatal("formal preweek review did not fail at its native live gate")
			}
			var unchanged submissions.Page
			if err := member.call(ctx, http.MethodGet, path, -1, nil, &unchanged, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, unchanged) {
				t.Fatal("denied early submission/review mutated the native competition projection")
			}
			var retained submissions.Draft
			if err := member.call(ctx, http.MethodGet, path+"/draft", -1, nil, &retained, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(retained, draft) {
				t.Fatal("denied early formal commands mutated the private working draft")
			}
			// Other frozen teams and the organiser cannot read the participant's draft.
			if err := r.owner.call(ctx, http.MethodGet, path+"/draft", -1, nil, nil, http.StatusForbidden); err != nil {
				t.Fatal(err)
			}
			for _, other := range item.teams.Teams {
				if other.ID != team.ID && other.State != "disbanded" {
					stranger := r.participant(t, other.Published[0].ID)
					if err := stranger.call(ctx, http.MethodGet, path+"/draft", -1, nil, nil, http.StatusForbidden); err != nil {
						t.Fatal(err)
					}
					break
				}
			}
			item.submissions = append(item.submissions, lifecycleSubmissionReceipt{TeamID: team.ID, DeliverableID: item.deliverable, Kind: kind, DraftRevision: draft.Revision, DraftAt: *draft.UpdatedAt, evidence: evidence})
		}
		if len(item.submissions) != 6 {
			t.Fatal("private draft census did not cover every frozen entry")
		}
	}
}

func (r *lifecycleJourney) submitLiveEvidence(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	// Earlier live support commands may advance domain time monotonically. Bind
	// the formal evidence to the actual current phase, not the older poll stamp.
	var phase struct {
		Now time.Time `json:"now"`
	}
	if err := r.control.call(ctx, http.MethodGet, "/clock", nil, &phase); err != nil {
		t.Fatal(err)
	}
	submittedAt := phase.Now
	if submittedAt.IsZero() || (!r.livePollAt.IsZero() && submittedAt.Before(r.livePollAt)) {
		t.Fatal("formal submission phase has no current monotonic domain instant")
	}
	for index := range r.events {
		item := &r.events[index]
		if submittedAt.Before(item.view.Configuration.LiveStart) || !submittedAt.Before(item.view.Configuration.SubmissionCutoff) {
			t.Fatal("formal submission phase is outside the actual native live window")
		}
		for i := range item.submissions {
			receipt := &item.submissions[i]
			// Retain two genuine private-only drafts for later absent/not-submitted
			// census cases; do not manufacture formal evidence from these drafts.
			if i >= 4 {
				continue
			}
			var member *lifecycleClient
			for _, team := range item.teams.Teams {
				if team.ID == receipt.TeamID {
					member = r.participant(t, team.Published[0].ID)
				}
			}
			if member == nil {
				t.Fatal("formal submission lost its frozen team member")
			}
			path := fmt.Sprintf("/api/v1/events/%s/teams/%s/submission", item.view.ID, receipt.TeamID)
			var before, next submissions.Page
			if err := member.call(ctx, http.MethodGet, path, -1, nil, &before, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if !before.CanSubmit || before.Final != nil {
				t.Fatal("actual live submission gate did not open cleanly")
			}
			if err := member.call(ctx, http.MethodPut, path, before.Revision, submissions.Command{Evidence: receipt.evidence}, &next, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if next.Final == nil || next.Final.Revision != 1 || next.Final.Late || next.Final.ActorID != member.person || !next.Final.SubmittedAt.Equal(submittedAt) || !reflect.DeepEqual(next.Final.Evidence, receipt.evidence) {
				t.Fatal("native live submission changed the reviewed source or identity")
			}
			receipt.FormalRevision = next.Final.Revision
			at := next.Final.SubmittedAt
			receipt.SubmittedAt = &at
		}
	}
}
