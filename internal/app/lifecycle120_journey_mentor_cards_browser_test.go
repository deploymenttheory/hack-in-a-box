package app

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/google/uuid"
)

type lifecycleBrowserMentorCardInput struct {
	Actor      string                 `json:"actor"`
	TeamID     uuid.UUID              `json:"team_id"`
	AuthorID   uuid.UUID              `json:"author_id"`
	Revision   int64                  `json:"revision"`
	Checkpoint scorecards.Checkpoint  `json:"checkpoint"`
	Command    scorecards.CardCommand `json:"command"`
}

func (r *lifecycleJourney) submitMentorCheckpointInBrowser(t *testing.T, item *lifecycleJourneyEvent, eventIndex, assignmentIndex int, actor string, client *lifecycleClient, teamID uuid.UUID, revision int64, command scorecards.CardCommand) scorecards.Card {
	t.Helper()
	if item.cohort.staff[actor] != client || command.Action != "submit" || command.Version != 0 {
		t.Fatal("mentor checkpoint lost its selected current actor or initial independent submission")
	}
	ack := r.admissionBrowserPhase(t, item, lifecycleBrowserPhase{Sequence: 1701 + eventIndex*20 + assignmentIndex, Action: "submit-live-mentor-card", MentorCard: &lifecycleBrowserMentorCardInput{Actor: actor, TeamID: teamID, AuthorID: client.person, Revision: revision, Checkpoint: item.review.Checkpoint, Command: command}})
	if !ack.Saved || ack.CardID == nil || *ack.CardID == uuid.Nil || ack.CommandID != nil {
		t.Fatal("mentor card requires its native card identity; no invented command UUID")
	}
	path := fmt.Sprintf("/api/v1/events/%s/teams/%s/scorecards", item.view.ID, teamID)
	var current scorecards.Page
	if err := client.call(t.Context(), http.MethodGet, path, -1, nil, &current, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if len(current.Items) != 1 {
		t.Fatal("mentor live checkpoint leaked other authors or produced duplicate cards")
	}
	card := current.Items[0]
	if card.Reference.ID != *ack.CardID || card.Reference.Version != 1 || card.Revision != revision+1 || card.AuthorID != client.person || card.TeamID != teamID || card.CheckpointID != command.CheckpointID || card.CheckpointVersion != command.CheckpointVersion || card.Rubric != item.review.Checkpoint.Rubric || card.RunID != item.review.Checkpoint.Definition.RunID || card.Purpose != "checkpoint" || card.State != "submitted" || card.SubmissionRevision != nil || !reflect.DeepEqual(card.Scores, command.Scores) {
		t.Fatal("browser mentor card does not match its complete independent native source, author and captured content")
	}
	var history scorecards.History
	if err := client.call(t.Context(), http.MethodGet, fmt.Sprintf("%s/%s/history?cursor=2&limit=1", path, *ack.CardID), -1, nil, &history, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if len(history.Items) != 1 || !reflect.DeepEqual(history.Items[0], card) {
		t.Fatal("browser mentor card lacks its exact immutable native first version")
	}
	item.browserLiveMentorCards = append(item.browserLiveMentorCards, lifecycleMentorCardReceipt{TeamID: card.TeamID, AuthorID: card.AuthorID, Reference: card.Reference, RunID: card.RunID})
	return card
}

func lifecycleBrowserMentorCheckpointsComplete(item lifecycleJourneyEvent) bool {
	return len(item.browserLiveMentorCards) == 12 && reflect.DeepEqual(item.browserLiveMentorCards, item.review.Cards)
}
