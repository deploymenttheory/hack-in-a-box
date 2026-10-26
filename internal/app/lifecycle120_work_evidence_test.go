package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

// Preserve external fixture receipts before later native steps can fail. These
// hashes/times are bounded evidence; never serialize work bodies or sessions.
func (r *lifecycleJourney) persistWorkProgress(t *testing.T) {
	t.Helper()
	type item struct {
		PreparationAccess  []lifecycleAccessReceipt            `json:"preparation_repository_access,omitempty"`
		LiveAccess         []lifecycleAccessReceipt            `json:"live_mentor_repository_access,omitempty"`
		EventID            uuid.UUID                           `json:"event_id"`
		Work               []lifecycleWorkReceipt              `json:"preparation_work"`
		Late               *lifecycleWorkReceipt               `json:"late_work,omitempty"`
		Practice           []lifecyclePracticeReceipt          `json:"fictional_judge_practice,omitempty"`
		JudgeRun           *uuid.UUID                          `json:"planned_judge_run_id,omitempty"`
		Review             *lifecycleMentorReviewReceipt       `json:"mentor_live_checkpoint,omitempty"`
		JudgeFinal         *lifecycleJudgeFinalReceipt         `json:"independent_judge_final,omitempty"`
		ResultDecision     *lifecycleResultDecisionReceipt     `json:"result_decision,omitempty"`
		ResultReleases     []lifecycleResultReleaseReceipt     `json:"result_releases,omitempty"`
		ResultPreparations []lifecycleResultPreparationReceipt `json:"result_preparations,omitempty"`
		TeamFeedback       []lifecycleTeamFeedbackReceipt      `json:"team_feedback,omitempty"`
		LiveHelp           lifecycleHelpReceipt                `json:"live_help"`
	}
	events := []item{}
	for _, event := range r.events {
		value := item{EventID: event.view.ID, Work: event.work, Late: event.lateWork, Practice: event.practice, PreparationAccess: event.access, LiveAccess: event.liveAccess}
		value.JudgeFinal = event.judgeFinal
		value.ResultDecision = event.resultDecision
		value.ResultReleases = event.resultReleases
		value.ResultPreparations = event.resultPreparations
		value.TeamFeedback = event.teamFeedback
		value.LiveHelp = event.liveHelp
		if event.judgeRun != uuid.Nil {
			runID := event.judgeRun
			value.JudgeRun = &runID
		}
		if event.review.Checkpoint.Definition.ID != uuid.Nil {
			review := event.review
			value.Review = &review
		}
		events = append(events, value)
	}
	raw, err := json.MarshalIndent(map[string]any{"checkpoint": "connected-work-and-practice-progress-only", "complete_lifecycle": false, "events": events}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("/artifacts", "work-progress.json"), append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
