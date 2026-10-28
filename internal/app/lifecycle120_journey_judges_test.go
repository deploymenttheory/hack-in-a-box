package app

import (
	"fmt"
	"net/http"
	"os"
	"reflect"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/google/uuid"
)

type lifecycleJudgeFinalReceipt struct {
	RunID                          uuid.UUID                   `json:"run_id"`
	EligibleTeamIDs                []uuid.UUID                 `json:"eligible_team_ids"`
	ReserveReadsDenied             int                         `json:"reserve_reads_denied"`
	MentorReadDenied               bool                        `json:"mentor_real_card_read_denied"`
	MissingJudgeDeliberationDenied bool                        `json:"missing_judge_deliberation_denied"`
	DeliberationOpened             bool                        `json:"deliberation_opened"`
	Cards                          []lifecycleJudgeCardReceipt `json:"cards"`
	OpenAcknowledgement            *judging.Receipt            `json:"open_acknowledgement,omitempty"`
}

type lifecycleJudgeCardReceipt struct {
	Acknowledgement    judging.Receipt        `json:"acknowledgement"`
	SubmissionRevision int64                  `json:"submission_revision"`
	Repository         judging.FreezeEvidence `json:"repository"`
}

func (r *lifecycleJourney) scoreIndependentJudgeFinals(t *testing.T) {
	t.Helper()
	for index := range r.events {
		item := &r.events[index]
		base := "/api/v1/events/" + item.view.ID.String()
		path := fmt.Sprintf("%s/judging/%s", base, item.judgeRun)
		receipt := &lifecycleJudgeFinalReceipt{RunID: item.judgeRun}
		item.judgeFinal = receipt
		if item.rubric.Definition.Mode == rubric.MentorQualification {
			selected := item.review.Qualification
			if selected == nil || selected.SignoffBinding == "" {
				t.Fatal("fresh judging lacks actual signed finalist selection")
			}
			receipt.EligibleTeamIDs = append([]uuid.UUID{}, selected.FinalistIDs...)
			for _, id := range selected.ReserveIDs {
				if err := item.cohort.staff["judge-a"].call(t.Context(), http.MethodGet, path+"/teams/"+id.String(), -1, nil, nil, http.StatusForbidden); err != nil {
					t.Fatal(err)
				}
				receipt.ReserveReadsDenied++
			}
		} else {
			for _, source := range item.cutoff {
				if source.Outcome == "frozen" {
					receipt.EligibleTeamIDs = append(receipt.EligibleTeamIDs, source.TeamID)
				}
			}
		}
		expected := 4
		if item.rubric.Definition.Mode == rubric.MentorQualification {
			expected = 3
		}
		if len(receipt.EligibleTeamIDs) != expected {
			t.Fatal("fresh judge cohort differs from native frozen/finalist source")
		}
		var configured rubric.Configuration
		if err := r.owner.call(t.Context(), http.MethodGet, base+"/rubric/configuration", -1, nil, &configured, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		unstarted := false
		for _, run := range configured.Runs {
			if run.ID == item.judgeRun {
				unstarted = run.Purpose == "judge_final" && run.StartedAt == nil
			}
		}
		if !unstarted {
			t.Fatal("practice or mentor qualification started independent judge assessment")
		}
		if err := item.cohort.staff["mentor-a"].call(t.Context(), http.MethodGet, path+"/teams/"+receipt.EligibleTeamIDs[0].String(), -1, nil, nil, http.StatusForbidden); err != nil {
			t.Fatal(err)
		}
		receipt.MentorReadDenied = true
		for judgeIndex, actor := range []string{"judge-a", "judge-b", "judge-c"} {
			client := item.cohort.staff[actor]
			for _, teamID := range receipt.EligibleTeamIDs {
				cardPath := path + "/teams/" + teamID.String()
				var brief judging.Brief
				if err := client.call(t.Context(), http.MethodGet, cardPath, -1, nil, &brief, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				if brief.OwnCard != nil || brief.Deliberation || len(brief.SubmittedCards) != 0 || brief.Reference != item.rubric.Reference || brief.TeamID != teamID || brief.RunID != item.judgeRun || brief.SubmissionRevision != 1 || brief.PrivateJudgeGuidance != item.rubric.Definition.PrivateJudgeGuidance {
					t.Fatal("fresh judge brief inherited scores or disclosed another independent card")
				}
				scores := []judging.Score{}
				values := map[uuid.UUID]int{}
				for _, category := range brief.Criteria.Categories {
					value := category.MinScore + 1
					if value >= category.MaxScore {
						t.Fatal("synthetic fresh score must differ from live/final mentor extrema")
					}
					values[category.ID] = value
					scores = append(scores, judging.Score{CategoryID: category.ID, Value: &value, Evidence: []judging.Evidence{{Note: "Fresh independent judge review of exact frozen final; no mentor or practice score carry"}}})
				}
				id := uuid.New()
				body := map[string]any{"request_id": id, "action": "submit", "submission_revision": brief.SubmissionRevision, "scores": scores, "reason": "Independent final judge submission against current frozen evidence"}
				if brief.Repository.DecisionID != nil {
					body["repository_decision_id"] = *brief.Repository.DecisionID
				}
				var accepted judging.Receipt
				if err := client.call(t.Context(), http.MethodPost, cardPath, 0, body, &accepted, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				if accepted.RequestID != id || accepted.ActorID != client.person || accepted.RunID != item.judgeRun || accepted.TeamID != teamID || accepted.Action != "submit" || accepted.Revision != 1 || accepted.State != "submitted" || accepted.Fingerprint == "" {
					t.Fatal("independent judge submission lost actor/source/version acknowledgement")
				}
				var recovered judging.Recovery
				if err := client.call(t.Context(), http.MethodGet, base+"/judging-receipts/"+id.String(), -1, nil, &recovered, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				if recovered.Receipt == nil || *recovered.Receipt != accepted {
					t.Fatal("read-only judge acknowledgement recovery changed original outcome")
				}
				var current judging.Brief
				if err := client.call(t.Context(), http.MethodGet, cardPath, -1, nil, &current, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				card := current.OwnCard
				if card == nil || card.AuthorID != client.person || card.RunID != item.judgeRun || card.TeamID != teamID || card.Version != 1 || card.State != "submitted" || card.SubmissionRevision != brief.SubmissionRevision || card.Rubric != brief.Reference || !reflect.DeepEqual(card.Repository, brief.Repository) || len(card.Scores) != len(scores) || len(current.SubmittedCards) != 0 || current.Deliberation {
					t.Fatal("native private final card changed author/frozen source or disclosed peers")
				}
				for _, score := range card.Scores {
					if score.Value == nil || *score.Value != values[score.CategoryID] {
						t.Fatal("judge score inherited practice, live or qualification values")
					}
				}
				receipt.Cards = append(receipt.Cards, lifecycleJudgeCardReceipt{Acknowledgement: accepted, SubmissionRevision: card.SubmissionRevision, Repository: card.Repository})
				r.persistWorkProgress(t)
			}
			if judgeIndex == 1 {
				var before, after judging.Round
				if err := r.owner.call(t.Context(), http.MethodGet, path, -1, nil, &before, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				if before.RequiredJudges != 3 || before.Deliberation {
					t.Fatal("native round lost all-required-three-judge gate")
				}
				if os.Getenv("HACKBOX_LIFECYCLE_BROWSER_PHASES") == "ui-journey" {
					r.deliberateInBrowser(t, item, index, before, "Negative proof: third required judge has not submitted", http.StatusConflict)
				} else if err := r.owner.call(t.Context(), http.MethodPost, path, before.Revision, map[string]any{"request_id": uuid.New(), "action": "open_deliberation", "reason": "Negative proof: third required judge has not submitted"}, nil, http.StatusConflict); err != nil {
					t.Fatal(err)
				}
				if err := r.owner.call(t.Context(), http.MethodGet, path, -1, nil, &after, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				if after.Revision != before.Revision || after.Deliberation {
					t.Fatal("missing-judge denial mutated deliberation")
				}
				receipt.MissingJudgeDeliberationDenied = true
			}
		}
		var round judging.Round
		if err := r.owner.call(t.Context(), http.MethodGet, path, -1, nil, &round, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		var opened judging.Receipt
		id := uuid.New()
		if os.Getenv("HACKBOX_LIFECYCLE_BROWSER_PHASES") == "ui-journey" {
			opened = r.deliberateInBrowser(t, item, index, round, "All three fresh required judge cards complete for every eligible final", http.StatusOK)
			id = opened.RequestID
		} else if err := r.owner.call(t.Context(), http.MethodPost, path, round.Revision, map[string]any{"request_id": id, "action": "open_deliberation", "reason": "All three fresh required judge cards complete for every eligible final"}, &opened, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if opened.RequestID != id || opened.ActorID != r.owner.person || opened.RunID != item.judgeRun || opened.Action != "open_deliberation" {
			t.Fatal("native deliberation acknowledged another actor or run")
		}
		if err := r.owner.call(t.Context(), http.MethodGet, path, -1, nil, &round, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if !round.Deliberation || len(receipt.Cards) != expected*3 {
			t.Fatal("actual completed native final did not open its intended deliberation")
		}
		for _, teamID := range receipt.EligibleTeamIDs {
			var brief judging.Brief
			if err := item.cohort.staff["judge-a"].call(t.Context(), http.MethodGet, path+"/teams/"+teamID.String(), -1, nil, &brief, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			judges := map[uuid.UUID]bool{}
			for _, card := range brief.SubmittedCards {
				if judges[card.AuthorID] || card.RunID != item.judgeRun || card.TeamID != teamID || card.State != "submitted" {
					t.Fatal("deliberation substituted or duplicated an independent judge")
				}
				judges[card.AuthorID] = true
			}
			if !brief.Deliberation || len(judges) != 3 {
				t.Fatal("explicit deliberation omitted a required current submitted card")
			}
		}
		receipt.DeliberationOpened = true
		receipt.OpenAcknowledgement = &opened
		r.persistWorkProgress(t)
	}
}
