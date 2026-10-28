package app

import (
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
	"github.com/deploymenttheory/hack-in-a-box/internal/teams"
)

type lifecycleOfferCutoffReceipt struct {
	RegistrationID          uuid.UUID `json:"registration_id"`
	PersonID                uuid.UUID `json:"person_id"`
	OfferID                 uuid.UUID `json:"original_offer_id"`
	ExpiresAt               time.Time `json:"expires_at"`
	ObservedBefore          time.Time `json:"observed_one_second_before_cutoff"`
	WorkingDays             int32     `json:"working_days"`
	ConfirmedBefore         int64     `json:"confirmed_before"`
	ConfirmedAfter          int64     `json:"confirmed_after"`
	LateResponseState       string    `json:"late_response_state"`
	LateOfferState          string    `json:"late_offer_state"`
	LateResponseRevision    int64     `json:"late_response_revision"`
	LateHTTPStatus          int       `json:"late_http_status"`
	PublishedTeamsUnchanged bool      `json:"published_teams_unchanged"`
	FrozenAfterCutoff       bool      `json:"frozen_after_real_admission_cutoff"`
}

// A real additional FIFO reserve gets an offer during pre-freeze remediation.
// Keep the existing two-working-day policy and genuine shared admission cutoff;
// at that boundary the API must return its actual expired outcome, never turn
// an old offered projection into a confirmed competitor or a frozen team move.
func (r *lifecycleJourney) expireSurvivingOffers(t *testing.T) {
	t.Helper()
	cutoff := r.events[0].view.Configuration.AdmissionCutoff
	if !cutoff.Equal(r.events[1].view.Configuration.AdmissionCutoff) {
		t.Fatal("concurrent offer cutoff fixture has different actual admission cutoffs")
	}
	if err := r.control.advance(t.Context(), cutoff.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	before := []registration.View{}
	for index := range r.events {
		item := &r.events[index]
		base := "/api/v1/events/" + item.view.ID.String()
		if item.cohort.reserveID == uuid.Nil || item.cohort.reservePerson == uuid.Nil || len(item.cohort.clients) != 37 {
			t.Fatal("surviving offer fixture lacks its separately applied native FIFO reserve")
		}
		var policy registration.OfferSettings
		if err := r.owner.call(t.Context(), http.MethodGet, base+"/registrations/offer-policy", -1, nil, &policy, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if !policy.Automated || policy.WorkingDays != 2 {
			t.Fatal("surviving offer changed the original approved two-working-day policy")
		}
		client := r.participant(t, item.cohort.reservePerson)
		var own struct {
			Items []registration.View `json:"items"`
		}
		if err := client.call(t.Context(), http.MethodGet, base+"/registrations/mine", -1, nil, &own, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if len(own.Items) != 1 {
			t.Fatal("reserve does not have exactly one native event application")
		}
		applied := own.Items[0]
		if applied.ID != item.cohort.reserveID || applied.PersonID != client.person || applied.State != "offered" || applied.Offer == nil || applied.Offer.State != "pending" || !applied.Offer.ExpiresAt.Equal(cutoff) {
			t.Fatal("original native reserve offer did not survive preparation until the admission cutoff")
		}
		var current event.View
		if err := r.owner.call(t.Context(), http.MethodGet, base, -1, nil, &current, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if current.Stage != "preparation" || current.ConfirmedAttendees != 32 {
			t.Fatal("surviving unaccepted offer counted toward the confirmed floor or changed the go decision")
		}
		item.offerCutoff = lifecycleOfferCutoffReceipt{RegistrationID: applied.ID, PersonID: client.person, OfferID: applied.Offer.ID, ExpiresAt: applied.Offer.ExpiresAt, ObservedBefore: cutoff.Add(-time.Second), WorkingDays: policy.WorkingDays, ConfirmedBefore: current.ConfirmedAttendees}
		before = append(before, applied)
	}
	if err := r.control.advance(t.Context(), cutoff); err != nil {
		t.Fatal(err)
	}
	for index := range r.events {
		item := &r.events[index]
		base := "/api/v1/events/" + item.view.ID.String()
		client := r.participant(t, item.cohort.reservePerson)
		var result registration.View
		// The response contract returns the reconciled current registration when
		// an offer has already expired. HTTP200 is not an acceptance receipt.
		if err := client.call(t.Context(), http.MethodPost, base+"/registrations/"+before[index].ID.String()+"/respond", before[index].Revision, map[string]string{"action": "accept"}, &result, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if result.ID != before[index].ID || result.PersonID != client.person || result.State != "declined" || result.Offer == nil || result.Offer.ID != before[index].Offer.ID || result.Offer.State != "expired" || result.Revision <= before[index].Revision {
			t.Fatal("late offer command bypassed the actual cutoff or lost its original expired identity")
		}
		for i := range item.cohort.registrations {
			if item.cohort.registrations[i].ID == result.ID {
				item.cohort.registrations[i] = result
			}
		}
		var current event.View
		if err := r.owner.call(t.Context(), http.MethodGet, base, -1, nil, &current, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if current.Stage != "preparation" || current.ConfirmedAttendees != item.offerCutoff.ConfirmedBefore {
			t.Fatal("late expired offer changed confirmed attendance or cancelled the invested event")
		}
		var roster teams.Page
		if err := r.owner.call(t.Context(), http.MethodGet, base+"/teams", -1, nil, &roster, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if roster.Frozen || !reflect.DeepEqual(roster.Teams, item.teams.Teams) {
			t.Fatal("expired offer changed the published roster or expired through an early freeze")
		}
		item.offerCutoff.ConfirmedAfter = current.ConfirmedAttendees
		item.offerCutoff.LateResponseState, item.offerCutoff.LateOfferState = result.State, result.Offer.State
		item.offerCutoff.LateResponseRevision, item.offerCutoff.LateHTTPStatus = result.Revision, http.StatusOK
		item.offerCutoff.PublishedTeamsUnchanged = true
		var frozen teams.Page
		var err error
		if os.Getenv("HACKBOX_LIFECYCLE_BROWSER_PHASES") == "ui-journey" {
			frozen, err = r.browserTeamActions(t, item, index)(teams.Command{Action: "freeze"})
		} else {
			frozen, err = lifecycleTeamCommand(t.Context(), r.owner, item.view.ID, teams.Command{Action: "freeze"}, http.StatusOK)
		}
		if err != nil {
			t.Fatal(err)
		}
		if !frozen.Frozen || len(frozen.Teams) != len(roster.Teams) {
			t.Fatal("actual post-cutoff team freeze did not commit")
		}
		for _, prior := range roster.Teams {
			found := false
			for _, team := range frozen.Teams {
				if team.ID != prior.ID {
					continue
				}
				found = true
				if !reflect.DeepEqual(prior.Published, team.Published) || prior.State != team.State || team.State == "published" && !team.Frozen {
					t.Fatal("freeze changed the published competitor assignment")
				}
			}
			if !found {
				t.Fatal("freeze replaced a published team identity")
			}
		}
		var destination uuid.UUID
		for _, team := range frozen.Teams {
			if team.State != "published" {
				continue
			}
			contains := false
			for _, member := range team.Published {
				contains = contains || member.ID == item.cohort.clients[3].person
			}
			if !contains {
				destination = team.ID
				break
			}
		}
		if destination == uuid.Nil {
			t.Fatal("freeze move denial lacks another real team")
		}
		if _, err := lifecycleTeamCommand(t.Context(), r.owner, item.view.ID, teams.Command{Action: "move", TeamID: destination, PersonID: item.cohort.clients[3].person}, http.StatusConflict); err != nil {
			t.Fatal(err)
		}
		if err := r.owner.call(t.Context(), http.MethodGet, base, -1, nil, &current, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		item.offerCutoff.FrozenAfterCutoff = true
		item.view, item.teams = current, frozen
	}
}
