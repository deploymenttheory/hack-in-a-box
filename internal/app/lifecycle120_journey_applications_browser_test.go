package app

import (
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
)

type lifecycleBrowserApplication struct {
	Actor  string                `json:"actor"`
	Form   registration.FormView `json:"form"`
	Answer string                `json:"answer"`
	State  string                `json:"state"`
}
type lifecycleBrowserSelection struct {
	Actor       string            `json:"actor"`
	Application registration.View `json:"application"`
	Reason      string            `json:"reason"`
}

type lifecycleBrowserSelectionReceipt struct {
	Actor            string `json:"actor"`
	Audience         string `json:"audience"`
	DecisionRecorded bool   `json:"decision_recorded"`
	ActiveNativeRole bool   `json:"active_native_role"`
}

type lifecycleBrowserApplicationReceipt struct {
	Actor               string `json:"actor"`
	Audience            string `json:"audience"`
	SubmissionState     string `json:"submission_state"`
	ExactFormAndAnswers bool   `json:"exact_form_and_answers"`
	OwnNativeRead       bool   `json:"own_native_read"`
}

func (r *lifecycleJourney) admissionBrowserPhase(
	t *testing.T,
	item *lifecycleJourneyEvent,
	sequence int,
	action string,
	application *lifecycleBrowserApplication,
) lifecycleBrowserAcknowledgement {
	t.Helper()
	var clock struct {
		Now time.Time `json:"now"`
	}
	if err := r.control.call(t.Context(), http.MethodGet, "/clock", nil, &clock); err != nil {
		t.Fatal(err)
	}
	return r.awaitBrowserPhase(
		t,
		lifecycleBrowserPhase{
			Sequence:    sequence,
			Action:      action,
			EventID:     item.view.ID,
			Now:         clock.Now,
			Application: application,
		},
	)
}

func (r *lifecycleJourney) browserAdmissionActions(
	t *testing.T,
	item *lifecycleJourneyEvent,
	index int,
) *lifecycleAdmissionActions {
	t.Helper()
	if os.Getenv("HACKBOX_LIFECYCLE_BROWSER_PHASES") != "ui-journey" {
		return nil
	}
	base := 200 + index*20
	return &lifecycleAdmissionActions{
		Open: func() (event.View, error) {
			before := r.readRoleEvent(t, item)
			ack := r.admissionBrowserPhase(t, item, base, "open-registration", nil)
			if !ack.Saved {
				t.Fatal("registration opening lacks its UI command")
			}
			after := r.readRoleEvent(t, item)
			if before.Stage != "draft" || after.Stage != "registration" ||
				after.Revision != before.Revision+1 ||
				after.ConfirmedAttendees != 0 ||
				!reflect.DeepEqual(after.Configuration, before.Configuration) {
				t.Fatal(
					"UI registration opening changed configuration, attendance or the wrong native stage",
				)
			}
			item.browserRegistrationOpened = true
			return after, nil
		},
		Select: func(alias string, application registration.View) (registration.View, error) {
			if alias == "mentor-a" || alias == "judge-a" {
				sequence := base + 11
				if alias == "judge-a" {
					sequence++
				}
				return r.selectFacilitatorInBrowser(t, item, sequence, lifecycleBrowserSelection{
					Actor:       alias,
					Application: application,
					Reason:      "Synthetic lifecycle role selection",
				}), nil
			}
			var selected registration.View
			err := r.owner.call(
				t.Context(),
				http.MethodPost,
				"/api/v1/events/"+item.view.ID.String()+"/registrations/"+application.ID.String()+"/decision",
				application.Revision,
				registration.ReviewInput{
					Action:      "selected",
					Reason:      "Synthetic lifecycle role selection",
					Acknowledge: true,
				},
				&selected,
				http.StatusOK,
			)
			return selected, err
		},
		Apply: func(alias string, actor *lifecycleClient, form registration.FormView) (registration.View, error) {
			slot, selected := map[string]int{"mentor-a": 0, "judge-a": 1, "attendee-35": 2, "attendee-36": 3}[alias]
			if !selected {
				return applyLifecycleForm(t.Context(), actor, item.view.ID, form)
			}
			state := "received"
			if alias == "attendee-35" {
				state = "confirmed"
			}
			if alias == "attendee-36" {
				state = "waitlisted"
			}
			return r.applyRegistrationInBrowser(
				t,
				item,
				actor,
				base+slot+1,
				lifecycleBrowserApplication{
					Actor:  alias,
					Form:   form,
					Answer: "Synthetic lifecycle motivation",
					State:  state,
				},
			), nil
		},
	}
}

func (r *lifecycleJourney) applyRegistrationInBrowser(
	t *testing.T,
	item *lifecycleJourneyEvent,
	actor *lifecycleClient,
	sequence int,
	input lifecycleBrowserApplication,
) registration.View {
	t.Helper()
	ack := r.admissionBrowserPhase(t, item, sequence, "apply-registration", &input)
	if !ack.Saved || ack.ApplicationID == nil {
		t.Fatal("registration phase lacks a real saved application")
	}
	base := "/api/v1/events/" + item.view.ID.String() + "/registrations"
	var own struct {
		Items []registration.View `json:"items"`
	}
	if err := actor.call(
		t.Context(),
		http.MethodGet,
		base+"/mine",
		-1,
		nil,
		&own,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	if len(own.Items) != 1 {
		t.Fatal("UI submission duplicated or omitted the owned native application")
	}
	current := own.Items[0]
	if current.ID != *ack.ApplicationID || current.EventID != item.view.ID ||
		current.PersonID != actor.person ||
		current.FormID != input.Form.Definition.ID ||
		input.Form.PublishedVersion == nil ||
		current.FormVersion != *input.Form.PublishedVersion ||
		current.State != input.State ||
		current.Audience != input.Form.Definition.Audience ||
		current.TeamPreference != "solo" ||
		current.InvitationReference != "" ||
		current.FallbackToAttendee {
		t.Fatal("native application differs from exact browser actor/form/state/preferences")
	}
	var detail registration.ApplicantDetail
	if err := r.owner.call(
		t.Context(),
		http.MethodGet,
		base+"/"+current.ID.String()+"/review",
		-1,
		nil,
		&detail,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	answers := forms.Answers{}
	answer, err := json.Marshal(input.Answer)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range input.Form.Definition.Fields {
		answers[field.ID.String()] = json.RawMessage(answer)
	}
	if !reflect.DeepEqual(detail.Applicant.Registration, current) || detail.Response == nil ||
		!reflect.DeepEqual(detail.Response.Definition, input.Form.Definition) ||
		!reflect.DeepEqual(detail.Response.Answers, answers) {
		t.Fatal("native owner review differs from exact published definition and submitted answers")
	}
	item.browserApplications = append(
		item.browserApplications,
		lifecycleBrowserApplicationReceipt{
			Actor:               input.Actor,
			Audience:            current.Audience,
			SubmissionState:     current.State,
			ExactFormAndAnswers: true,
			OwnNativeRead:       true,
		},
	)
	return current
}

func (r *lifecycleJourney) selectFacilitatorInBrowser(
	t *testing.T,
	item *lifecycleJourneyEvent,
	sequence int,
	input lifecycleBrowserSelection,
) registration.View {
	t.Helper()
	var clock struct {
		Now time.Time `json:"now"`
	}
	if err := r.control.call(t.Context(), http.MethodGet, "/clock", nil, &clock); err != nil {
		t.Fatal(err)
	}
	before := r.readRoleEvent(t, item)
	ack := r.awaitBrowserPhase(t, lifecycleBrowserPhase{
		Sequence:  sequence,
		Action:    "select-facilitator",
		EventID:   item.view.ID,
		Now:       clock.Now,
		Selection: &input,
	})
	if !ack.Saved || ack.ApplicationID == nil || *ack.ApplicationID != input.Application.ID {
		t.Fatal("facilitator selection lacks its exact UI command")
	}
	var detail registration.ApplicantDetail
	if err := r.owner.call(
		t.Context(),
		http.MethodGet,
		"/api/v1/events/"+item.view.ID.String()+"/registrations/"+input.Application.ID.String()+"/review",
		-1,
		nil,
		&detail,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	current := detail.Applicant.Registration
	if current.ID != input.Application.ID || current.EventID != item.view.ID || current.PersonID != input.Application.PersonID ||
		current.FormID != input.Application.FormID ||
		current.FormVersion != input.Application.FormVersion ||
		current.Audience != input.Application.Audience ||
		current.State != "selected" ||
		current.Revision != input.Application.Revision+1 {
		t.Fatal("native selection differs from captured applicant and revision")
	}
	decisions := 0
	for _, entry := range detail.History {
		if entry.Action == "reviewed" {
			decisions++
			if entry.From != "received" || entry.To != "selected" || entry.Reason != input.Reason {
				t.Fatal("recorded selection lost its reason or source state")
			}
		}
	}
	if decisions != 1 {
		t.Fatal("selection duplicated or omitted its native decision")
	}
	r.verifyRoleGrants(
		t,
		item,
		[]lifecycleBrowserRoleGrant{
			{Actor: input.Actor, PersonID: current.PersonID, Role: current.Audience},
		},
	)
	after := r.readRoleEvent(t, item)
	if after.Stage != before.Stage || after.ConfirmedAttendees != before.ConfirmedAttendees ||
		!reflect.DeepEqual(after.Configuration, before.Configuration) {
		t.Fatal("facilitator selection changed competition stage, census or configuration")
	}
	item.browserSelections = append(item.browserSelections, lifecycleBrowserSelectionReceipt{
		Actor:            input.Actor,
		Audience:         current.Audience,
		DecisionRecorded: true,
		ActiveNativeRole: true,
	})
	return current
}
