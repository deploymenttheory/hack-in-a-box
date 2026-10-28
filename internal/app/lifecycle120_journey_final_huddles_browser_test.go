package app

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/moderation"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/google/uuid"
)

type lifecycleBrowserFinalHuddleInput struct {
	Actor          string                     `json:"actor"`
	ActorID        uuid.UUID                  `json:"actor_id"`
	TeamID         uuid.UUID                  `json:"team_id"`
	HuddleID       uuid.UUID                  `json:"huddle_id"`
	Revision       int64                      `json:"revision"`
	Version        int64                      `json:"version"`
	Action         string                     `json:"action"`
	Reason         string                     `json:"reason"`
	Definition     moderation.Definition      `json:"definition"`
	Cards          []scorecards.CardReference `json:"cards"`
	FrozenRevision *int64                     `json:"frozen_revision"`
}
type lifecycleFinalHuddleActionReceipt struct {
	TeamID      uuid.UUID `json:"team_id"`
	HuddleID    uuid.UUID `json:"huddle_id"`
	ActorID     uuid.UUID `json:"actor_id"`
	ModeratorID uuid.UUID `json:"moderator_id"`
	Action      string    `json:"action"`
	Version     int64     `json:"version"`
}

func (r *lifecycleJourney) configureFinalHuddleInBrowser(t *testing.T, item *lifecycleJourneyEvent, teamIndex int, teamID uuid.UUID, revision int64, definition moderation.Definition, reason string, cards []scorecards.CardReference) moderation.Huddle {
	t.Helper()
	safe := definition
	safe.ID = uuid.Nil // The production configuration owner mints this resource ID.
	input := lifecycleBrowserFinalHuddleInput{Actor: "owner", ActorID: r.owner.person, TeamID: teamID, Revision: revision, Action: "configure", Reason: reason, Definition: safe, Cards: cards}
	ack := r.admissionBrowserPhase(t, item, lifecycleBrowserPhase{Sequence: 2001 + teamIndex*4, Action: "configure-final-huddle", Huddle: &input})
	if !ack.Saved || ack.HuddleID == nil || *ack.HuddleID == uuid.Nil || ack.CommandID != nil {
		t.Fatal("huddle configuration lacks its actual UI-created resource identity")
	}
	definition.ID = *ack.HuddleID
	huddle := r.readOriginalHuddleAction(t, item, r.owner, teamID, *ack.HuddleID, 1, "configure", reason)
	if !sameLifecycleHuddleDefinition(huddle.Definition, definition) || huddle.State != "planned" || huddle.Checkpoint.Purpose != "mentor_final" || item.review.FinalCheckpoint == nil || !reflect.DeepEqual(huddle.Checkpoint, *item.review.FinalCheckpoint) || huddle.PrimaryID != item.cohort.staff["mentor-a"].person || huddle.Review.ReviewerID != item.cohort.staff["mentor-b"].person {
		t.Fatal("browser configuration lost its exact final checkpoint or intended current panel")
	}
	expected := map[uuid.UUID]bool{r.owner.person: true, item.cohort.staff["mentor-a"].person: true, item.cohort.staff["mentor-b"].person: true}
	if len(huddle.PanelIDs) != 3 {
		t.Fatal("huddle configuration changed its intended native panel census")
	}
	for _, id := range huddle.PanelIDs {
		if !expected[id] {
			t.Fatal("huddle configuration introduced another panel member")
		}
		delete(expected, id)
	}
	if len(expected) != 0 {
		t.Fatal("huddle configuration omitted an intended panel member")
	}
	item.browserFinalHuddleActions = append(item.browserFinalHuddleActions, lifecycleFinalHuddleActionReceipt{TeamID: teamID, HuddleID: huddle.Definition.ID, ActorID: r.owner.person, ModeratorID: huddle.Definition.ModeratorID, Action: "configure", Version: huddle.Version})
	return huddle
}

func (r *lifecycleJourney) decideFinalHuddleInBrowser(t *testing.T, item *lifecycleJourneyEvent, teamIndex, actionIndex int, client *lifecycleClient, revision int64, before moderation.Huddle, action, reason string, cards []scorecards.CardReference, frozen int64) moderation.Huddle {
	t.Helper()
	actor := "owner"
	if action == "complete_second_review" {
		actor = "mentor-b"
		if client != item.cohort.staff[actor] {
			t.Fatal("second-review UI actor differs from selected native mentor")
		}
	} else if client != r.owner {
		t.Fatal("formal final huddle decision lost its named native moderator")
	}
	input := lifecycleBrowserFinalHuddleInput{Actor: actor, ActorID: client.person, TeamID: before.TeamID, HuddleID: before.Definition.ID, Revision: revision, Version: before.Version, Action: action, Reason: reason, Definition: before.Definition, Cards: cards, FrozenRevision: &frozen}
	ack := r.admissionBrowserPhase(t, item, lifecycleBrowserPhase{Sequence: 2002 + teamIndex*4 + actionIndex, Action: "decide-final-huddle", Huddle: &input})
	if !ack.Saved || ack.HuddleID == nil || *ack.HuddleID != before.Definition.ID || ack.CommandID != nil {
		t.Fatal("huddle decision acknowledged another original resource")
	}
	huddle := r.readOriginalHuddleAction(t, item, client, before.TeamID, before.Definition.ID, before.Version+1, action, reason)
	if !sameLifecycleHuddleDefinition(huddle.Definition, before.Definition) || !reflect.DeepEqual(huddle.Checkpoint, before.Checkpoint) {
		t.Fatal("formal huddle action changed its captured immutable source")
	}
	switch action {
	case "complete_second_review":
		if huddle.State != "planned" || huddle.Review.CompletedAt == nil || huddle.Review.Card == nil || len(cards) != 2 || *huddle.Review.Card != cards[1] {
			t.Fatal("second review did not retain the actual independent second-author final card")
		}
	case "open", "sign_off":
		if huddle.SubmissionRevision == nil || *huddle.SubmissionRevision != frozen || len(huddle.Cards) != 2 {
			t.Fatal("huddle lost its exact frozen output or both original independent final cards")
		}
		expected := map[scorecards.CardReference]bool{}
		for _, ref := range cards {
			expected[ref] = true
		}
		for _, ref := range huddle.Cards {
			if !expected[ref] {
				t.Fatal("huddle substituted an original final card")
			}
			delete(expected, ref)
		}
		if len(expected) != 0 {
			t.Fatal("huddle omitted an independent final card")
		}
		if action == "open" && huddle.State != "open" {
			t.Fatal("huddle opening did not retain its native state")
		}
		if action == "sign_off" && (huddle.State != "signed" || huddle.SignoffID == nil || huddle.SignoffContext == nil || huddle.SignoffContext.Binding == "") {
			t.Fatal("huddle sign-off lacks its native immutable final proof")
		}
	default:
		t.Fatal("unexpected final huddle browser action")
	}
	item.browserFinalHuddleActions = append(item.browserFinalHuddleActions, lifecycleFinalHuddleActionReceipt{TeamID: before.TeamID, HuddleID: huddle.Definition.ID, ActorID: client.person, ModeratorID: huddle.Definition.ModeratorID, Action: action, Version: huddle.Version})
	return huddle
}

func (r *lifecycleJourney) readOriginalHuddleAction(t *testing.T, item *lifecycleJourneyEvent, client *lifecycleClient, teamID, id uuid.UUID, version int64, action, reason string) moderation.Huddle {
	t.Helper()
	path := fmt.Sprintf("/api/v1/events/%s/teams/%s/moderation-huddles/%s", item.view.ID, teamID, id)
	var huddle moderation.Huddle
	if err := client.call(t.Context(), http.MethodGet, path, -1, nil, &huddle, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if huddle.Definition.ID != id || huddle.TeamID != teamID || huddle.Version != version || huddle.ActorID != client.person || huddle.Action != action || huddle.Reason != reason {
		t.Fatal("huddle action lost its exact native resource, author, original action or version")
	}
	var history moderation.History
	if err := client.call(t.Context(), http.MethodGet, fmt.Sprintf("%s/history?cursor=%d&limit=1", path, version+1), -1, nil, &history, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if len(history.Items) != 1 || !reflect.DeepEqual(history.Items[0], huddle) {
		t.Fatal("formal huddle action lacks its exact original immutable history")
	}
	return huddle
}
func sameLifecycleHuddleDefinition(a, b moderation.Definition) bool {
	return a.ID == b.ID && a.CheckpointID == b.CheckpointID && a.CheckpointVersion == b.CheckpointVersion && a.ModeratorID == b.ModeratorID && a.SecondReviewerID == b.SecondReviewerID && a.ReviewDueAt.Equal(b.ReviewDueAt)
}
func lifecycleBrowserFinalHuddlesComplete(item lifecycleJourneyEvent) bool {
	if item.rubric.Definition.Mode != rubric.MentorQualification {
		return len(item.browserFinalHuddleActions) == 0 && len(item.review.FinalHuddles) == 0
	}
	if len(item.browserFinalHuddleActions) != 16 || len(item.review.FinalHuddles) != 4 {
		return false
	}
	for index, huddle := range item.review.FinalHuddles {
		moderator := item.browserFinalHuddleActions[index*4].ModeratorID
		if moderator == uuid.Nil {
			return false
		}
		for offset, action := range []string{"configure", "complete_second_review", "open", "sign_off"} {
			receipt := item.browserFinalHuddleActions[index*4+offset]
			actor := moderator
			if offset == 1 {
				actor = item.cohort.staff["mentor-b"].person
			}
			if receipt.ModeratorID != moderator || receipt.ActorID != actor || receipt.TeamID != huddle.TeamID || receipt.HuddleID != huddle.ID || receipt.Action != action || receipt.Version != int64(offset+1) {
				return false
			}
		}
	}
	return true
}
