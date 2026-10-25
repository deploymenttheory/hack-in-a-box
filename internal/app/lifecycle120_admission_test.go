package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
)

type lifecycleAdmissions struct {
	clients       []*lifecycleClient
	registrations []registration.View
	attendeeForm  registration.FormView
}

// Every mutation here travels through the authenticated native HTTP owner.
// The final integrated scenario will reuse this phase, including its negative
// gates; independently running this checkpoint does not complete #120.
func admitLifecycleCohort(ctx context.Context, owner *lifecycleClient, current event.View) (lifecycleAdmissions, error) {
	var cohort lifecycleAdmissions
	base := "/api/v1/events/" + current.ID.String()
	form, err := publishLifecycleForm(ctx, owner, current.ID, "attendee")
	if err != nil {
		return cohort, err
	}
	cohort.attendeeForm = form
	if current, err = lifecycleTransition(ctx, owner, current.ID, "registration", http.StatusOK); err != nil {
		return cohort, err
	}
	var offerPolicy registration.OfferSettings
	if err := owner.call(ctx, http.MethodPut, base+"/registrations/offer-policy", 0,
		map[string]any{"automated": true, "working_days": 2}, &offerPolicy, http.StatusOK); err != nil {
		return cohort, err
	}
	if !offerPolicy.Automated || offerPolicy.WorkingDays != 2 {
		return cohort, fmt.Errorf("native FIFO automation was not explicitly approved")
	}
	// Selected facilitators use the application/review workflow and cannot
	// inflate the count that determines the competition go/no-go decision.
	for _, role := range []string{"mentor", "judge"} {
		staffForm, err := publishLifecycleForm(ctx, owner, current.ID, role)
		if err != nil {
			return cohort, err
		}
		staff, err := signInLifecycle(ctx, "http://public:8080", "http://provider:8080", role+"-a")
		if err != nil {
			return cohort, err
		}
		application, err := applyLifecycleForm(ctx, staff, current.ID, staffForm)
		if err != nil {
			return cohort, err
		}
		if application.Kind != "facilitator" || application.State != "received" {
			return cohort, fmt.Errorf("native facilitator application was not received")
		}
		var selected registration.View
		if err := owner.call(ctx, http.MethodPost, base+"/registrations/"+application.ID.String()+"/decision", application.Revision,
			registration.ReviewInput{Action: "selected", Reason: "Synthetic lifecycle role selection", Acknowledge: true}, &selected, http.StatusOK); err != nil {
			return cohort, err
		}
		if selected.State != "selected" || selected.PersonID != staff.person {
			return cohort, fmt.Errorf("native facilitator selection did not commit its role")
		}
	}
	for i := 1; i <= 36; i++ {
		client, err := signInLifecycle(ctx, "http://public:8080", "http://provider:8080", fmt.Sprintf("attendee-%02d", i))
		if err != nil {
			return cohort, err
		}
		applied, err := applyLifecycleForm(ctx, client, current.ID, form)
		if err != nil {
			return cohort, err
		}
		want := "confirmed"
		if i > current.Configuration.Capacity {
			want = "waitlisted"
		}
		if applied.PersonID != client.person || applied.State != want || applied.Kind != "attendee" {
			return cohort, fmt.Errorf("native admission %d did not enforce confirmed capacity", i)
		}
		cohort.clients = append(cohort.clients, client)
		cohort.registrations = append(cohort.registrations, applied)
		if i == 29 {
			if _, err := lifecycleTransition(ctx, owner, current.ID, "preparation", http.StatusConflict); err != nil {
				return cohort, err
			}
		}
		if i == 30 {
			if err := owner.call(ctx, http.MethodGet, base, -1, nil, &current, http.StatusOK); err != nil {
				return cohort, err
			}
			if current.Stage != "registration" || current.ConfirmedAttendees != 30 {
				return cohort, fmt.Errorf("staff counted as competitors or threshold automatically advanced the event")
			}
		}
	}
	current, err = lifecycleTransition(ctx, owner, current.ID, "preparation", http.StatusOK)
	if err != nil {
		return cohort, err
	}
	if current.ConfirmedAttendees != 35 {
		return cohort, fmt.Errorf("explicit competition go decision changed the confirmed count")
	}
	// A withdrawal after go must preserve the stage and cause a real FIFO
	// offer. Accept through its owner before admission closes, during preparation.
	departed := cohort.registrations[34]
	var withdrawn registration.View
	if err := cohort.clients[34].call(ctx, http.MethodPost, base+"/registrations/"+departed.ID.String()+"/respond", departed.Revision,
		map[string]string{"action": "withdraw"}, &withdrawn, http.StatusOK); err != nil {
		return cohort, err
	}
	cohort.registrations[34] = withdrawn
	var own struct {
		Items []registration.View `json:"items"`
	}
	if err := cohort.clients[35].call(ctx, http.MethodGet, base+"/registrations/mine", -1, nil, &own, http.StatusOK); err != nil {
		return cohort, err
	}
	if len(own.Items) != 1 || own.Items[0].State != "offered" || own.Items[0].Offer == nil || own.Items[0].WaitlistPosition != nil {
		return cohort, fmt.Errorf("native preparation-phase FIFO offer was not reserved")
	}
	if err := owner.call(ctx, http.MethodGet, base, -1, nil, &current, http.StatusOK); err != nil {
		return cohort, err
	}
	if current.Stage != "preparation" || current.ConfirmedAttendees != 34 {
		return cohort, fmt.Errorf("an outstanding offer counted as confirmed or withdrawal regressed the stage")
	}
	var accepted registration.View
	if err := cohort.clients[35].call(ctx, http.MethodPost, base+"/registrations/"+own.Items[0].ID.String()+"/respond", own.Items[0].Revision,
		map[string]string{"action": "accept"}, &accepted, http.StatusOK); err != nil {
		return cohort, err
	}
	cohort.registrations[35] = accepted
	if err := owner.call(ctx, http.MethodGet, base, -1, nil, &current, http.StatusOK); err != nil {
		return cohort, err
	}
	if withdrawn.State != "withdrawn" || accepted.State != "confirmed" || current.Stage != "preparation" || current.ConfirmedAttendees != 35 {
		return cohort, fmt.Errorf("withdrawal/FIFO acceptance changed the go decision or competition census")
	}
	return cohort, nil
}

func publishLifecycleForm(ctx context.Context, owner *lifecycleClient, eventID uuid.UUID, audience string) (registration.FormView, error) {
	fieldID := uuid.New()
	definition := forms.Definition{ID: uuid.New(), Name: "Synthetic " + audience + " application", Audience: audience, Fields: []forms.Field{
		{ID: fieldID, Label: "Synthetic motivation", Type: "text", Required: true, Choices: []string{}},
	}}
	path := "/api/v1/events/" + eventID.String() + "/registration/forms/" + definition.ID.String()
	var saved registration.FormView
	if err := owner.call(ctx, http.MethodPut, path, 0, definition, &saved, http.StatusOK); err != nil {
		return saved, err
	}
	var published registration.FormView
	err := owner.call(ctx, http.MethodPost, path+"/publish", saved.Revision,
		map[string]any{"version": saved.Version, "acknowledge_response_versions": true}, &published, http.StatusOK)
	if err == nil && (published.PublishedVersion == nil || *published.PublishedVersion != saved.Version) {
		err = fmt.Errorf("native form publication did not preserve its exact version")
	}
	return published, err
}

func applyLifecycleForm(ctx context.Context, actor *lifecycleClient, eventID uuid.UUID, form registration.FormView) (registration.View, error) {
	answers := forms.Answers{}
	for _, field := range form.Definition.Fields {
		answers[field.ID.String()] = json.RawMessage(`"Synthetic lifecycle motivation"`)
	}
	var applied registration.View
	err := actor.call(ctx, http.MethodPost, "/api/v1/events/"+eventID.String()+"/registrations", -1,
		registration.Application{ID: uuid.New(), FormID: form.Definition.ID, FormVersion: *form.PublishedVersion,
			Answers: answers, TeamPreference: "solo"}, &applied, http.StatusOK)
	return applied, err
}

func lifecycleTransition(ctx context.Context, owner *lifecycleClient, eventID uuid.UUID, stage string, want int) (event.View, error) {
	base := "/api/v1/events/" + eventID.String()
	var current event.View
	if err := owner.call(ctx, http.MethodGet, base, -1, nil, &current, http.StatusOK); err != nil {
		return current, err
	}
	var next event.View
	if want == http.StatusConflict && stage == "preparation" {
		var rejected struct {
			Error struct {
				Code     string        `json:"code"`
				Blockers []event.Issue `json:"blockers"`
			} `json:"error"`
		}
		if err := owner.call(ctx, http.MethodPost, base+"/transition", current.Revision,
			map[string]any{"stage": stage, "reason": "Synthetic lifecycle decision", "attendance_exception": false}, &rejected, want); err != nil {
			return next, err
		}
		minimumBlocked := false
		for _, blocker := range rejected.Error.Blockers {
			minimumBlocked = minimumBlocked || blocker.Code == "attendance_shortfall"
		}
		if rejected.Error.Code != "event_blocked" || !minimumBlocked {
			return next, fmt.Errorf("preparation was not blocked by the actual confirmed-attendance minimum")
		}
		if err := owner.call(ctx, http.MethodGet, base, -1, nil, &next, http.StatusOK); err != nil {
			return next, err
		}
		if next.Stage != current.Stage || next.Revision != current.Revision {
			return next, fmt.Errorf("blocked preparation changed native event state")
		}
		return next, nil
	}
	err := owner.call(ctx, http.MethodPost, base+"/transition", current.Revision,
		map[string]any{"stage": stage, "reason": "Synthetic lifecycle decision", "attendance_exception": false}, &next, want)
	return next, err
}

func TestLifecycleComposeAdmission(t *testing.T) {
	if os.Getenv("HACKBOX_LIFECYCLE_ISOLATED") != "synthetic-only" {
		t.Skip("run only in the isolated lifecycle Compose project")
	}
	owner, err := signInLifecycle(t.Context(), "http://admin:8080", "http://provider:8080", "owner")
	if err != nil {
		t.Fatal(err)
	}
	timeline, err := newLifecycleTimeline(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	var current event.View
	if err := owner.call(t.Context(), http.MethodPost, "/api/v1/events", -1, lifecycleEventInput("Admission", timeline), &current, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	cohort, err := admitLifecycleCohort(t.Context(), owner, current)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := allocateLifecycleTeams(t.Context(), owner, current.ID, &cohort); err != nil {
		t.Fatal(err)
	}
	t.Log("partial native HTTP admission/allocation checkpoint; L01-L18 integrated lifecycle remains required")
}
