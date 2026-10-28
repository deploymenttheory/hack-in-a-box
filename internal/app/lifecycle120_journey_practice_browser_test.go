package app

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/google/uuid"
)

type lifecycleBrowserPracticeInput struct {
	Actor  string           `json:"actor"`
	Before judging.Practice `json:"before"`
	Scores []judging.Score  `json:"scores"`
}

func (r *lifecycleJourney) practiceInBrowser(t *testing.T, item *lifecycleJourneyEvent, eventIndex, judgeIndex int, judge *lifecycleClient, before judging.Practice, scores []judging.Score) judging.Receipt {
	t.Helper()
	alias := []string{"judge-a", "judge-b", "judge-c"}[judgeIndex]
	if item.cohort.staff[alias] != judge {
		t.Fatal("practice actor differs from actual selected normal-OIDC judge")
	}
	ack := r.admissionBrowserPhase(t, item, lifecycleBrowserPhase{Sequence: 1501 + eventIndex*20 + judgeIndex, Action: "submit-fictional-practice", Practice: &lifecycleBrowserPracticeInput{Actor: alias, Before: before, Scores: scores}})
	if !ack.Saved || ack.CommandID == nil || *ack.CommandID == uuid.Nil {
		t.Fatal("practice lacks its actual UI-generated original command identity")
	}
	var recovery struct {
		Receipt *judging.Receipt `json:"receipt"`
	}
	path := fmt.Sprintf("/api/v1/events/%s/judging-receipts/%s", item.view.ID, *ack.CommandID)
	if err := judge.call(t.Context(), http.MethodGet, path, -1, nil, &recovery, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if recovery.Receipt == nil || recovery.Receipt.RequestID != *ack.CommandID {
		t.Fatal("practice lacks its immutable native original acknowledgement")
	}
	item.browserPracticeJudges = append(item.browserPracticeJudges, judge.person)
	return *recovery.Receipt
}

func lifecycleBrowserPracticeComplete(item lifecycleJourneyEvent) bool {
	if len(item.practice) != 3 || len(item.browserPracticeJudges) != 3 {
		return false
	}
	for i, receipt := range item.practice {
		if item.browserPracticeJudges[i] != receipt.JudgeID {
			return false
		}
	}
	return true
}
