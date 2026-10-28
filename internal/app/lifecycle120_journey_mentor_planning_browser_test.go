package app

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/google/uuid"
)

type lifecycleBrowserMentorWindow struct {
	OpensAt  time.Time `json:"opens_at"`
	ClosesAt time.Time `json:"closes_at"`
	Timezone string    `json:"timezone"`
}
type lifecycleBrowserMentorPlanningInput struct {
	Window        *lifecycleBrowserMentorWindow `json:"window,omitempty"`
	Purpose       string                        `json:"purpose"`
	Reason        string                        `json:"reason"`
	Configuration *rubric.Configuration         `json:"configuration,omitempty"`
	Revision      int64                         `json:"revision"`
	Command       *scorecards.CheckpointCommand `json:"command,omitempty"`
}
type lifecycleMentorPlanningReceipt struct {
	Action       string     `json:"action"`
	Purpose      string     `json:"purpose"`
	RunID        uuid.UUID  `json:"run_id"`
	CheckpointID *uuid.UUID `json:"checkpoint_id,omitempty"`
	Version      int64      `json:"version"`
}

func (r *lifecycleJourney) createMentorRunInBrowser(t *testing.T, item *lifecycleJourneyEvent, sequence int, purpose, reason string, before rubric.Configuration) rubric.Configuration {
	t.Helper()
	safe := before
	safe.Definition.PrivateJudgeGuidance = ""
	ack := r.admissionBrowserPhase(t, item, lifecycleBrowserPhase{Sequence: sequence, Action: "create-mentor-assessment", MentorPlanning: &lifecycleBrowserMentorPlanningInput{Purpose: purpose, Reason: reason, Configuration: &safe}})
	if !ack.Saved || ack.RunID == nil || *ack.RunID == uuid.Nil || ack.CommandID != nil {
		t.Fatal("mentor assessment lacks its actual server-created run identity")
	}
	var current rubric.Configuration
	if err := r.owner.call(t.Context(), http.MethodGet, "/api/v1/events/"+item.view.ID.String()+"/rubric/configuration", -1, nil, &current, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if !lifecycleCreatedAssessmentRun(before, current, *ack.RunID, purpose) {
		t.Fatal("browser mentor run changed its exact new identity, approved source, purpose or retained runs")
	}

	item.browserMentorPlanning = append(item.browserMentorPlanning, lifecycleMentorPlanningReceipt{Action: "create_run", Purpose: purpose, RunID: *ack.RunID, Version: before.Reference.Version})
	return current
}

func (r *lifecycleJourney) configureMentorCheckpointInBrowser(t *testing.T, item *lifecycleJourneyEvent, sequence int, purpose string, before scorecards.CheckpointPage, expected scorecards.CheckpointCommand) scorecards.Checkpoint {
	t.Helper()
	window := lifecycleBrowserMentorWindow{OpensAt: item.view.Configuration.LiveStart, ClosesAt: item.view.Configuration.SubmissionCutoff, Timezone: item.view.Configuration.Timezone}
	if purpose == "mentor_final" {
		window.OpensAt = item.view.Configuration.SubmissionCutoff
		window.ClosesAt = item.view.Configuration.LiveEnd.Add(time.Hour)
	}
	safe := expected
	safe.Definition.ID = uuid.Nil // The production UI mints the actual checkpoint ID.
	ack := r.admissionBrowserPhase(t, item, lifecycleBrowserPhase{Sequence: sequence, Action: "configure-mentor-checkpoint", MentorPlanning: &lifecycleBrowserMentorPlanningInput{Purpose: purpose, Revision: before.Revision, Command: &safe, Window: &window}})
	if !ack.Saved || ack.CheckpointID == nil || *ack.CheckpointID == uuid.Nil || ack.CommandID != nil {
		t.Fatal("mentor checkpoint lacks its actual UI-created resource identity")
	}
	expected.Definition.ID = *ack.CheckpointID
	var current scorecards.CheckpointPage
	if err := r.owner.call(t.Context(), http.MethodGet, "/api/v1/events/"+item.view.ID.String()+"/scorecard-checkpoints?limit=100", -1, nil, &current, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if current.Revision != before.Revision+1 || len(current.Items) != len(before.Items)+1 {
		t.Fatal("mentor checkpoint planning changed an unexpected event revision or checkpoint census")
	}
	for _, old := range before.Items {
		found := false
		for _, checkpoint := range current.Items {
			if reflect.DeepEqual(checkpoint, old) {
				found = true
			}
		}
		if !found {
			t.Fatal("mentor checkpoint planning mutated a previous independent window")
		}
	}
	var found *scorecards.Checkpoint
	for _, checkpoint := range current.Items {
		if checkpoint.Definition.ID == *ack.CheckpointID {
			copy := checkpoint
			found = &copy
		}
	}
	if found == nil || !reflect.DeepEqual(found.Definition, expected.Definition) || found.Version != expected.Version+1 || found.Purpose != purpose || found.Rubric != item.rubric.Reference || !reflect.DeepEqual(found.Criteria, item.rubric.Definition.PublicDefinition) {
		t.Fatal("browser checkpoint lost its exact native definition, version, rubric or public criteria")
	}
	if !found.OpensAt.Equal(window.OpensAt) || !found.ClosesAt.Equal(window.ClosesAt) {
		t.Fatal("native checkpoint window did not resolve to its exact known event instants")
	}
	if !found.ClosesAt.After(found.OpensAt) || found.Timezone != item.view.Configuration.Timezone {
		t.Fatal("browser checkpoint has an invalid native resolved window")
	}
	if purpose == "checkpoint" && (found.OpensAt.Before(item.view.Configuration.LiveStart) || found.ClosesAt.After(item.view.Configuration.SubmissionCutoff)) {
		t.Fatal("live checkpoint escaped native live/cutoff bounds")
	}
	if purpose == "mentor_final" && found.OpensAt.Before(item.view.Configuration.SubmissionCutoff) {
		t.Fatal("final qualification opened before the actual cutoff")
	}
	item.browserMentorPlanning = append(item.browserMentorPlanning, lifecycleMentorPlanningReceipt{Action: "configure_checkpoint", Purpose: purpose, RunID: found.Definition.RunID, CheckpointID: ack.CheckpointID, Version: found.Version})
	return *found
}

func lifecycleBrowserMentorPlanningComplete(item lifecycleJourneyEvent) bool {
	expected := 2
	if item.rubric.Definition.Mode == rubric.MentorQualification {
		expected = 4
	}
	if len(item.browserMentorPlanning) != expected {
		return false
	}
	for index, receipt := range item.browserMentorPlanning {
		checkpoint := &item.review.Checkpoint
		if index >= 2 {
			checkpoint = item.review.FinalCheckpoint
		}
		if checkpoint == nil || receipt.Purpose != checkpoint.Purpose || receipt.RunID != checkpoint.Definition.RunID {
			return false
		}
		if index%2 == 0 {
			if receipt.Action != "create_run" || receipt.CheckpointID != nil || receipt.Version != checkpoint.Rubric.Version {
				return false
			}
		} else if receipt.Action != "configure_checkpoint" || receipt.CheckpointID == nil || *receipt.CheckpointID != checkpoint.Definition.ID || receipt.Version != checkpoint.Version {
			return false
		}
	}
	return true
}

// Independent native verification shared by judge and mentor planning. The
// browser's original response still supplies the actual newly created ID.
func lifecycleCreatedAssessmentRun(before, after rubric.Configuration, id uuid.UUID, purpose string) bool {
	if after.Revision != before.Revision+1 || after.Reference != before.Reference || len(after.Runs) != len(before.Runs)+1 {
		return false
	}
	for _, old := range before.Runs {
		if old.ID == id {
			return false
		}
		found := false
		for _, run := range after.Runs {
			if reflect.DeepEqual(run, old) {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	for _, run := range after.Runs {
		if run.ID == id {
			return run.Purpose == purpose && run.Reference == before.Reference && run.StartedAt == nil && run.PreviousID == nil
		}
	}
	return false
}
