package app

import (
	"net/http"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

type lifecycleBrowserEventDecision struct {
	Before        event.View           `json:"before"`
	Configuration *event.Configuration `json:"configuration,omitempty"`
	Reason        string               `json:"reason"`
}

func (r *lifecycleJourney) eventDecisionInBrowser(t *testing.T, item *lifecycleJourneyEvent, index int, action string, before event.View, configuration *event.Configuration, reason string) event.View {
	t.Helper()
	sequence := 901 + index*20
	if action == "approve-attendance-live" {
		sequence++
	} else if action != "postpone-event" {
		t.Fatal("unsupported UI event decision")
	}
	ack := r.admissionBrowserPhase(t, item, lifecycleBrowserPhase{Sequence: sequence, Action: action, EventDecision: &lifecycleBrowserEventDecision{Before: before, Configuration: configuration, Reason: reason}})
	if !ack.Saved {
		t.Fatal("event decision lacks a real UI command acknowledgement")
	}
	var current event.View
	if err := r.owner.call(t.Context(), http.MethodGet, "/api/v1/events/"+before.ID.String(), -1, nil, &current, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if current.ID != before.ID || current.Revision != before.Revision+1 {
		t.Fatal("UI event decision changed the wrong native event or revision")
	}
	return current
}
