package app

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/moderation"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/google/uuid"
)

type lifecycleFinalHuddleReceipt struct {
	TeamID             uuid.UUID                  `json:"team_id"`
	ID                 uuid.UUID                  `json:"huddle_id"`
	Version            int64                      `json:"version"`
	SignoffID          uuid.UUID                  `json:"signoff_id"`
	SubmissionRevision int64                      `json:"submission_revision"`
	Binding            string                     `json:"signoff_binding"`
	Cards              []scorecards.CardReference `json:"final_card_references"`
}

// Plan the separate final-review namespace before live. It opens only at the
// actual cutoff and cannot inherit the live checkpoint's cards or scores.
func (r *lifecycleJourney) planFinalMentorReview(t *testing.T) {
	t.Helper()
	for index := range r.events {
		item := &r.events[index]
		if item.rubric.Definition.Mode != rubric.MentorQualification {
			continue
		}
		base := "/api/v1/events/" + item.view.ID.String()
		var configured rubric.Configuration
		if err := r.owner.call(t.Context(), http.MethodGet, base+"/rubric/configuration", -1, nil, &configured, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if os.Getenv("HACKBOX_LIFECYCLE_BROWSER_PHASES") == "ui-journey" {
			configured = r.createMentorRunInBrowser(t, item, 1941, "mentor_final", "Plan fresh frozen-output mentor qualification, excluded from judge totals", configured)
		} else if err := r.owner.call(t.Context(), http.MethodPost, base+"/rubric/configuration", configured.Revision, map[string]any{"action": "create_run", "version": configured.Reference.Version, "purpose": "mentor_final", "reason": "Plan fresh frozen-output mentor qualification, excluded from judge totals"}, &configured, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		runID := uuid.Nil
		for _, run := range configured.Runs {
			if run.Purpose == "mentor_final" {
				if runID != uuid.Nil || run.StartedAt != nil {
					t.Fatal("final mentor run duplicated or started before live")
				}
				runID = run.ID
			}
		}
		if runID == uuid.Nil || runID == item.review.Checkpoint.Definition.RunID {
			t.Fatal("final mentor run reused live checkpoint")
		}
		zone, err := time.LoadLocation(item.view.Configuration.Timezone)
		if err != nil {
			t.Fatal(err)
		}
		var page scorecards.CheckpointPage
		if err := r.owner.call(t.Context(), http.MethodGet, base+"/scorecard-checkpoints", -1, nil, &page, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		command := scorecards.CheckpointCommand{Definition: scorecards.CheckpointDefinition{ID: uuid.New(), Name: "Fresh frozen-output mentor qualification", RunID: runID, OpensWallTime: item.view.Configuration.SubmissionCutoff.In(zone).Format("2006-01-02T15:04:05"), ClosesWallTime: item.view.Configuration.LiveEnd.Add(time.Hour).In(zone).Format("2006-01-02T15:04:05")}, Reason: "Independent final evidence review after cutoff, no live score carry"}
		var checkpoint scorecards.Checkpoint
		if os.Getenv("HACKBOX_LIFECYCLE_BROWSER_PHASES") == "ui-journey" {
			checkpoint = r.configureMentorCheckpointInBrowser(t, item, 1942, "mentor_final", page, command)
		} else if err := r.owner.call(t.Context(), http.MethodPost, base+"/scorecard-checkpoints", page.Revision, command, &checkpoint, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if checkpoint.Purpose != "mentor_final" || checkpoint.Rubric != item.rubric.Reference || checkpoint.Definition.ID == item.review.Checkpoint.Definition.ID {
			t.Fatal("final mentor checkpoint lost its approved independent namespace")
		}
		item.review.FinalCheckpoint = &checkpoint
		r.persistWorkProgress(t)
	}
}

func (r *lifecycleJourney) reviewFrozenMentorEvidence(t *testing.T) {
	t.Helper()
	for index := range r.events {
		item := &r.events[index]
		checkpoint := item.review.FinalCheckpoint
		if item.rubric.Definition.Mode != rubric.MentorQualification {
			if checkpoint != nil {
				t.Fatal("all-team judging acquired mentor qualification requirement")
			}
			continue
		}
		if checkpoint == nil {
			t.Fatal("final mentor review was not planned before live")
		}
		for _, source := range item.cutoff {
			if source.Outcome != "frozen" {
				continue
			}
			if source.SubmissionRevision == nil {
				t.Fatal("frozen source has no exact formal revision")
			}
			path := fmt.Sprintf("/api/v1/events/%s/teams/%s/scorecards", item.view.ID, source.TeamID)
			refs := []scorecards.CardReference{}
			for _, actor := range []string{"mentor-a", "mentor-b"} {
				client := item.cohort.staff[actor]
				var page scorecards.Page
				if err := client.call(t.Context(), http.MethodGet, path, -1, nil, &page, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				for _, prior := range page.Items {
					if prior.AuthorID != client.person {
						t.Fatal("independent final reviewer saw another author's private card")
					}
					if prior.CheckpointID == checkpoint.Definition.ID {
						t.Fatal("fresh final review inherited a final card")
					}
				}
				scores := []scorecards.CategoryScore{}
				for _, category := range checkpoint.Criteria.Categories {
					value := category.MaxScore
					scores = append(scores, scorecards.CategoryScore{CategoryID: category.ID, Value: &value, Evidence: []scorecards.EvidenceReference{{Kind: "note", Note: "Fresh independent review of this exact frozen synthetic final; live checkpoint excluded"}}})
				}
				var card scorecards.Card
				command := scorecards.CardCommand{Action: "submit", CheckpointID: checkpoint.Definition.ID, CheckpointVersion: checkpoint.Version, Scores: scores}
				if os.Getenv("HACKBOX_LIFECYCLE_BROWSER_PHASES") == "ui-journey" {
					card = r.submitIndependentMentorCardInBrowser(t, item, 1801+len(item.review.FinalCards), client, page, lifecycleBrowserMentorCardInput{Actor: actor, TeamID: source.TeamID, AuthorID: client.person, Revision: page.Revision, Checkpoint: *checkpoint, Command: command, FrozenRevision: source.SubmissionRevision})
					item.browserFinalMentorCards = append(item.browserFinalMentorCards, lifecycleMentorCardReceipt{TeamID: card.TeamID, AuthorID: card.AuthorID, Reference: card.Reference, RunID: card.RunID})
				} else if err := client.call(t.Context(), http.MethodPost, path, page.Revision, command, &card, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				if card.Purpose != "mentor_final" || card.RunID != checkpoint.Definition.RunID || card.AuthorID != client.person || card.TeamID != source.TeamID || card.SubmissionRevision == nil || *card.SubmissionRevision != *source.SubmissionRevision || card.Reference.Version != 1 || card.State != "submitted" {
					t.Fatal("fresh final mentor card lost exact author/run/frozen revision")
				}
				for _, live := range item.review.Cards {
					if live.Reference.ID == card.Reference.ID || live.RunID == card.RunID {
						t.Fatal("live checkpoint accumulated into final mentor evidence")
					}
				}
				refs = append(refs, card.Reference)
				item.review.FinalCards = append(item.review.FinalCards, lifecycleMentorCardReceipt{TeamID: card.TeamID, AuthorID: card.AuthorID, Reference: card.Reference, RunID: card.RunID})
			}
			id := uuid.New()
			huddlePath := fmt.Sprintf("/api/v1/events/%s/teams/%s/moderation-huddles/%s", item.view.ID, source.TeamID, id)
			var current event.View
			if err := r.owner.call(t.Context(), http.MethodGet, "/api/v1/events/"+item.view.ID.String(), -1, nil, &current, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			var huddle moderation.Huddle
			command := map[string]any{"action": "configure", "version": 0, "reason": "Record exact independent frozen-final review context", "definition": moderation.Definition{ID: id, CheckpointID: checkpoint.Definition.ID, CheckpointVersion: checkpoint.Version, ModeratorID: r.owner.person, SecondReviewerID: item.cohort.staff["mentor-b"].person, ReviewDueAt: checkpoint.ClosesAt.Add(-time.Minute)}}
			if os.Getenv("HACKBOX_LIFECYCLE_BROWSER_PHASES") == "ui-journey" {
				huddle = r.configureFinalHuddleInBrowser(t, item, len(item.review.FinalHuddles), source.TeamID, current.Revision, command["definition"].(moderation.Definition), command["reason"].(string), refs)
				id = huddle.Definition.ID
				huddlePath = fmt.Sprintf("/api/v1/events/%s/teams/%s/moderation-huddles/%s", item.view.ID, source.TeamID, id)
			} else if err := r.owner.call(t.Context(), http.MethodPut, huddlePath, current.Revision, command, &huddle, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			for actionIndex, step := range []struct {
				client *lifecycleClient
				action string
			}{{item.cohort.staff["mentor-b"], "complete_second_review"}, {r.owner, "open"}, {r.owner, "sign_off"}} {
				if err := r.owner.call(t.Context(), http.MethodGet, "/api/v1/events/"+item.view.ID.String(), -1, nil, &current, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				if os.Getenv("HACKBOX_LIFECYCLE_BROWSER_PHASES") == "ui-journey" {
					huddle = r.decideFinalHuddleInBrowser(t, item, len(item.review.FinalHuddles), actionIndex, step.client, current.Revision, huddle, step.action, "Recorded current independent final review and exact source", refs, *source.SubmissionRevision)
				} else if err := step.client.call(t.Context(), http.MethodPut, huddlePath, current.Revision, map[string]any{"action": step.action, "version": huddle.Version, "reason": "Recorded current independent final review and exact source"}, &huddle, http.StatusOK); err != nil {
					t.Fatal(err)
				}
			}
			if huddle.SignoffID == nil || huddle.SignoffContext == nil || huddle.SignoffContext.Binding == "" || huddle.SubmissionRevision == nil || *huddle.SubmissionRevision != *source.SubmissionRevision || len(huddle.Cards) != 2 || huddle.Review.CompletedAt == nil {
				t.Fatal("native huddle lacks current signed independent frozen source")
			}
			expected := map[scorecards.CardReference]bool{}
			for _, ref := range refs {
				expected[ref] = true
			}
			for _, ref := range huddle.Cards {
				if !expected[ref] {
					t.Fatal("native huddle substituted a final card reference")
				}
				delete(expected, ref)
			}
			if len(expected) != 0 {
				t.Fatal("native signed huddle omitted an independent final author")
			}
			item.review.FinalHuddles = append(item.review.FinalHuddles, lifecycleFinalHuddleReceipt{TeamID: source.TeamID, ID: id, Version: huddle.Version, SignoffID: *huddle.SignoffID, SubmissionRevision: *source.SubmissionRevision, Binding: huddle.SignoffContext.Binding, Cards: huddle.Cards})
			r.persistWorkProgress(t)
		}
		if len(item.review.FinalCards) != 8 || len(item.review.FinalHuddles) != 4 {
			t.Fatal("final mentor review omitted a genuine frozen entry")
		}
	}
}
