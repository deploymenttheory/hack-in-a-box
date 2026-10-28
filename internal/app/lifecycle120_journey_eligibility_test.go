package app

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
	"github.com/google/uuid"
)

type lifecycleEligibilityReceipt struct {
	PersonID            uuid.UUID `json:"person_id"`
	Role                string    `json:"role"`
	Revoked             bool      `json:"grant_revoked"`
	ActiveDenied        bool      `json:"active_grant_competition_denied"`
	LastingDenied       bool      `json:"revoked_grant_competition_denied"`
	ForeignEventAllowed bool      `json:"other_event_history_did_not_exclude"`
}

func (r *lifecycleJourney) proveLastingEligibility(t *testing.T, item *lifecycleJourneyEvent, form registration.FormView, otherEventHistory bool) {
	t.Helper()
	base := "/api/v1/events/" + item.view.ID.String()
	for _, subject := range []struct{ actor, role string }{{"former-organiser", "organiser"}, {"early-access", "early_brief_access"}} {
		client, err := signInLifecycle(t.Context(), "http://public.localhost:8080", "http://provider.localhost:8080", subject.actor)
		if err != nil {
			t.Fatal(err)
		}
		receipt := lifecycleEligibilityReceipt{PersonID: client.person, Role: subject.role}
		// Event zero has already retained this person's exclusion. Before any
		// local grant, event one's normal admission must remain possible.
		if otherEventHistory {
			admission, err := applyLifecycleForm(t.Context(), client, item.view.ID, form)
			if err != nil {
				t.Fatal(err)
			}
			if admission.State != "confirmed" {
				t.Fatal("foreign-event eligibility history blocked a current local admission")
			}
			var withdrawn registration.View
			if err := client.call(t.Context(), http.MethodPost, base+"/registrations/"+admission.ID.String()+"/respond", admission.Revision, map[string]string{"action": "withdraw"}, &withdrawn, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if withdrawn.State != "withdrawn" {
				t.Fatal("explicit local withdrawal was not acknowledged")
			}
			receipt.ForeignEventAllowed = true
		}
		readEvent := func() event.View {
			t.Helper()
			var current event.View
			if err := r.owner.call(t.Context(), http.MethodGet, base, -1, nil, &current, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			return current
		}
		grant := func(active bool) {
			t.Helper()
			current := readEvent()
			body := map[string]any{"person_id": client.person, "role": subject.role, "active": active, "acknowledge_competition_restriction": active}
			var ack struct {
				Revision int64 `json:"revision"`
			}
			if err := r.owner.call(t.Context(), http.MethodPut, base+"/staff", current.Revision, body, &ack, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if ack.Revision <= current.Revision {
				t.Fatal("native staff change lacks its current event acknowledgement")
			}
		}
		deny := func() {
			t.Helper()
			before := readEvent()
			answers := forms.Answers{}
			for _, field := range form.Definition.Fields {
				answers[field.ID.String()] = json.RawMessage(`"Synthetic eligibility attempt"`)
			}
			input := registration.Application{ID: uuid.New(), FormID: form.Definition.ID, FormVersion: *form.PublishedVersion, Answers: answers, TeamPreference: "solo"}
			var rejected struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := client.call(t.Context(), http.MethodPost, base+"/registrations", -1, input, &rejected, http.StatusConflict); err != nil {
				t.Fatal(err)
			}
			if rejected.Error.Code != "competition_ineligible" {
				t.Fatal("registration rejection did not establish lasting eligibility exclusion")
			}
			after := readEvent()
			if after.Revision != before.Revision || after.ConfirmedAttendees != before.ConfirmedAttendees {
				t.Fatal("denied ineligible registration mutated the native event")
			}
		}
		grant(true)
		deny()
		receipt.ActiveDenied = true
		grant(false)
		deny()
		receipt.LastingDenied = true
		var staff struct {
			Items []struct {
				PersonID uuid.UUID `json:"person_id"`
				Role     string    `json:"role"`
				Active   bool      `json:"active"`
			} `json:"items"`
		}
		if err := r.owner.call(t.Context(), http.MethodGet, base+"/staff", -1, nil, &staff, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		for _, member := range staff.Items {
			if member.PersonID == client.person && member.Role == subject.role && !member.Active {
				receipt.Revoked = true
			}
		}
		if !receipt.Revoked {
			t.Fatal("lasting exclusion was not verified after actual native grant revocation")
		}
		item.eligibility = append(item.eligibility, receipt)
	}
}
