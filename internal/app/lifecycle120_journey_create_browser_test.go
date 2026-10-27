package app

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

// The ordinary EventForm generates its own resource ID. Fixture IPC carries
// requested safe inputs; no ID, session or application write is injected.
type lifecycleBrowserCreateEvent struct {
	Name          string              `json:"name"`
	Slug          string              `json:"slug"`
	Configuration event.Configuration `json:"configuration"`
}

func (r *lifecycleJourney) creationDirectory(t *testing.T) []event.View {
	t.Helper()
	var page struct {
		Items []event.View `json:"items"`
	}
	if err := r.owner.call(
		t.Context(),
		http.MethodGet,
		"/api/v1/events?limit=50",
		-1,
		nil,
		&page,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	return page.Items
}

func (r *lifecycleJourney) createEventInBrowser(
	t *testing.T,
	item *lifecycleJourneyEvent,
	input event.Input,
	index int,
) {
	t.Helper()
	before := r.creationDirectory(t)
	if len(before) != index {
		t.Fatal("UI creation must begin with the exact earlier native events")
	}
	var clock struct {
		Now time.Time `json:"now"`
	}
	if err := r.control.call(t.Context(), http.MethodGet, "/clock", nil, &clock); err != nil {
		t.Fatal(err)
	}
	ack := r.awaitBrowserPhase(
		t,
		lifecycleBrowserPhase{
			Sequence: 100 + index,
			Action:   "create-event",
			Now:      clock.Now,
			Create: &lifecycleBrowserCreateEvent{
				Name:          input.Name,
				Slug:          input.Slug,
				Configuration: input.Configuration,
			},
		},
	)
	if !ack.Saved || ack.EventID == uuid.Nil {
		t.Fatal("new event lacks its real UI save acknowledgement")
	}
	for _, earlier := range before {
		if earlier.ID == ack.EventID {
			t.Fatal("UI creation reused an earlier event")
		}
	}
	var current event.View
	if err := r.owner.call(
		t.Context(),
		http.MethodGet,
		"/api/v1/events/"+ack.EventID.String(),
		-1,
		nil,
		&current,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	if current.ID != ack.EventID || current.Name != input.Name || current.Slug != input.Slug ||
		!reflect.DeepEqual(current.Configuration, input.Configuration) ||
		current.OwnerID == nil ||
		*current.OwnerID != r.owner.person ||
		current.Stage != "draft" ||
		current.Revision != 1 ||
		current.ConfirmedAttendees != 0 ||
		current.PublishedAt != nil ||
		!current.CanEdit ||
		!current.CanManageOwner ||
		current.ChallengePublication == nil ||
		!current.ChallengePublication.Equal(r.timeline.Publication) {
		t.Fatal(
			"native UI-created event differs from exact intended identity/configuration/ownership/draft gates",
		)
	}
	after := r.creationDirectory(t)
	if len(after) != len(before)+1 {
		t.Fatal("reviewing and creating an event duplicated or removed native resources")
	}
	for _, earlier := range before {
		found := false
		for _, saved := range after {
			if saved.ID == earlier.ID {
				found = true
				if !reflect.DeepEqual(saved, earlier) {
					t.Fatal("creating another event mutated the earlier native view")
				}
			}
		}
		if !found {
			t.Fatal("UI creation removed an earlier event")
		}
	}
	item.view = current
	item.browserEventCreated = true
}
