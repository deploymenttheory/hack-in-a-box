package app

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
)

type lifecycleMentorReviewReceipt struct {
	CoverageRevision  int64                          `json:"coverage_revision"`
	Assignments       []mentors.Assignment           `json:"assignments"`
	Checkpoint        scorecards.Checkpoint          `json:"checkpoint"`
	Cards             []lifecycleMentorCardReceipt   `json:"live_checkpoint_cards"`
	EarlyWritesDenied int                            `json:"early_checkpoint_writes_denied"`
	FinalCheckpoint   *scorecards.Checkpoint         `json:"final_checkpoint,omitempty"`
	FinalCards        []lifecycleMentorCardReceipt   `json:"final_cards,omitempty"`
	FinalHuddles      []lifecycleFinalHuddleReceipt  `json:"final_huddles,omitempty"`
	Qualification     *lifecycleQualificationReceipt `json:"qualification,omitempty"`
}

type lifecycleMentorCardReceipt struct {
	TeamID    uuid.UUID                `json:"team_id"`
	AuthorID  uuid.UUID                `json:"author_id"`
	Reference scorecards.CardReference `json:"reference"`
	RunID     uuid.UUID                `json:"run_id"`
}

// Coverage and a separate live checkpoint are configured before live. They
// cannot supply a final judge card or silently accumulate into final results.
func (r *lifecycleJourney) prepareMentorReview(t *testing.T) {
	t.Helper()
	for index := range r.events {
		item := &r.events[index]
		base := "/api/v1/events/" + item.view.ID.String()
		path := base + "/mentor-coverage"
		var page mentors.Page
		if err := r.owner.call(t.Context(), http.MethodGet, path, -1, nil, &page, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		window := mentors.Window{Start: item.view.Configuration.LiveStart, End: item.view.Configuration.LiveEnd}
		for _, actor := range []string{"mentor-a", "mentor-b"} {
			client := item.cohort.staff[actor]
			if client == nil {
				t.Fatal("selected native mentor missing", actor)
			}
			if err := r.owner.call(t.Context(), http.MethodPost, path, page.Revision, map[string]any{"action": "profile", "reason": "Actual selected mentor availability for independent live review", "person_id": client.person, "skills": []string{"design"}, "availability": []mentors.Window{window}, "maximum_teams": 6}, &page, http.StatusOK); err != nil {
				t.Fatal(err)
			}
		}
		for _, team := range item.teams.Teams {
			if team.State == "disbanded" {
				continue
			}
			for _, selection := range []struct{ slot, actor string }{{"primary", "mentor-a"}, {"second", "mentor-b"}} {
				slot, actor := selection.slot, selection.actor
				if err := r.owner.call(t.Context(), http.MethodPost, path, page.Revision, map[string]any{"action": "assign", "reason": "Distinct current primary and second mentor for this frozen team", "team_id": team.ID, "person_id": item.cohort.staff[actor].person, "slot": slot, "window": window}, &page, http.StatusOK); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := r.owner.call(t.Context(), http.MethodPost, path, page.Revision, map[string]any{"action": "publish", "reason": "Reviewed complete independent primary and second coverage"}, &page, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if len(page.Published) != 12 || len(page.PublishedGaps) != 0 {
			t.Fatal("native coverage is incomplete")
		}
		item.review.CoverageRevision = page.Revision
		item.review.Assignments = page.Published
		var configured rubric.Configuration
		if err := r.owner.call(t.Context(), http.MethodGet, base+"/rubric/configuration", -1, nil, &configured, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if os.Getenv("HACKBOX_LIFECYCLE_BROWSER_PHASES") == "ui-journey" {
			configured = r.createMentorRunInBrowser(t, item, 1901+index*20, "checkpoint", "Independent live observation, never final score carry", configured)
		} else if err := r.owner.call(t.Context(), http.MethodPost, base+"/rubric/configuration", configured.Revision, map[string]any{"action": "create_run", "version": configured.Reference.Version, "purpose": "checkpoint", "reason": "Independent live observation, never final score carry"}, &configured, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		runID := uuid.Nil
		for _, run := range configured.Runs {
			if run.Purpose == "checkpoint" {
				if runID != uuid.Nil {
					t.Fatal("duplicate live checkpoint run")
				}
				runID = run.ID
			}
		}
		if runID == uuid.Nil {
			t.Fatal("native checkpoint run missing")
		}
		zone, err := time.LoadLocation(item.view.Configuration.Timezone)
		if err != nil {
			t.Fatal(err)
		}
		var checkpoints scorecards.CheckpointPage
		if err := r.owner.call(t.Context(), http.MethodGet, base+"/scorecard-checkpoints", -1, nil, &checkpoints, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		command := scorecards.CheckpointCommand{Definition: scorecards.CheckpointDefinition{ID: uuid.New(), Name: "Independent live synthetic observation", RunID: runID, OpensWallTime: window.Start.In(zone).Format("2006-01-02T15:04:05"), ClosesWallTime: item.view.Configuration.SubmissionCutoff.In(zone).Format("2006-01-02T15:04:05")}, Reason: "Configured before live; excluded from final judge totals"}
		if os.Getenv("HACKBOX_LIFECYCLE_BROWSER_PHASES") == "ui-journey" {
			item.review.Checkpoint = r.configureMentorCheckpointInBrowser(t, item, 1902+index*20, "checkpoint", checkpoints, command)
		} else if err := r.owner.call(t.Context(), http.MethodPost, base+"/scorecard-checkpoints", checkpoints.Revision, command, &item.review.Checkpoint, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if item.review.Checkpoint.Purpose != "checkpoint" || item.review.Checkpoint.Rubric != item.rubric.Reference {
			t.Fatal("native checkpoint changed purpose or rubric")
		}
		if err := r.owner.call(t.Context(), http.MethodGet, base, -1, nil, &item.view, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		for _, actor := range []string{"mentor-a", "mentor-b"} {
			path := fmt.Sprintf("%s/teams/%s/scorecards", base, page.Published[0].TeamID)
			command := scorecards.CardCommand{Action: "save", CheckpointID: item.review.Checkpoint.Definition.ID, CheckpointVersion: item.review.Checkpoint.Version, Scores: []scorecards.CategoryScore{}}
			if err := item.cohort.staff[actor].call(t.Context(), http.MethodPost, path, item.view.Revision, command, nil, http.StatusForbidden); err != nil {
				t.Fatal(err)
			}
			item.review.EarlyWritesDenied++
		}
	}
}

func (r *lifecycleJourney) recordLiveMentorCheckpoints(t *testing.T) {
	t.Helper()
	for index := range r.events {
		item := &r.events[index]
		for assignmentIndex, assignment := range item.review.Assignments {
			actor := "mentor-a"
			if assignment.Slot == "second" {
				actor = "mentor-b"
			}
			client := item.cohort.staff[actor]
			path := fmt.Sprintf("/api/v1/events/%s/teams/%s/scorecards", item.view.ID, assignment.TeamID)
			var page scorecards.Page
			if err := client.call(t.Context(), http.MethodGet, path, -1, nil, &page, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if len(page.Items) != 0 {
				t.Fatal("live observation unexpectedly inherited another author's card")
			}
			scores := []scorecards.CategoryScore{}
			for _, category := range item.review.Checkpoint.Criteria.Categories {
				value := category.MinScore
				scores = append(scores, scorecards.CategoryScore{CategoryID: category.ID, Value: &value, Evidence: []scorecards.EvidenceReference{{Kind: "note", Note: "Independent observation of synthetic live work; this checkpoint never supplies final judge scores"}}})
			}
			command := scorecards.CardCommand{Action: "submit", CheckpointID: item.review.Checkpoint.Definition.ID, CheckpointVersion: item.review.Checkpoint.Version, Scores: scores}
			var card scorecards.Card
			if os.Getenv("HACKBOX_LIFECYCLE_BROWSER_PHASES") == "ui-journey" {
				card = r.submitMentorCheckpointInBrowser(t, item, index, assignmentIndex, actor, client, assignment.TeamID, page.Revision, command)
			} else if err := client.call(t.Context(), http.MethodPost, path, page.Revision, command, &card, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if card.AuthorID != client.person || card.TeamID != assignment.TeamID || card.Purpose != "checkpoint" || card.RunID != item.review.Checkpoint.Definition.RunID || card.State != "submitted" || card.SubmissionRevision != nil {
				t.Fatal("live checkpoint escaped its independent pre-final namespace")
			}
			item.review.Cards = append(item.review.Cards, lifecycleMentorCardReceipt{TeamID: card.TeamID, AuthorID: card.AuthorID, Reference: card.Reference, RunID: card.RunID})
		}
		if len(item.review.Cards) != 12 {
			t.Fatal("native live review omitted one independent assigned mentor")
		}
		r.persistWorkProgress(t)
	}
}
