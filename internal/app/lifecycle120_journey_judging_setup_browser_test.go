package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/google/uuid"
)

type lifecycleBrowserSetupJudge struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}
type lifecycleBrowserJudgingSetupInput struct {
	Configuration *rubric.Configuration        `json:"configuration,omitempty"`
	Round         *judging.Round               `json:"round,omitempty"`
	TeamID        uuid.UUID                    `json:"team_id"`
	TeamName      string                       `json:"team_name"`
	Judges        []lifecycleBrowserSetupJudge `json:"judges,omitempty"`
	Reason        string                       `json:"reason"`
}

func (r *lifecycleJourney) createFinalRoundInBrowser(t *testing.T, item *lifecycleJourneyEvent, index int, before rubric.Configuration) rubric.Configuration {
	t.Helper()
	// Only approved public criteria and immutable references cross fixture IPC.
	safe := before
	safe.Definition.PrivateJudgeGuidance = ""
	ack := r.admissionBrowserPhase(t, item, lifecycleBrowserPhase{Sequence: 1601 + index*20, Action: "create-final-round", JudgingSetup: &lifecycleBrowserJudgingSetupInput{Configuration: &safe, Reason: "Plan an independent final round; practice never scores this run"}})
	if !ack.Saved || ack.RunID == nil || *ack.RunID == uuid.Nil || ack.CommandID != nil {
		t.Fatal("final round requires its actual server-generated run identity, not a command UUID")
	}
	var current rubric.Configuration
	if err := r.owner.call(t.Context(), http.MethodGet, "/api/v1/events/"+item.view.ID.String()+"/rubric/configuration", -1, nil, &current, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if !lifecycleCreatedAssessmentRun(before, current, *ack.RunID, "judge_final") {
		t.Fatal("browser round creation changed an exact source, version, retained run or new run identity")
	}

	item.browserFinalRoundCreated = true
	return current
}

func (r *lifecycleJourney) assignFinalJudgesInBrowser(t *testing.T, item *lifecycleJourneyEvent, index, teamIndex int, teamID uuid.UUID, teamName string, before judging.Round, judges []*lifecycleClient) judging.Receipt {
	t.Helper()
	input := lifecycleBrowserJudgingSetupInput{Round: &before, TeamID: teamID, TeamName: teamName, Reason: "Three independently selected current judges, never mentor votes"}
	var staff struct {
		Items []struct {
			PersonID uuid.UUID `json:"person_id"`
			Name     string    `json:"display_name"`
			Role     string    `json:"role"`
			Active   bool      `json:"active"`
		} `json:"items"`
	}
	if err := r.owner.call(t.Context(), http.MethodGet, "/api/v1/events/"+item.view.ID.String()+"/staff", -1, nil, &staff, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	ids := []uuid.UUID{}
	for _, judge := range judges {
		found := false
		for _, person := range staff.Items {
			if person.PersonID == judge.person && person.Active && person.Role == "judge" && person.Name == judge.name {
				found = true
				input.Judges = append(input.Judges, lifecycleBrowserSetupJudge{person.PersonID, person.Name})
			}
		}
		if !found {
			t.Fatal("selected native judge is not present in current named staff")
		}
		ids = append(ids, judge.person)
	}
	ack := r.admissionBrowserPhase(t, item, lifecycleBrowserPhase{Sequence: 1602 + index*20 + teamIndex, Action: "assign-final-judges", JudgingSetup: &input})
	if !ack.Saved || ack.CommandID == nil || *ack.CommandID == uuid.Nil {
		t.Fatal("judge assignment omitted its original browser-generated command UUID")
	}
	var recovered struct {
		Receipt *judging.Receipt `json:"receipt"`
	}
	path := fmt.Sprintf("/api/v1/events/%s/judging-receipts/%s", item.view.ID, *ack.CommandID)
	if err := r.owner.call(t.Context(), http.MethodGet, path, -1, nil, &recovered, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	command := judging.RoundCommand{RequestID: *ack.CommandID, Action: "assign", TeamID: teamID, JudgeIDs: ids, Reason: input.Reason}
	raw, err := json.Marshal(struct {
		Run     uuid.UUID
		Version int64
		Command judging.RoundCommand
	}{item.judgeRun, before.Revision, command})
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	receipt := recovered.Receipt
	if receipt == nil || receipt.RequestID != command.RequestID || receipt.ActorID != r.owner.person || receipt.RunID != item.judgeRun || receipt.TeamID != teamID || receipt.Action != "assign" || receipt.State != "recorded" || receipt.Revision != before.Revision+1 || receipt.Fingerprint != hex.EncodeToString(sum[:]) {
		t.Fatal("judge assignment lacks its exact immutable native receipt")
	}
	item.browserAssignedTeams = append(item.browserAssignedTeams, teamID)
	return *receipt
}

func lifecycleBrowserJudgingSetupComplete(item lifecycleJourneyEvent) bool {
	if !item.browserFinalRoundCreated || len(item.browserAssignedTeams) != 6 {
		return false
	}
	next := 0
	for _, team := range item.teams.Teams {
		if team.State != "disbanded" {
			if next >= len(item.browserAssignedTeams) || item.browserAssignedTeams[next] != team.ID {
				return false
			}
			next++
		}
	}
	return next == 6
}
