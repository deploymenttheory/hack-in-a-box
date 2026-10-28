package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
	"github.com/deploymenttheory/hack-in-a-box/internal/repositories"
	"github.com/deploymenttheory/hack-in-a-box/internal/teams"
)

type lifecycleAttendanceReceipt struct {
	TeamID                uuid.UUID   `json:"smaller_team_id"`
	RepositoryID          int64       `json:"retained_repository_id"`
	Withdrawn             []uuid.UUID `json:"withdrawn_people"`
	Survivors             []uuid.UUID `json:"unmoved_survivors"`
	ConfirmedBefore       int64       `json:"confirmed_before"`
	ConfirmedAfter        int64       `json:"confirmed_after"`
	ConfiguredMinimum     int         `json:"configured_minimum"`
	MoveDenied            bool        `json:"post_freeze_move_denied"`
	LateAdmissionDenied   bool        `json:"post_cutoff_backfill_denied"`
	UnwaivedLiveDenied    bool        `json:"unwaived_live_denied"`
	EmptyReasonDenied     bool        `json:"empty_exception_reason_denied"`
	ExceptionDecisionID   uuid.UUID   `json:"attendance_exception_decision_id"`
	ExceptionRevision     int64       `json:"attendance_exception_revision"`
	ExceptionReasonSHA256 string      `json:"exception_reason_sha256"`
	OtherEventUnchanged   bool        `json:"other_event_unchanged"`
}

// Publication already established the configured go floor and native frozen
// teams. Actual later withdrawals can leave a pair; they cannot authorise moves
// or backfill, erase its repository, or silently cancel the invested event.
func (r *lifecycleJourney) depleteFrozenTeams(t *testing.T) {
	t.Helper()
	stranger, err := signInLifecycle(
		t.Context(),
		"http://public.localhost:8080",
		"http://provider.localhost:8080",
		"stranger",
	)
	if err != nil {
		t.Fatal(err)
	}
	for index := range r.events {
		item := &r.events[index]
		base := "/api/v1/events/" + item.view.ID.String()
		var before event.View
		if err := r.owner.call(
			t.Context(),
			http.MethodGet,
			base,
			-1,
			nil,
			&before,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		var selected *teams.Team
		for _, team := range item.teams.Teams {
			if team.State == "published" && team.Frozen && len(team.Published) == 5 {
				copy := team
				selected = &copy
				break
			}
		}
		if selected == nil || before.ConfirmedAttendees != 32 ||
			before.Configuration.MinimumAttendance != 30 ||
			before.PublishedAt == nil {
			t.Fatal(
				"post-freeze withdrawal lacks the genuine published target-five cohort and go decision",
			)
		}
		receipt := lifecycleAttendanceReceipt{
			TeamID:            selected.ID,
			ConfirmedBefore:   before.ConfirmedAttendees,
			ConfiguredMinimum: before.Configuration.MinimumAttendance,
			Withdrawn:         []uuid.UUID{},
			Survivors:         []uuid.UUID{selected.Published[0].ID, selected.Published[1].ID},
		}
		for _, member := range selected.Published[2:] {
			found := false
			for i, applied := range item.cohort.registrations {
				if applied.PersonID != member.ID {
					continue
				}
				found = true
				var withdrawn registration.View
				if os.Getenv("HACKBOX_LIFECYCLE_BROWSER_PHASES") == "ui-journey" {
					withdrawn = r.respondToAdmissionInBrowser(t, item, r.participant(t, member.ID),
						611+index*20+len(item.browserFrozenWithdrawals),
						lifecycleBrowserAdmissionResponse{Actor: fmt.Sprintf("attendee-%02d", i+1), Application: applied, Action: "withdraw"})
					item.browserFrozenWithdrawals = append(item.browserFrozenWithdrawals, withdrawn.PersonID)
				} else if err := r.participant(t, member.ID).
					call(t.Context(), http.MethodPost, base+"/registrations/"+applied.ID.String()+"/respond", applied.Revision, map[string]string{"action": "withdraw"}, &withdrawn, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				if withdrawn.State != "withdrawn" || withdrawn.PersonID != member.ID ||
					withdrawn.Revision <= applied.Revision {
					t.Fatal(
						"actual post-freeze withdrawal lacks its original person/revision acknowledgement",
					)
				}
				item.cohort.registrations[i] = withdrawn
				receipt.Withdrawn = append(receipt.Withdrawn, member.ID)
			}
			if !found {
				t.Fatal("selected frozen member has no native registration")
			}
		}
		var current teams.Page
		if err := r.owner.call(
			t.Context(),
			http.MethodGet,
			base+"/teams",
			-1,
			nil,
			&current,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if !current.Frozen || len(current.Teams) != len(item.teams.Teams) {
			t.Fatal("withdrawal replaced frozen team identities")
		}
		for _, prior := range item.teams.Teams {
			found := false
			for _, now := range current.Teams {
				if now.ID != prior.ID {
					continue
				}
				found = true
				if now.ID != selected.ID {
					if !reflect.DeepEqual(prior, now) {
						t.Fatal("withdrawal moved members of another frozen team")
					}
				} else if now.State != "published" || !now.Frozen || len(now.Published) != 2 || !reflect.DeepEqual(now.Published, prior.Published[:2]) {
					t.Fatal("a pair left after freeze was reassigned, retired or backfilled")
				}
			}
			if !found {
				t.Fatal("withdrawal removed an original frozen team identity")
			}
		}
		var repositoryViews []repositories.View
		if err := r.owner.call(
			t.Context(),
			http.MethodGet,
			base+"/repositories",
			-1,
			nil,
			&repositoryViews,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		for _, existing := range item.repositories {
			if existing.TeamID != selected.ID || existing.GitHubID == nil {
				continue
			}
			for _, retained := range repositoryViews {
				if retained.TeamID == existing.TeamID && retained.GitHubID != nil &&
					*retained.GitHubID == *existing.GitHubID &&
					retained.Name == existing.Name &&
					retained.State == "ready" &&
					!retained.Unused {
					receipt.RepositoryID = *retained.GitHubID
				}
			}
		}
		if receipt.RepositoryID == 0 {
			t.Fatal("post-freeze depletion lost the original usable private repository")
		}
		var destination uuid.UUID
		for _, team := range current.Teams {
			if team.State == "published" && team.ID != selected.ID {
				destination = team.ID
				break
			}
		}
		if destination == uuid.Nil {
			t.Fatal("move denial lacks another frozen team")
		}
		if _, err := lifecycleTeamCommand(
			t.Context(),
			r.owner,
			item.view.ID,
			teams.Command{Action: "move", TeamID: destination, PersonID: receipt.Survivors[0]},
			http.StatusConflict,
		); err != nil {
			t.Fatal(err)
		}
		receipt.MoveDenied = true
		var depleted event.View
		if err := r.owner.call(
			t.Context(),
			http.MethodGet,
			base,
			-1,
			nil,
			&depleted,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if depleted.Stage != "preparation" || depleted.ConfirmedAttendees != 29 ||
			len(receipt.Withdrawn) != 3 {
			t.Fatal(
				"post-go withdrawals did not preserve preparation with the actual below-minimum census",
			)
		}
		answers := forms.Answers{}
		for _, field := range item.cohort.attendeeForm.Definition.Fields {
			answers[field.ID.String()] = json.RawMessage(`"Synthetic prohibited late backfill"`)
		}
		input := registration.Application{
			ID:             uuid.New(),
			FormID:         item.cohort.attendeeForm.Definition.ID,
			FormVersion:    *item.cohort.attendeeForm.PublishedVersion,
			Answers:        answers,
			TeamPreference: "solo",
		}
		var rejected struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := stranger.call(
			t.Context(),
			http.MethodPost,
			base+"/registrations",
			-1,
			input,
			&rejected,
			http.StatusConflict,
		); err != nil {
			t.Fatal(err)
		}
		var unchanged event.View
		if err := r.owner.call(
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
		if rejected.Error.Code != "registration_closed" || !reflect.DeepEqual(depleted, unchanged) {
			t.Fatal("late backfill was not rejected without changing the frozen event")
		}
		receipt.LateAdmissionDenied, receipt.ConfirmedAfter = true, depleted.ConfirmedAttendees
		item.view, item.teams, item.attendance = depleted, current, receipt
	}
}

func (r *lifecycleJourney) enterLiveWithAttendanceException(t *testing.T) {
	t.Helper()
	for index := range r.events {
		item := &r.events[index]
		base := "/api/v1/events/" + item.view.ID.String()
		var before, otherBefore event.View
		otherPath := "/api/v1/events/" + r.events[1-index].view.ID.String()
		if err := r.owner.call(
			t.Context(),
			http.MethodGet,
			base,
			-1,
			nil,
			&before,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if err := r.owner.call(
			t.Context(),
			http.MethodGet,
			otherPath,
			-1,
			nil,
			&otherBefore,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if before.ConfirmedAttendees != item.attendance.ConfirmedAfter ||
			before.ConfirmedAttendees >= int64(before.Configuration.MinimumAttendance) {
			t.Fatal("attendance exception does not cover real below-minimum withdrawals")
		}
		for _, denied := range []struct {
			exception bool
			reason    string
		}{{false, "Do not bypass the current attendance floor"}, {true, ""}} {
			var rejected struct {
				Error struct {
					Code     string        `json:"code"`
					Blockers []event.Issue `json:"blockers"`
				} `json:"error"`
			}
			if err := r.owner.call(
				t.Context(),
				http.MethodPost,
				base+"/transition",
				before.Revision,
				map[string]any{
					"stage":                "live",
					"reason":               denied.reason,
					"attendance_exception": denied.exception,
				},
				&rejected,
				http.StatusConflict,
			); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, blocker := range rejected.Error.Blockers {
				found = found || blocker.Code == "attendance_shortfall"
			}
			var unchanged event.View
			if err := r.owner.call(
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
			if rejected.Error.Code != "event_blocked" || !found ||
				!reflect.DeepEqual(before, unchanged) {
				t.Fatal("unapproved or unreasoned attendance exception changed the native event")
			}
		}
		const reason = "Approve continuation after three actual post-freeze withdrawals; preserve the frozen pair and the team's invested work."
		var live event.View
		if err := r.owner.call(
			t.Context(),
			http.MethodPost,
			base+"/transition",
			before.Revision,
			map[string]any{"stage": "live", "reason": reason, "attendance_exception": true},
			&live,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if live.Stage != "live" || live.Revision != before.Revision+1 ||
			live.ConfirmedAttendees != 29 {
			t.Fatal("reasoned attendance exception lost its original live acknowledgement")
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
		for _, decision := range decisions.Items {
			if decision.Kind == "transition_to_live" && decision.ActorID == r.owner.person &&
				decision.Reason == reason &&
				decision.Revision == live.Revision {
				if item.attendance.ExceptionDecisionID != uuid.Nil {
					t.Fatal("one attendance exception created duplicate decisions")
				}
				item.attendance.ExceptionDecisionID = decision.ID
			}
		}
		if item.attendance.ExceptionDecisionID == uuid.Nil {
			t.Fatal("live attendance exception lacks the recorded current actor and reason")
		}
		var otherAfter event.View
		if err := r.owner.call(
			t.Context(),
			http.MethodGet,
			otherPath,
			-1,
			nil,
			&otherAfter,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(otherBefore, otherAfter) {
			t.Fatal("one attendance exception changed the concurrent event")
		}
		hash := sha256.Sum256([]byte(reason))
		item.attendance.ExceptionReasonSHA256 = hex.EncodeToString(hash[:])
		item.attendance.ExceptionRevision = live.Revision
		item.attendance.UnwaivedLiveDenied, item.attendance.EmptyReasonDenied, item.attendance.OtherEventUnchanged = true, true, true
		item.view = live
	}
}
