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
		EventID  uuid.UUID                     `json:"event_id"`
		Work     []lifecycleWorkReceipt        `json:"preparation_work"`
		Late     *lifecycleWorkReceipt         `json:"late_work,omitempty"`
		Practice []lifecyclePracticeReceipt    `json:"fictional_judge_practice,omitempty"`
		JudgeRun *uuid.UUID                    `json:"planned_judge_run_id,omitempty"`
		Review   *lifecycleMentorReviewReceipt `json:"mentor_live_checkpoint,omitempty"`
	}
	events := []item{}
	for _, event := range r.events {
		value := item{EventID: event.view.ID, Work: event.work, Late: event.lateWork, Practice: event.practice}
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
