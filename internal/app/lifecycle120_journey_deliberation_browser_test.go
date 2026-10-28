package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/google/uuid"
)

type lifecycleBrowserDeliberationInput struct {
	Round          judging.Round `json:"round"`
	Reason         string        `json:"reason"`
	ExpectedStatus int           `json:"expected_status"`
	ActorID        uuid.UUID     `json:"actor_id"`
}
type lifecycleDeliberationBrowserReceipt struct {
	RequestID        uuid.UUID `json:"request_id"`
	RunID            uuid.UUID `json:"run_id"`
	ExpectedRevision int64     `json:"expected_revision"`
	Accepted         bool      `json:"accepted"`
}

func (r *lifecycleJourney) deliberateInBrowser(t *testing.T, item *lifecycleJourneyEvent, eventIndex int, before judging.Round, reason string, status int) judging.Receipt {
	t.Helper()
	sequence := 2201 + eventIndex*20
	if status == http.StatusOK {
		sequence++
	}
	ack := r.admissionBrowserPhase(t, item, lifecycleBrowserPhase{Sequence: sequence, Action: "open-deliberation", Deliberation: &lifecycleBrowserDeliberationInput{Round: before, Reason: reason, ExpectedStatus: status, ActorID: r.owner.person}})
	accepted := status == http.StatusOK
	if ack.CommandID == nil || *ack.CommandID == uuid.Nil || ack.Saved != accepted || !ack.Completed {
		t.Fatal("deliberation UI did not retain its actual original attempt and outcome")
	}
	path := fmt.Sprintf("/api/v1/events/%s/judging/%s", item.view.ID, item.judgeRun)
	var after judging.Round
	if err := r.owner.call(t.Context(), http.MethodGet, path, -1, nil, &after, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	var receipt judging.Receipt
	if !accepted {
		if status != http.StatusConflict || !reflect.DeepEqual(before, after) {
			t.Fatal("missing-judge UI rejection mutated native deliberation")
		}
	} else {
		if after.RunID != before.RunID || after.Revision != before.Revision+1 || !after.Deliberation || !reflect.DeepEqual(after.Assignments, before.Assignments) || after.RequiredJudges != before.RequiredJudges {
			t.Fatal("deliberation UI changed native round identity or independent coverage")
		}
		var original judging.Recovery
		if err := r.owner.call(t.Context(), http.MethodGet, fmt.Sprintf("/api/v1/events/%s/judging-receipts/%s", item.view.ID, *ack.CommandID), -1, nil, &original, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(struct {
			Run     uuid.UUID
			Version int64
			Command judging.RoundCommand
		}{before.RunID, before.Revision, judging.RoundCommand{RequestID: *ack.CommandID, Action: "open_deliberation", Reason: reason}})
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		if original.Receipt == nil {
			t.Fatal("deliberation lacks its original native acknowledgement")
		}
		receipt = *original.Receipt
		if receipt.RequestID != *ack.CommandID || receipt.ActorID != r.owner.person || receipt.RunID != before.RunID || receipt.TeamID != uuid.Nil || receipt.Action != "open_deliberation" || receipt.Revision != before.Revision+1 || receipt.State != "recorded" || receipt.Fingerprint != hex.EncodeToString(sum[:]) {
			t.Fatal("deliberation original receipt lost exact actor/action/source/fingerprint")
		}
	}
	item.browserDeliberation = append(item.browserDeliberation, lifecycleDeliberationBrowserReceipt{RequestID: *ack.CommandID, RunID: before.RunID, ExpectedRevision: before.Revision, Accepted: accepted})
	return receipt
}

func lifecycleBrowserDeliberationComplete(item lifecycleJourneyEvent) bool {
	return len(item.browserDeliberation) == 2 && !item.browserDeliberation[0].Accepted && item.browserDeliberation[1].Accepted && item.browserDeliberation[0].RunID == item.judgeRun && item.browserDeliberation[1].RunID == item.judgeRun && item.judgeFinal != nil && item.judgeFinal.OpenAcknowledgement != nil && item.browserDeliberation[1].RequestID == item.judgeFinal.OpenAcknowledgement.RequestID
}
