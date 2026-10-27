package app

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
)

type lifecycleBrowserAdmissionResponse struct {
	Actor       string            `json:"actor"`
	Application registration.View `json:"application"`
	Action      string            `json:"action"`
}

func (r *lifecycleJourney) respondToAdmissionInBrowser(
	t *testing.T,
	item *lifecycleJourneyEvent,
	actor *lifecycleClient,
	sequence int,
	input lifecycleBrowserAdmissionResponse,
) registration.View {
	t.Helper()
	before := r.readRoleEvent(t, item)
	ack := r.admissionBrowserPhase(
		t,
		item,
		lifecycleBrowserPhase{Sequence: sequence, Action: "respond-registration", Response: &input},
	)
	if !ack.Saved || ack.ApplicationID == nil || *ack.ApplicationID != input.Application.ID {
		t.Fatal("admission response lacks its exact UI command")
	}
	var own struct {
		Items []registration.View `json:"items"`
	}
	if err := actor.call(
		t.Context(),
		http.MethodGet,
		"/api/v1/events/"+item.view.ID.String()+"/registrations/mine",
		-1,
		nil,
		&own,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	if len(own.Items) != 1 {
		t.Fatal("UI admission response duplicated or omitted the owned registration")
	}
	current := own.Items[0]
	expectedState := "withdrawn"
	expectedCount := before.ConfirmedAttendees - 1
	if input.Action == "accept" {
		expectedState = "confirmed"
		expectedCount = before.ConfirmedAttendees + 1
		if current.Offer == nil || input.Application.Offer == nil ||
			current.Offer.ID != input.Application.Offer.ID ||
			current.Offer.State != "accepted" {
			t.Fatal("UI acceptance lost its exact native FIFO offer")
		}
	} else if input.Action != "withdraw" {
		t.Fatal("unexpected browser admission response")
	}
	if current.ID != input.Application.ID || current.EventID != item.view.ID || current.PersonID != actor.person ||
		current.FormID != input.Application.FormID ||
		current.FormVersion != input.Application.FormVersion ||
		current.State != expectedState ||
		current.Revision != input.Application.Revision+1 {
		t.Fatal(
			"UI admission response differs from the captured native registration/revision/state",
		)
	}
	after := r.readRoleEvent(t, item)
	if before.Stage != "preparation" || after.Stage != before.Stage ||
		after.ConfirmedAttendees != expectedCount ||
		!reflect.DeepEqual(after.Configuration, before.Configuration) {
		t.Fatal(
			"UI withdrawal or acceptance changed the go decision, configuration or wrong confirmed census",
		)
	}
	item.browserAdmissionDecisions = append(item.browserAdmissionDecisions, input.Action)
	return current
}
