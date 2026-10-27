package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/teams"
)

type lifecyclePostponementReceipt struct {
	OriginalLiveStart      time.Time `json:"original_live_start"`
	RevisedLiveStart       time.Time `json:"revised_live_start"`
	RevisedCutoff          time.Time `json:"revised_cutoff"`
	PublishedAt            time.Time `json:"original_publication_retained"`
	DecisionID             uuid.UUID `json:"decision_id"`
	NoticeID               uuid.UUID `json:"notice_id"`
	Revision               int64     `json:"revision"`
	ReasonSHA256           string    `json:"reason_sha256"`
	UnapprovedDenied       bool      `json:"unapproved_change_denied"`
	FrozenDeadlineDenied   bool      `json:"frozen_deadline_change_denied"`
	OtherEventUnchanged    bool      `json:"other_event_unchanged"`
	FrozenTeamsPreserved   bool      `json:"frozen_teams_preserved"`
	CommonContentPreserved bool      `json:"common_content_preserved"`
	CurrentAudience        bool      `json:"current_notice_audience_verified"`
}

// Deliberately postpone each event separately after the common release. Keeping
// its original publication and frozen identities proves this is a recorded
// exception, rather than silently scheduling a second theme release.
func (r *lifecycleJourney) postponePublishedEvents(t *testing.T) {
	t.Helper()
	readEvent := func(id uuid.UUID) event.View {
		t.Helper()
		var value event.View
		if err := r.owner.call(
			t.Context(),
			http.MethodGet,
			"/api/v1/events/"+id.String(),
			-1,
			nil,
			&value,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		return value
	}
	for index := range r.events {
		item := &r.events[index]
		other := r.events[1-index].view.ID
		otherBefore := readEvent(other)
		before := readEvent(item.view.ID)
		base := "/api/v1/events/" + before.ID.String()
		if before.PublishedAt == nil || before.Stage != "preparation" {
			t.Fatal("postponement requires the actual released preparation event")
		}
		var roster teams.Page
		if err := r.owner.call(
			t.Context(),
			http.MethodGet,
			base+"/teams",
			-1,
			nil,
			&roster,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		reader := item.cohort.clients[3]
		var released content.Page
		contentPath := base + "/content/" + item.challenge.ID.String()
		if err := reader.call(
			t.Context(),
			http.MethodGet,
			contentPath,
			-1,
			nil,
			&released,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		location, err := time.LoadLocation(before.Configuration.Timezone)
		if err != nil {
			t.Fatal(err)
		}
		next := before.Configuration
		next.LiveStart = next.LiveStart.In(location).AddDate(0, 0, 1).UTC()
		next.LiveEnd = next.LiveEnd.In(location).AddDate(0, 0, 1).UTC()
		next.SubmissionCutoff = next.SubmissionCutoff.In(location).AddDate(0, 0, 1).UTC()
		input := event.Input{
			ID:                                before.ID,
			Name:                              before.Name,
			Slug:                              before.Slug,
			Configuration:                     next,
			AcknowledgeCompetitionRestriction: true,
		}
		deny := func(body event.Input, revision int64, code string) {
			t.Helper()
			var rejected struct {
				Error struct {
					Code     string        `json:"code"`
					Blockers []event.Issue `json:"blockers"`
				} `json:"error"`
			}
			current := readEvent(item.view.ID)
			if err := r.owner.call(
				t.Context(),
				http.MethodPut,
				base,
				revision,
				body,
				&rejected,
				http.StatusConflict,
			); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, blocker := range rejected.Error.Blockers {
				found = found || blocker.Code == code
			}
			if rejected.Error.Code != "event_blocked" || !found ||
				!reflect.DeepEqual(current, readEvent(item.view.ID)) {
				t.Fatal(
					"denied date exception changed current event state or lost its specific blocker",
				)
			}
		}
		deny(input, before.Revision, "postponement_approval")
		const reason = "Synthetic venue disruption: approve the later dates and retain the released theme and existing team work."
		input.ApprovePostponement, input.Reason = true, reason
		var changed event.View
		if err := r.owner.call(
			t.Context(),
			http.MethodPut,
			base,
			before.Revision,
			input,
			&changed,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if changed.Revision != before.Revision+1 || changed.PublishedAt == nil ||
			!changed.PublishedAt.Equal(*before.PublishedAt) ||
			changed.Stage != before.Stage ||
			!reflect.DeepEqual(changed.Configuration, next) {
			t.Fatal(
				"approved postponement lost its exact dates, original publication or native acknowledgement",
			)
		}
		var decisions struct {
			Items []struct {
				ID       uuid.UUID `json:"id"`
				ActorID  uuid.UUID `json:"actor_id"`
				Kind     string    `json:"kind"`
				Reason   string    `json:"reason"`
				Revision int64     `json:"revision"`
			} `json:"items"`
		}
		if err := r.owner.call(
			t.Context(),
			http.MethodGet,
			base+"/decisions",
			-1,
			nil,
			&decisions,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		receipt := lifecyclePostponementReceipt{
			OriginalLiveStart: before.Configuration.LiveStart,
			RevisedLiveStart:  next.LiveStart,
			RevisedCutoff:     next.SubmissionCutoff,
			PublishedAt:       *before.PublishedAt,
			Revision:          changed.Revision,
			UnapprovedDenied:  true,
		}
		for _, decision := range decisions.Items {
			if decision.Kind == "postponement" && decision.Revision == changed.Revision &&
				decision.ActorID == r.owner.person &&
				decision.Reason == reason {
				if receipt.DecisionID != uuid.Nil {
					t.Fatal("one approved postponement recorded duplicate decisions")
				}
				receipt.DecisionID = decision.ID
			}
		}
		if receipt.DecisionID == uuid.Nil {
			t.Fatal("approved postponement has no exact actor/reason/revision decision")
		}
		hash := sha256.Sum256([]byte(reason))
		receipt.ReasonSHA256 = hex.EncodeToString(hash[:])
		var notices struct {
			Items []content.Notice `json:"items"`
		}
		for _, currentReader := range []*lifecycleClient{reader, item.cohort.staff["mentor-a"]} {
			if err := currentReader.call(
				t.Context(),
				http.MethodGet,
				base+"/notices",
				-1,
				nil,
				&notices,
				http.StatusOK,
			); err != nil {
				t.Fatal(err)
			}
			if len(notices.Items) != 1 {
				t.Fatal("postponement lacks one current native revised-date notice")
			}
			notice := notices.Items[0]
			if notice.Revision != changed.Revision || !notice.LiveStart.Equal(next.LiveStart) ||
				!notice.LiveEnd.Equal(next.LiveEnd) ||
				!notice.SubmissionCutoff.Equal(next.SubmissionCutoff) ||
				receipt.NoticeID != uuid.Nil && receipt.NoticeID != notice.ID {
				t.Fatal("current reader received different or unacknowledged revised dates")
			}
			receipt.NoticeID = notice.ID
		}
		if err := item.cohort.clients[0].call(
			t.Context(),
			http.MethodGet,
			base+"/notices",
			-1,
			nil,
			nil,
			http.StatusForbidden,
		); err != nil {
			t.Fatal(err)
		}
		receipt.CurrentAudience = true
		var afterRoster teams.Page
		if err := r.owner.call(
			t.Context(),
			http.MethodGet,
			base+"/teams",
			-1,
			nil,
			&afterRoster,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if !roster.Frozen || !afterRoster.Frozen ||
			!reflect.DeepEqual(roster.Teams, afterRoster.Teams) {
			t.Fatal("postponement reopened or reassigned frozen team identities")
		}
		receipt.FrozenTeamsPreserved = true
		var afterContent content.Page
		if err := reader.call(
			t.Context(),
			http.MethodGet,
			contentPath,
			-1,
			nil,
			&afterContent,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		var publicRubric json.RawMessage
		if err := reader.call(
			t.Context(),
			http.MethodGet,
			base+"/rubric",
			-1,
			nil,
			&publicRubric,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		rubricHash := sha256.Sum256(publicRubric)
		if !reflect.DeepEqual(released.Document, afterContent.Document) ||
			!reflect.DeepEqual(released.ReleasedVersion, afterContent.ReleasedVersion) ||
			!reflect.DeepEqual(released.ReleasedAt, afterContent.ReleasedAt) ||
			!reflect.DeepEqual(released.AcknowledgedAt, afterContent.AcknowledgedAt) ||
			hex.EncodeToString(rubricHash[:]) != item.commonHash {
			t.Fatal(
				"postponement changed published challenge/rubric bytes or original acknowledgement",
			)
		}
		receipt.CommonContentPreserved = true
		invalid := input
		invalid.Configuration.TeamFreeze = invalid.Configuration.TeamFreeze.Add(time.Hour)
		deny(invalid, changed.Revision, "already_frozen")
		receipt.FrozenDeadlineDenied = true
		if !reflect.DeepEqual(otherBefore, readEvent(other)) {
			t.Fatal("postponing one event changed the concurrent event")
		}
		receipt.OtherEventUnchanged = true
		item.view, item.postponement = changed, receipt
	}
	// Subsequent native mentor/practice/cutoff/retention planning uses revised
	// dates. Publication remains the actual original common-release instant.
	r.timeline.Live = r.events[0].view.Configuration.LiveStart
	r.timeline.Cutoff = r.events[0].view.Configuration.SubmissionCutoff
	r.timeline.Completed = r.events[0].view.Configuration.LiveEnd
	for _, item := range r.events {
		if !item.view.Configuration.LiveStart.Equal(r.timeline.Live) ||
			!item.view.Configuration.SubmissionCutoff.Equal(r.timeline.Cutoff) ||
			!item.view.Configuration.LiveEnd.Equal(r.timeline.Completed) {
			t.Fatal("the two separately approved exception timelines did not align")
		}
	}
}
