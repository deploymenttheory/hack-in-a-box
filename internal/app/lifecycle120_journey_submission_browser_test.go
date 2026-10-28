package app

import (
	"fmt"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/google/uuid"
)

type lifecycleBrowserSubmissionInput struct {
	Actor    string                 `json:"actor"`
	Kind     string                 `json:"kind"`
	TeamID   uuid.UUID              `json:"team_id"`
	Evidence []submissions.Evidence `json:"evidence"`
	Revision int64                  `json:"revision"`
}

// The browser makes the reviewed mutation; the caller independently reads the
// native draft or formal receipt and retains every original privacy/window gate.
func (r *lifecycleJourney) submitEvidenceInBrowser(t *testing.T, item *lifecycleJourneyEvent, eventIndex, teamIndex int, actor *lifecycleClient, kind string, teamID uuid.UUID, evidence []submissions.Evidence, revision int64) {
	t.Helper()
	alias := ""
	for i, person := range item.cohort.clients {
		if person.person == actor.person {
			alias = fmt.Sprintf("attendee-%02d", i+1)
		}
	}
	if alias == "" {
		t.Fatal("submission lacks its actual normal-OIDC team member")
	}
	base := 1101
	if kind == "formal" {
		base = 1201
	} else if kind != "draft" {
		t.Fatal("unsupported evidence action")
	}
	ack := r.admissionBrowserPhase(t, item, lifecycleBrowserPhase{Sequence: base + eventIndex*20 + teamIndex, Action: "submit-" + kind + "-evidence", Submission: &lifecycleBrowserSubmissionInput{Actor: alias, Kind: kind, TeamID: teamID, Evidence: evidence, Revision: revision}})
	if !ack.Saved {
		t.Fatal("submission lacks its real UI acknowledgement")
	}
	if kind == "draft" {
		item.browserDraftTeams = append(item.browserDraftTeams, teamID)
	} else {
		item.browserFormalTeams = append(item.browserFormalTeams, teamID)
	}
}

func lifecycleBrowserSubmissionsComplete(item lifecycleJourneyEvent) bool {
	if len(item.submissions) != 6 || len(item.browserDraftTeams) != 6 || len(item.browserFormalTeams) != 4 {
		return false
	}
	for i, receipt := range item.submissions {
		if item.browserDraftTeams[i] != receipt.TeamID {
			return false
		}
		if i < 4 && item.browserFormalTeams[i] != receipt.TeamID {
			return false
		}
	}
	return true
}
