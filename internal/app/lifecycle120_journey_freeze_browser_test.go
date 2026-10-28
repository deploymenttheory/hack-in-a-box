package app

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/submissionfreeze"
	"github.com/google/uuid"
)

type lifecycleBrowserFreezeInput struct {
	TeamID  uuid.UUID                `json:"team_id"`
	Before  submissionfreeze.Page    `json:"before"`
	Command submissionfreeze.Command `json:"command"`
}

func (r *lifecycleJourney) changeFreezeInBrowser(t *testing.T, item *lifecycleJourneyEvent, eventIndex int, teamID uuid.UUID, before submissionfreeze.Page, command submissionfreeze.Command) submissionfreeze.Receipt {
	t.Helper()
	sequence := 1301 + eventIndex*20 + len(item.browserBranchTeams)
	if command.Action == "reconcile" {
		sequence = 1401 + eventIndex*20 + len(item.browserReconciledTeams)
	} else if command.Action != "configure" {
		t.Fatal("unsupported UI cutoff action")
	}
	// The production review creates its own request identity. Never seed it.
	command.RequestID = uuid.Nil
	ack := r.admissionBrowserPhase(t, item, lifecycleBrowserPhase{Sequence: sequence, Action: "freeze-" + command.Action, Freeze: &lifecycleBrowserFreezeInput{TeamID: teamID, Before: before, Command: command}})
	if !ack.Saved || ack.CommandID == nil || *ack.CommandID == uuid.Nil {
		t.Fatal("cutoff action lacks an actual UI-generated request identity")
	}
	var recovery submissionfreeze.Recovery
	path := fmt.Sprintf("/api/v1/events/%s/teams/%s/submission-freeze/commands/%s", item.view.ID, teamID, *ack.CommandID)
	if err := r.owner.call(t.Context(), http.MethodGet, path, -1, nil, &recovery, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if recovery.Receipt == nil || recovery.Receipt.RequestID != *ack.CommandID || recovery.Receipt.Action != command.Action || recovery.Receipt.ExpectedRevision != before.Revision || recovery.Receipt.Revision != before.Revision+1 {
		t.Fatal("UI cutoff action lacks its exact immutable native acknowledgement")
	}
	receipt := *recovery.Receipt
	if command.Action == "configure" {
		if receipt.Branch != command.Branch {
			t.Fatal("UI changed the reviewed submission branch")
		}
		item.browserBranchTeams = append(item.browserBranchTeams, teamID)
	} else {
		decision := receipt.Decision
		if decision == nil || decision.SourceID != command.SourceID || decision.SHA != command.SHA || decision.NativeRevision != command.NativeRevision || decision.SubmissionRevision != command.SubmissionRevision || decision.Reason != command.Reason || decision.ExternalEvidence != command.ExternalEvidence || !reflect.DeepEqual(decision.ObservationIDs, command.ObservationIDs) || !reflect.DeepEqual(decision.PushIDs, command.PushIDs) {
			t.Fatal("UI changed the exact reviewed native cutoff decision")
		}
		item.browserReconciledTeams = append(item.browserReconciledTeams, teamID)
	}
	return receipt
}

func lifecycleBrowserFreezeComplete(item lifecycleJourneyEvent) bool {
	if len(item.browserBranchTeams) != 6 || len(item.browserReconciledTeams) != 3 {
		return false
	}
	for i, work := range item.work {
		if item.browserBranchTeams[i] != work.TeamID {
			return false
		}
	}
	var reconciled []uuid.UUID
	for _, receipt := range item.cutoff {
		if receipt.SelectedSHA != "" {
			reconciled = append(reconciled, receipt.TeamID)
		}
	}
	return reflect.DeepEqual(reconciled, item.browserReconciledTeams)
}
