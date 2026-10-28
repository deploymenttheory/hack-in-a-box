package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
)

// Browser barriers are private fixture IPC, never application endpoints or
// authentication overrides. The native test independently reads every effect.
type lifecycleBrowserPhase struct {
	Objective   *lifecycleBrowserObjectiveInput    `json:"objective,omitempty"`
	Challenge   *lifecycleBrowserChallengeInput    `json:"challenge,omitempty"`
	Rubric      *lifecycleBrowserRubricInput       `json:"rubric,omitempty"`
	TeamChange  *lifecycleBrowserTeamChange        `json:"team_change,omitempty"`
	Form        *lifecycleBrowserFormInput         `json:"form,omitempty"`
	Sequence    int                                `json:"sequence"`
	Action      string                             `json:"action"`
	EventID     uuid.UUID                          `json:"event_id"`
	Now         time.Time                          `json:"domain_now"`
	Primary     string                             `json:"primary"`
	Actor       string                             `json:"actor,omitempty"`
	TeamID      *uuid.UUID                         `json:"team_id,omitempty"`
	Roles       *lifecycleBrowserRolesPhase        `json:"roles,omitempty"`
	Create      *lifecycleBrowserCreateEvent       `json:"create,omitempty"`
	Application *lifecycleBrowserApplication       `json:"application,omitempty"`
	Selection   *lifecycleBrowserSelection         `json:"selection,omitempty"`
	Response    *lifecycleBrowserAdmissionResponse `json:"response,omitempty"`
}

type lifecycleBrowserAcknowledgement struct {
	ObjectiveID   *uuid.UUID               `json:"objective_id,omitempty"`
	ContentID     *uuid.UUID               `json:"content_id,omitempty"`
	FormID        *uuid.UUID               `json:"form_id,omitempty"`
	Sequence      int                      `json:"sequence"`
	Action        string                   `json:"action"`
	EventID       uuid.UUID                `json:"event_id"`
	Saved         bool                     `json:"saved_by_ui"`
	Completed     bool                     `json:"completed_by_ui"`
	ApplicationID *uuid.UUID               `json:"application_id,omitempty"`
	Help          *lifecycleBrowserHelpAck `json:"help,omitempty"`
}

func lifecycleBrowserRequired(t *testing.T) bool {
	t.Helper()
	switch os.Getenv("HACKBOX_LIFECYCLE_BROWSER_PHASES") {
	case "":
		return false
	case "planning", "live-recovery", "role-matrix", "ui-journey":
		return true
	default:
		t.Fatal("unknown native browser phase selection")
		return false
	}
}

func (r *lifecycleJourney) saveBrandInBrowser(
	t *testing.T,
	item *lifecycleJourneyEvent,
	sequence int,
) {
	t.Helper()
	var clock struct {
		Now time.Time `json:"now"`
	}
	if err := r.control.call(t.Context(), http.MethodGet, "/clock", nil, &clock); err != nil {
		t.Fatal(err)
	}
	phase := lifecycleBrowserPhase{
		Sequence: sequence, Action: "save-branding", EventID: item.view.ID,
		Now: clock.Now, Primary: item.brand.Primary,
	}
	ack := r.awaitBrowserPhase(t, phase)
	if !ack.Saved {
		t.Fatal("branding phase did not report a UI save")
	}
	var native templates.BrandView
	if err := r.owner.call(
		t.Context(),
		http.MethodGet,
		"/api/v1/events/"+item.view.ID.String()+"/branding",
		-1,
		nil,
		&native,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(native.Branding, item.brand) || native.Revision != item.view.Revision+1 {
		t.Fatal("browser save differs from independently observed native branding/revision")
	}
	item.browserBrandVerified = true
}

func (r *lifecycleJourney) awaitBrowserPhase(
	t *testing.T,
	phase lifecycleBrowserPhase,
) lifecycleBrowserAcknowledgement {
	t.Helper()
	directory := "/artifacts/browser"
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(phase)
	if err != nil {
		t.Fatal(err)
	}
	request := filepath.Join(directory, fmt.Sprintf("phase-%d.json", phase.Sequence))
	if err := os.WriteFile(request+".tmp", encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(request+".tmp", request); err != nil {
		t.Fatal(err)
	}
	response := filepath.Join(directory, fmt.Sprintf("result-%d.json", phase.Sequence))
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	for {
		body, readErr := os.ReadFile(response)
		if readErr == nil {
			var ack lifecycleBrowserAcknowledgement
			if err := json.Unmarshal(body, &ack); err != nil {
				t.Fatal("invalid browser phase receipt")
			}
			sameEvent := ack.EventID == phase.EventID
			if phase.Action == "create-event" {
				sameEvent = phase.EventID == uuid.Nil && ack.EventID != uuid.Nil
			}
			if ack.Sequence != phase.Sequence || !sameEvent ||
				ack.Action != phase.Action ||
				!ack.Completed {
				t.Fatal("browser phase did not acknowledge the exact native event action")
			}
			return ack
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			t.Fatal(readErr)
		}
		select {
		case <-t.Context().Done():
			t.Fatal("native journey cancelled while awaiting browser action")
		case <-deadline.C:
			t.Fatal("native browser action did not finish within its bounded barrier")
		case <-ticker.C:
		}
	}
}

type lifecycleBrowserHelpAck struct {
	RequestID               uuid.UUID `json:"request_id"`
	AcceptedResponseDropped bool      `json:"accepted_response_dropped"`
	OfflineObserved         bool      `json:"offline_observed"`
	PutAttempts             int       `json:"put_attempts"`
}

type lifecycleBrowserHelpReceipt struct {
	EventID                 uuid.UUID `json:"event_id"`
	RequestID               uuid.UUID `json:"request_id"`
	AcceptedResponseDropped bool      `json:"accepted_response_dropped"`
	OfflineObserved         bool      `json:"offline_observed"`
	PutAttempts             int       `json:"put_attempts"`
	NativeResolved          bool      `json:"native_resolved"`
	OtherTeamDenied         bool      `json:"other_team_denied"`
	NativeQueueExact        bool      `json:"native_queue_exact"`
}

func (r *lifecycleJourney) recoverLiveHelpInBrowser(t *testing.T) {
	t.Helper()
	if mode := os.Getenv(
		"HACKBOX_LIFECYCLE_BROWSER_PHASES",
	); mode != "live-recovery" &&
		mode != "role-matrix" && mode != "ui-journey" {
		return
	}
	item := &r.events[0]
	own, other, _ := r.feedbackReaders(t, item, item.liveHelp.TeamID)
	actor := ""
	for index, person := range item.cohort.clients {
		if person.person == own.person {
			actor = fmt.Sprintf("attendee-%02d", index+1)
		}
	}
	if actor == "" {
		t.Fatal("browser recovery lacks the original cohort actor")
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
			Sequence: 3,
			Action:   "recover-live-help",
			EventID:  item.view.ID,
			Now:      clock.Now,
			Actor:    actor,
			TeamID:   &item.liveHelp.TeamID,
		},
	)
	if !ack.Saved || ack.Help == nil || ack.Help.RequestID == uuid.Nil ||
		!ack.Help.AcceptedResponseDropped ||
		!ack.Help.OfflineObserved ||
		ack.Help.PutAttempts != 1 {
		t.Fatal("browser recovery lacks an actual accepted-response loss and one submitted request")
	}
	path := "/api/v1/events/" + item.view.ID.String() + "/help/" + ack.Help.RequestID.String()
	var current help.Page
	if err := own.call(
		t.Context(),
		http.MethodGet,
		path,
		-1,
		nil,
		&current,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	request := current.Request
	if request.EventID != item.view.ID || request.TeamID != item.liveHelp.TeamID ||
		request.RequesterID != own.person ||
		request.Category != "competition_output" ||
		request.Description != "PRIVATE synthetic browser recovery help" ||
		request.Location != "Synthetic recovery desk" ||
		request.MeetingURL != "" ||
		request.Status != "resolved" ||
		request.Revision != 3 ||
		request.AssigneeID == nil ||
		*request.AssigneeID != item.cohort.staff["mentor-a"].person ||
		request.FirstClaimedAt == nil ||
		request.ResolvedAt == nil {
		t.Fatal(
			"recovered UI request differs from independently read native identity/input/claim/resolution",
		)
	}
	if err := other.call(
		t.Context(),
		http.MethodGet,
		path,
		-1,
		nil,
		nil,
		http.StatusNotFound,
	); err != nil {
		t.Fatal(err)
	}
	var queue help.Queue
	if err := r.owner.call(
		t.Context(),
		http.MethodGet,
		"/api/v1/events/"+item.view.ID.String()+"/help",
		-1,
		nil,
		&queue,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	ids := map[uuid.UUID]bool{item.liveHelp.RequestID: true, request.ID: true}
	if len(queue.Items) != 2 {
		t.Fatal("browser recovery duplicated or removed a native help request")
	}
	for _, summary := range queue.Items {
		if !ids[summary.ID] || summary.Status != "resolved" {
			t.Fatal("native recovery queue contains an unexpected request/state")
		}
		delete(ids, summary.ID)
	}
	if len(ids) != 0 {
		t.Fatal("native recovery queue lost an original request")
	}
	item.browserHelp = &lifecycleBrowserHelpReceipt{
		EventID:                 item.view.ID,
		RequestID:               request.ID,
		AcceptedResponseDropped: true,
		OfflineObserved:         true,
		PutAttempts:             1,
		NativeResolved:          true,
		OtherTeamDenied:         true,
		NativeQueueExact:        true,
	}
}
