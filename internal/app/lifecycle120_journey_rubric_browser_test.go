package app

import (
	"fmt"
	"net/http"
	"os"
	"reflect"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/google/uuid"
)

type lifecycleBrowserRubricInput struct {
	Definition    rubric.Definition    `json:"definition"`
	Configuration rubric.Configuration `json:"configuration"`
	Reason        string               `json:"reason"`
}

func (r *lifecycleJourney) browserRubricConfiguration(t *testing.T, item *lifecycleJourneyEvent, index int) func(rubric.Definition) (rubric.Configuration, error) {
	t.Helper()
	if os.Getenv("HACKBOX_LIFECYCLE_BROWSER_PHASES") != "ui-journey" {
		return nil
	}
	return func(expected rubric.Definition) (rubric.Configuration, error) {
		path := "/api/v1/events/" + item.view.ID.String()
		var before, saved, approved rubric.Configuration
		var eventBefore, eventAfter event.View
		read := func(value *rubric.Configuration) error {
			return r.owner.call(t.Context(), http.MethodGet, path+"/rubric/configuration", -1, nil, value, http.StatusOK)
		}
		if err := read(&before); err != nil {
			return approved, err
		}
		if before.Revision != 0 || before.Reference.Version != 0 || before.Approved {
			return approved, fmt.Errorf("UI rubric journey requires the native unapproved defaults")
		}
		if err := r.owner.call(t.Context(), http.MethodGet, path, -1, nil, &eventBefore, http.StatusOK); err != nil {
			return approved, err
		}
		ack := r.admissionBrowserPhase(t, item, lifecycleBrowserPhase{
			Sequence: 501 + index*20, Action: "save-rubric",
			Rubric: &lifecycleBrowserRubricInput{Definition: expected, Configuration: before, Reason: "Synthetic exact common assessment contract"},
		})
		if !ack.Saved {
			return approved, fmt.Errorf("rubric draft lacks its actual UI save")
		}
		if err := read(&saved); err != nil {
			return approved, err
		}
		if saved.Revision != 1 || saved.Reference.Version != 1 || saved.Approved || saved.PublishedVersion != nil || len(saved.Definition.Prizes) != len(expected.Prizes) {
			return approved, fmt.Errorf("UI rubric save changed unexpected native versions or approval")
		}
		// New award identities are minted by the actual UI, never injected into
		// its controls. Compare every authored field after validating those IDs.
		prizeIDs := map[uuid.UUID]bool{}
		for i, prize := range saved.Definition.Prizes {
			if prize.ID == uuid.Nil || prizeIDs[prize.ID] {
				return approved, fmt.Errorf("UI rubric awards lack unique native identities")
			}
			prizeIDs[prize.ID] = true
			expected.Prizes[i].ID = prize.ID
		}
		if !reflect.DeepEqual(saved.Definition, expected) || saved.Reference.Digest != expected.PublicDigest() || saved.Reference.ID == uuid.Nil {
			return approved, fmt.Errorf("UI rubric definition differs from the exact independently read native snapshot")
		}
		ack = r.admissionBrowserPhase(t, item, lifecycleBrowserPhase{
			Sequence: 502 + index*20, Action: "approve-rubric",
			Rubric: &lifecycleBrowserRubricInput{Definition: expected, Configuration: saved, Reason: "Explicit exact common rubric approval"},
		})
		if !ack.Saved {
			return approved, fmt.Errorf("rubric approval lacks its actual UI decision")
		}
		if err := read(&approved); err != nil {
			return approved, err
		}
		if !approved.Approved || approved.Revision != saved.Revision+1 || approved.Reference != saved.Reference || !reflect.DeepEqual(approved.Definition, saved.Definition) || approved.PublishedVersion != nil || len(approved.Runs) != 0 {
			return approved, fmt.Errorf("UI approval altered criteria, released them or started assessment")
		}
		if err := r.owner.call(t.Context(), http.MethodGet, path, -1, nil, &eventAfter, http.StatusOK); err != nil {
			return approved, err
		}
		if eventAfter.Stage != eventBefore.Stage || eventAfter.ConfirmedAttendees != eventBefore.ConfirmedAttendees || !reflect.DeepEqual(eventAfter.Configuration, eventBefore.Configuration) {
			return approved, fmt.Errorf("UI rubric authoring altered the native event or admission contract")
		}
		item.browserRubricApproved = true
		return approved, nil
	}
}
