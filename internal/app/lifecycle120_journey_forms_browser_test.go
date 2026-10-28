package app

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
)

type lifecycleBrowserFormInput struct {
	Name     string `json:"name"`
	Audience string `json:"audience"`
	Label    string `json:"label"`
}

func (r *lifecycleJourney) browserRegistrationForms(
	t *testing.T,
	item *lifecycleJourneyEvent,
) []registration.FormView {
	t.Helper()
	var page struct {
		Items []registration.FormView `json:"items"`
	}
	if err := r.owner.call(
		t.Context(),
		http.MethodGet,
		"/api/v1/events/"+item.view.ID.String()+"/registration/forms",
		-1,
		nil,
		&page,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	return page.Items
}

func (r *lifecycleJourney) publishRegistrationFormInBrowser(
	t *testing.T,
	item *lifecycleJourneyEvent,
	sequence int,
	input lifecycleBrowserFormInput,
) registration.FormView {
	t.Helper()
	before := r.browserRegistrationForms(t, item)
	eventBefore := r.readRoleEvent(t, item)
	ack := r.admissionBrowserPhase(
		t,
		item,
		lifecycleBrowserPhase{
			Sequence: sequence,
			Action:   "publish-registration-form",
			Form:     &input,
		},
	)
	if !ack.Saved || ack.FormID == nil {
		t.Fatal("form publication lacks its exact UI command")
	}
	after := r.browserRegistrationForms(t, item)
	if len(after) != len(before)+1 {
		t.Fatal("UI form publication added the wrong number of native forms")
	}
	for _, original := range before {
		found := false
		for _, current := range after {
			if current.Definition.ID == original.Definition.ID {
				if !reflect.DeepEqual(current, original) {
					t.Fatal("new UI form changed a previous form or its publication")
				}
				found = true
			}
		}
		if !found || original.Definition.ID == *ack.FormID {
			t.Fatal("UI form creation replaced a previous form")
		}
	}
	var published registration.FormView
	for _, current := range after {
		if current.Definition.ID == *ack.FormID {
			published = current
		}
	}
	if published.Definition.ID != *ack.FormID || published.Definition.Name != input.Name || published.Definition.Audience != input.Audience ||
		len(published.Definition.Fields) != 1 || published.Version != 1 ||
		published.Revision != 2 ||
		published.PublishedVersion == nil ||
		*published.PublishedVersion != 1 {
		t.Fatal("UI publication lost the exact native form identity, definition or version")
	}
	field := published.Definition.Fields[0]
	if field.Label != input.Label || field.Type != "text" || !field.Required || field.Help != "" ||
		len(field.Choices) != 0 ||
		field.VisibleWhen != nil {
		t.Fatal("UI publication differs from the reviewed required question")
	}
	eventAfter := r.readRoleEvent(t, item)
	if eventAfter.Stage != eventBefore.Stage ||
		eventAfter.ConfirmedAttendees != eventBefore.ConfirmedAttendees ||
		!reflect.DeepEqual(eventAfter.Configuration, eventBefore.Configuration) {
		t.Fatal("form publication changed competition stage, configuration or census")
	}
	item.browserPublishedForms = append(item.browserPublishedForms, input.Audience)
	return published
}
