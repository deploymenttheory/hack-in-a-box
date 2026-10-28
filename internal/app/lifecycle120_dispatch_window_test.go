package app

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

type lifecycleDispatchFixture struct {
	view      event.View
	cohort    lifecycleAdmissions
	challenge content.Page
	rubric    rubric.Configuration
	receipt   lifecycleDispatchReceipt
}

type lifecycleDispatchReceipt struct {
	EventID                  uuid.UUID  `json:"event_id"`
	Mode                     string     `json:"mode"`
	ChallengeID              uuid.UUID  `json:"challenge_id"`
	ApprovedRevision         int64      `json:"original_approved_revision"`
	OriginalVersion          int64      `json:"original_version"`
	OriginalLiveStart        time.Time  `json:"original_live_start"`
	ScheduledAt              time.Time  `json:"scheduled_at"`
	DispatchAt               time.Time  `json:"controlled_domain_dispatch_at"`
	ObservedState            string     `json:"native_dispatch_state"`
	PublishedAt              *time.Time `json:"published_at"`
	MissedHistoryAt          *time.Time `json:"original_missed_history_at"`
	UnpublishedReadersDenied bool       `json:"missed_release_readers_denied"`
	ManualLiveDenied         bool       `json:"manual_live_cannot_bypass_missed_release"`
	PastApprovalDenied       bool       `json:"past_schedule_reapproval_denied"`
	RecoveryLiveStart        *time.Time `json:"recovery_live_start,omitempty"`
	RecoveryScheduledAt      *time.Time `json:"recovery_scheduled_at,omitempty"`
	RecoveryPublishedAt      *time.Time `json:"recovery_published_at,omitempty"`
	RecoveryVersion          int64      `json:"recovery_version,omitempty"`
	FrozenTeamsPreserved     bool       `json:"recovery_frozen_teams_preserved"`
}

// The host starts the actual native worker only after dispatch-ready.json is
// written. No test invokes maintenance/Dispatch or edits a due job in SQL. Both
// modes use genuine OIDC, registrations, frozen teams and reviewed content.
func TestLifecycleComposeDispatchWindow(t *testing.T) {
	if os.Getenv("HACKBOX_LIFECYCLE_ISOLATED") != "synthetic-only" {
		t.Skip("isolated native lifecycle dispatch cases only")
	}
	name := os.Getenv("HACKBOX_LIFECYCLE_DISPATCH_CASE")
	if name != "last-on-time" && name != "first-late" {
		t.Fatal("choose one explicit dispatch boundary case")
	}
	control, err := newLifecycleControl(os.Getenv("HACKBOX_LIFECYCLE_CONTROL"))
	if err != nil {
		t.Fatal(err)
	}
	defer control.close()
	timeline, err := control.initialTimeline(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	owner, err := signInLifecycle(t.Context(), "http://admin.localhost:8080", "http://provider.localhost:8080", "owner")
	if err != nil {
		t.Fatal(err)
	}
	items := []lifecycleDispatchFixture{}
	for _, mode := range []string{rubric.AllTeams, rubric.MentorQualification} {
		var view event.View
		if err := owner.call(
			t.Context(),
			http.MethodPost,
			"/api/v1/events",
			-1,
			lifecycleEventInput("Dispatch "+mode, timeline),
			&view,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		cohort, err := admitLifecycleCohort(t.Context(), owner, view)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := allocateLifecycleTeams(
			t.Context(),
			owner,
			view.ID,
			&cohort,
			control,
			nil,
		); err != nil {
			t.Fatal(err)
		}
		challenge, configured, err := installLifecycleChallenge(t.Context(), owner, view.ID, mode)
		if err != nil {
			t.Fatal(err)
		}
		if configured.Definition.Mode != mode || challenge.State != "approved" ||
			challenge.ReleasedVersion != nil ||
			challenge.Document.ScheduledAt == nil ||
			!challenge.Document.ScheduledAt.Equal(timeline.Publication) {
			t.Fatal("dispatch case does not have actual approved scheduled content")
		}
		items = append(
			items,
			lifecycleDispatchFixture{
				view:      view,
				cohort:    cohort,
				challenge: challenge,
				rubric:    configured,
				receipt: lifecycleDispatchReceipt{
					EventID:           view.ID,
					Mode:              mode,
					ChallengeID:       challenge.ID,
					ApprovedRevision:  challenge.Revision,
					OriginalVersion:   challenge.Version,
					OriginalLiveStart: timeline.Live,
					ScheduledAt:       timeline.Publication,
				},
			},
		)
	}
	if err := control.advance(t.Context(), timeline.Publication.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	for i := range items {
		dispatchReadersDenied(t, &items[i])
	}
	// Independent user-approved limit, not a value copied from the implementation.
	dispatchAt := timeline.Publication.Add(time.Minute)
	if name == "first-late" {
		dispatchAt = dispatchAt.Add(time.Microsecond)
	}
	if err := control.advance(t.Context(), dispatchAt); err != nil {
		t.Fatal(err)
	}
	for i := range items {
		items[i].receipt.DispatchAt = dispatchAt
		var before event.View
		if err := owner.call(
			t.Context(),
			http.MethodGet,
			"/api/v1/events/"+items[i].view.ID.String(),
			-1,
			nil,
			&before,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if before.PublishedAt != nil {
			t.Fatal("worker dispatched before the host started it")
		}
	}
	ready, err := json.Marshal(
		map[string]any{"case": name, "domain_dispatch_at": dispatchAt, "event_count": len(items)},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join("/artifacts", "dispatch-ready.json"),
		ready,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	expected := "released"
	if name == "first-late" {
		expected = "missed"
	}
	for i := range items {
		item := &items[i]
		page := waitLifecycleDispatchState(t, owner, item.view.ID, item.challenge.ID, expected)
		item.receipt.ObservedState = page.State
		var current event.View
		base := "/api/v1/events/" + item.view.ID.String()
		if err := owner.call(
			t.Context(),
			http.MethodGet,
			base,
			-1,
			nil,
			&current,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if name == "last-on-time" {
			if current.PublishedAt == nil || !current.PublishedAt.Equal(dispatchAt) ||
				page.ReleasedVersion == nil ||
				*page.ReleasedVersion != item.challenge.Version ||
				page.ReleasedAt == nil ||
				!page.ReleasedAt.Equal(dispatchAt) {
				t.Fatal(
					"the actual native worker did not accept the inclusive sixty-second boundary",
				)
			}
			var visible content.Page
			if err := item.cohort.clients[3].call(
				t.Context(),
				http.MethodGet,
				base+"/content/"+item.challenge.ID.String(),
				-1,
				nil,
				&visible,
				http.StatusOK,
			); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(visible.ReleasedVersion, page.ReleasedVersion) {
				t.Fatal("on-time reader did not receive the actual released version")
			}
			item.receipt.PublishedAt = current.PublishedAt
			continue
		}
		if current.PublishedAt != nil || page.ReleasedVersion != nil || page.ReleasedAt != nil {
			t.Fatal("first late native dispatch disclosed the missed challenge")
		}
		dispatchReadersDenied(t, item)
		item.receipt.UnpublishedReadersDenied = true
		item.receipt.MissedHistoryAt = lifecycleMissedHistory(t, owner, item, dispatchAt)
		var rejected struct {
			Error struct {
				Code     string        `json:"code"`
				Blockers []event.Issue `json:"blockers"`
			} `json:"error"`
		}
		if err := owner.call(
			t.Context(),
			http.MethodPost,
			base+"/transition",
			current.Revision,
			map[string]any{
				"stage":                "live",
				"reason":               "An attendance exception cannot replace a missed common publication",
				"attendance_exception": true,
			},
			&rejected,
			http.StatusConflict,
		); err != nil {
			t.Fatal(err)
		}
		if !lifecycleDispatchBlocker(rejected.Error.Code, rejected.Error.Blockers, "unpublished") {
			t.Fatal("manual live transition bypassed the actual missed-release gate")
		}
		var unchanged event.View
		if err := owner.call(
			t.Context(),
			http.MethodGet,
			base,
			-1,
			nil,
			&unchanged,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(current, unchanged) {
			t.Fatal("rejected manual live transition changed the missed event")
		}
		item.receipt.ManualLiveDenied = true
		input := content.Input{Kind: "challenge", Document: item.challenge.Document}
		input.Document.ScheduledAt = nil
		path := base + "/content/" + item.challenge.ID.String()
		if err := owner.call(
			t.Context(),
			http.MethodPut,
			path,
			page.Revision,
			input,
			&page,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if err := owner.call(
			t.Context(),
			http.MethodPost,
			path+"/actions",
			page.Revision,
			map[string]string{
				"action": "review",
				"reason": "Review the same missed schedule without changing dates",
			},
			&page,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		beforeReview := page
		if err := owner.call(
			t.Context(),
			http.MethodPost,
			path+"/actions",
			page.Revision,
			map[string]string{
				"action": "approve",
				"reason": "A reviewed draft cannot silently recover a missed preparation week",
			},
			&rejected,
			http.StatusConflict,
		); err != nil {
			t.Fatal(err)
		}
		if !lifecycleDispatchBlocker(
			rejected.Error.Code,
			rejected.Error.Blockers,
			"missed_schedule",
		) {
			t.Fatal("past common schedule was reapproved without postponement")
		}
		if err := owner.call(
			t.Context(),
			http.MethodGet,
			path+"?preview=true",
			-1,
			nil,
			&page,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(beforeReview, page) {
			t.Fatal("denied late approval changed the native draft")
		}
		item.receipt.PastApprovalDenied = true
		item.challenge, item.view = page, current
	}
	if name == "first-late" {
		for i := range items {
			recoverLifecycleMissedRelease(t, owner, &items[i], dispatchAt)
		}
		newPublication := timeline.Publication.In(mustLifecycleLocation(t)).AddDate(0, 0, 1).UTC()
		if err := control.advance(t.Context(), newPublication.Add(-time.Second)); err != nil {
			t.Fatal(err)
		}
		for i := range items {
			dispatchReadersDenied(t, &items[i])
		}
		if err := control.advance(t.Context(), newPublication); err != nil {
			t.Fatal(err)
		}
		for i := range items {
			item := &items[i]
			page := waitLifecycleDispatchState(
				t,
				owner,
				item.view.ID,
				item.challenge.ID,
				"released",
			)
			if page.Version <= item.receipt.OriginalVersion || page.ReleasedAt == nil ||
				!page.ReleasedAt.Equal(newPublication) ||
				page.ReleasedVersion == nil ||
				*page.ReleasedVersion != page.Version {
				t.Fatal(
					"missed release was not replaced by an explicitly reviewed new scheduled version",
				)
			}
			var current event.View
			if err := owner.call(
				t.Context(),
				http.MethodGet,
				"/api/v1/events/"+item.view.ID.String(),
				-1,
				nil,
				&current,
				http.StatusOK,
			); err != nil {
				t.Fatal(err)
			}
			if current.PublishedAt == nil || !current.PublishedAt.Equal(newPublication) {
				t.Fatal("postponed event lost its actual new publication instant")
			}
			if !reflect.DeepEqual(
				item.receipt.MissedHistoryAt,
				lifecycleMissedHistory(t, owner, item, dispatchAt),
			) {
				t.Fatal("recovery rewrote original missed dispatch history")
			}
			var visible content.Page
			if err := item.cohort.clients[3].call(
				t.Context(),
				http.MethodGet,
				"/api/v1/events/"+item.view.ID.String()+"/content/"+item.challenge.ID.String(),
				-1,
				nil,
				&visible,
				http.StatusOK,
			); err != nil {
				t.Fatal(err)
			}
			if visible.ReleasedVersion == nil || *visible.ReleasedVersion != page.Version {
				t.Fatal("recovered attendee response lost the actual released version")
			}
			item.receipt.RecoveryPublishedAt, item.receipt.RecoveryVersion = current.PublishedAt, *visible.ReleasedVersion
		}
	}
	receipts := []lifecycleDispatchReceipt{}
	for _, item := range items {
		receipts = append(receipts, item.receipt)
	}
	proof, err := json.MarshalIndent(
		map[string]any{
			"case":                   name,
			"complete_lifecycle":     false,
			"allowed_window_seconds": 60,
			"late_precision":         "one PostgreSQL microsecond after the inclusive limit",
			"worker_dispatch":        "actual native worker started by host after ready barrier; no direct handler or SQL setup",
			"events":                 receipts,
		},
		"",
		"  ",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("/artifacts/dispatch-window-proof.json", proof, 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustLifecycleLocation(t *testing.T) *time.Location {
	t.Helper()
	value, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func lifecycleDispatchBlocker(code string, blockers []event.Issue, wanted string) bool {
	if code != "event_blocked" {
		return false
	}
	for _, blocker := range blockers {
		if blocker.Code == wanted {
			return true
		}
	}
	return false
}

func dispatchReadersDenied(t *testing.T, item *lifecycleDispatchFixture) {
	t.Helper()
	base := "/api/v1/events/" + item.view.ID.String()
	for _, path := range []string{base + "/content/" + item.challenge.ID.String(), base + "/rubric"} {
		if err := item.cohort.clients[3].call(
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
	}
	var page content.List
	if err := item.cohort.clients[3].call(
		t.Context(),
		http.MethodGet,
		base+"/content?q=Synthetic",
		-1,
		nil,
		&page,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 0 || page.NextCursor != nil {
		t.Fatal("unreleased challenge leaked through search metadata")
	}
}

func waitLifecycleDispatchState(
	t *testing.T,
	owner *lifecycleClient,
	eventID, id uuid.UUID,
	state string,
) content.Page {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var page content.Page
		if err := owner.call(
			t.Context(),
			http.MethodGet,
			"/api/v1/events/"+eventID.String()+"/content/"+id.String()+"?preview=true",
			-1,
			nil,
			&page,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if page.State == state {
			return page
		}
		if page.State != "approved" {
			t.Fatal("native dispatcher entered an unexpected state: " + page.State)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("actual native worker did not reach expected dispatch state: " + state)
	return content.Page{}
}

func lifecycleMissedHistory(
	t *testing.T,
	owner *lifecycleClient,
	item *lifecycleDispatchFixture,
	at time.Time,
) *time.Time {
	t.Helper()
	var history content.History
	if err := owner.call(
		t.Context(),
		http.MethodGet,
		"/api/v1/events/"+item.view.ID.String()+"/content/"+item.challenge.ID.String()+"/history",
		-1,
		nil,
		&history,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	var found *time.Time
	for _, change := range history.Items {
		if change.Action == "missed" && change.Version == item.receipt.OriginalVersion {
			if found != nil || !change.At.Equal(at) {
				t.Fatal("native missed history duplicated or lost its actual domain instant")
			}
			copy := change.At
			found = &copy
		}
	}
	if found == nil || history.NextCursor != nil {
		t.Fatal("original actual missed dispatch history is absent or not fully read")
	}
	return found
}

func recoverLifecycleMissedRelease(
	t *testing.T,
	owner *lifecycleClient,
	item *lifecycleDispatchFixture,
	originalDispatch time.Time,
) {
	t.Helper()
	base := "/api/v1/events/" + item.view.ID.String()
	var beforeTeams struct {
		Teams json.RawMessage `json:"teams"`
	}
	if err := owner.call(
		t.Context(),
		http.MethodGet,
		base+"/teams",
		-1,
		nil,
		&beforeTeams,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	current := item.view
	configuration := current.Configuration
	location := mustLifecycleLocation(t)
	configuration.LiveStart = configuration.LiveStart.In(location).AddDate(0, 0, 1).UTC()
	configuration.LiveEnd = configuration.LiveEnd.In(location).AddDate(0, 0, 1).UTC()
	configuration.SubmissionCutoff = configuration.SubmissionCutoff.In(location).
		AddDate(0, 0, 1).
		UTC()
	input := event.Input{
		ID:                                current.ID,
		Name:                              current.Name,
		Slug:                              current.Slug,
		Configuration:                     configuration,
		Reason:                            "Postpone after the recorded missed dispatch to preserve the full preparation week",
		AcknowledgeCompetitionRestriction: true,
	}
	if err := owner.call(
		t.Context(),
		http.MethodPut,
		base,
		current.Revision,
		input,
		&current,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	if current.PublishedAt != nil || current.ChallengePublication == nil ||
		!current.ChallengePublication.After(originalDispatch) {
		t.Fatal("missed event did not preserve a full new preparation week")
	}
	var afterTeams struct {
		Teams json.RawMessage `json:"teams"`
	}
	if err := owner.call(
		t.Context(),
		http.MethodGet,
		base+"/teams",
		-1,
		nil,
		&afterTeams,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeTeams.Teams, afterTeams.Teams) {
		t.Fatal("missed-release postponement changed frozen team identities")
	}
	path := base + "/content/" + item.challenge.ID.String()
	var page content.Page
	if err := owner.call(
		t.Context(),
		http.MethodGet,
		path+"?preview=true",
		-1,
		nil,
		&page,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	draft := content.Input{Kind: "challenge", Document: item.challenge.Document}
	draft.Document.ScheduledAt = nil
	if err := owner.call(
		t.Context(),
		http.MethodPut,
		path,
		page.Revision,
		draft,
		&page,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"review", "approve"} {
		if err := owner.call(
			t.Context(),
			http.MethodPost,
			path+"/actions",
			page.Revision,
			map[string]string{
				"action": action,
				"reason": "Explicitly review and approve the postponed common release",
			},
			&page,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
	}
	if page.Document.ScheduledAt == nil ||
		!page.Document.ScheduledAt.Equal(*current.ChallengePublication) {
		t.Fatal("new approval did not bind the revised common schedule")
	}
	item.challenge, item.view = page, current
	item.receipt.RecoveryLiveStart = &current.Configuration.LiveStart
	item.receipt.RecoveryScheduledAt = current.ChallengePublication
	item.receipt.FrozenTeamsPreserved = true
}
