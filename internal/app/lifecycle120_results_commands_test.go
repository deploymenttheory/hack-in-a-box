package app

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/results"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/google/uuid"
)

type lifecycleResultDecisionReceipt struct {
	RunID                          uuid.UUID                 `json:"run_id"`
	SourceBinding                  string                    `json:"source_binding"`
	Commands                       []results.Receipt         `json:"commands"`
	Advisories                     []results.AdvisoryReceipt `json:"advisories"`
	Votes                          []results.Receipt         `json:"votes"`
	ControlledReplyLost            bool                      `json:"controlled_reply_lost"`
	ControlledReplyPOSTs           int64                     `json:"controlled_reply_posts"`
	AdvisoryNoTallyOrVoteChange    bool                      `json:"advisory_no_tally_or_vote_change"`
	AdvisoryScope                  string                    `json:"advisory_scope"`
	MentorVoteDenied               bool                      `json:"mentor_vote_denied"`
	MissingJudgeConfirmationDenied bool                      `json:"missing_judge_confirmation_denied"`
	SplitConfirmationDenied        bool                      `json:"split_confirmation_denied"`
	ExplicitRevoteMajority         bool                      `json:"explicit_revote_majority"`
	OriginalVoteSurvivesRevote     bool                      `json:"original_vote_survives_revote"`
	ConfirmationID                 *uuid.UUID                `json:"confirmation_id,omitempty"`
	JudgeApprovalID                *uuid.UUID                `json:"judge_approval_id,omitempty"`
	DecisionBinding                string                    `json:"decision_binding,omitempty"`
	Awards                         []results.Award           `json:"awards,omitempty"`
	AnnouncementsBeforeRelease     int                       `json:"announcements_before_release"`
}

func (r *lifecycleJourney) currentResults(t *testing.T, item *lifecycleJourneyEvent) results.Page {
	t.Helper()
	var page results.Page
	if err := r.owner.call(t.Context(), http.MethodGet, "/api/v1/events/"+item.view.ID.String()+"/results", -1, nil, &page, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	return page
}

func (r *lifecycleJourney) changeResults(t *testing.T, item *lifecycleJourneyEvent, page results.Page, body map[string]any) results.Page {
	t.Helper()
	path := "/api/v1/events/" + item.view.ID.String() + "/results"
	id := uuid.New()
	body = lifecycleResultCommandBody(id, page, body)
	var next results.Page
	if err := r.owner.call(t.Context(), http.MethodPost, path, page.Revision, body, &next, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	ack := next.Receipt
	// Organiser acknowledgements bind the exact command/version; the native
	// prepared source carries its judge run (ballot receipts carry RunID too).
	if ack == nil || ack.RequestID != id || ack.ActorID != r.owner.person || ack.Action != body["action"] || ack.ExpectedRevision != page.Revision || ack.ExpectedVersion != page.Round.Version || ack.Version != next.Round.Version || ack.Fingerprint == "" || next.Round.Source.JudgeRunID != item.judgeRun || next.Round.Source.Binding != body["source_binding"] {
		t.Fatal("native result command lost original actor/source/version acknowledgement")
	}
	var recovered results.Recovery
	if err := r.owner.call(t.Context(), http.MethodGet, path+"/requests/"+id.String(), -1, nil, &recovered, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(recovered.Receipt, ack) {
		t.Fatal("read-only original result acknowledgement changed")
	}
	item.resultDecision.Commands = append(item.resultDecision.Commands, *ack)
	r.persistWorkProgress(t)
	return next
}

func lifecycleResultCommandBody(id uuid.UUID, page results.Page, body map[string]any) map[string]any {
	body["request_id"] = id
	body["version"] = page.Round.Version
	body["reason"] = "Explicit synthetic reviewed result decision; no automatic announcement"
	return body
}

func (r *lifecycleJourney) voteResult(t *testing.T, item *lifecycleJourneyEvent, client *lifecycleClient, tieID string, teamID uuid.UUID) results.BallotPage {
	t.Helper()
	path := "/api/v1/events/" + item.view.ID.String() + "/results"
	var before, next results.BallotPage
	if err := client.call(t.Context(), http.MethodGet, path+"/ballots", -1, nil, &before, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	body := lifecycleResultBallotBody(id, before, tieID, teamID)
	if err := client.call(t.Context(), http.MethodPost, path+"/ballots", before.Revision, body, &next, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	ack := next.Receipt
	if ack == nil || ack.RequestID != id || ack.ActorID != client.person || ack.RunID != item.judgeRun || ack.Action != "vote" || ack.TieID != tieID || ack.ExpectedRevision != before.Revision || ack.ExpectedVersion != before.Version || ack.Version != next.Version || next.SourceBinding != before.SourceBinding {
		t.Fatal("native final ballot lost exact judge/source/version")
	}
	matched := false
	for _, tie := range next.Ties {
		if tie.State.Tie.ID == tieID && tie.OwnVote != nil && tie.OwnVote.JudgeID == client.person && tie.OwnVote.TeamID == teamID {
			matched = true
		}
	}
	if !matched {
		t.Fatal("native own ballot did not retain explicit current candidate")
	}
	var recovered results.Recovery
	if err := client.call(t.Context(), http.MethodGet, path+"/requests/"+id.String(), -1, nil, &recovered, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(recovered.Receipt, ack) {
		t.Fatal("original judge vote acknowledgement changed during read recovery")
	}
	item.resultDecision.Votes = append(item.resultDecision.Votes, *ack)
	r.persistWorkProgress(t)
	return next
}

// The API requires deliverable_id even when the evidence concerns the whole
// frozen entry. The native evidence DTO preserves that explicit JSON null.
func lifecycleResultBallotBody(id uuid.UUID, page results.BallotPage, tieID string, teamID uuid.UUID) map[string]any {
	return map[string]any{"request_id": id, "version": page.Version, "action": "vote", "source_binding": page.SourceBinding, "tie_id": tieID, "team_id": teamID, "evidence": []results.VoteEvidence{{TeamID: teamID, Note: "Recorded independent preference against this exact frozen candidate"}}, "reason": "Explicit ballot request; only current final judges may vote"}
}

func lifecycleSyntheticAwards(t *testing.T, page results.Page) []results.Award {
	t.Helper()
	if len(page.Preview.Criteria.Prizes) != 2 {
		t.Fatal("fixture prizes were not declared before common approval")
	}
	awards := []results.Award{}
	for index, prize := range page.Preview.Criteria.Prizes {
		winner := uuid.Nil
		// Qualification's signed native panel owns category winners, including
		// non-finalists. Final judges cannot add or reinterpret category ties.
		if prize.CategoryID != nil && page.Preview.Criteria.Mode == rubric.MentorQualification {
			if page.Preview.Selection == nil || page.Preview.Selection.SignedBinding == "" {
				t.Fatal("category award lacks its actual signed qualification source")
			}
			for _, category := range page.Preview.Selection.Categories {
				if category.CategoryID == *prize.CategoryID && len(category.TeamIDs) == 1 {
					winner = category.TeamIDs[0]
				}
			}
		}
		for _, state := range page.TieStates {
			match := prize.CategoryID == nil && state.Tie.Kind == "final" || prize.CategoryID != nil && state.Tie.Kind == "category" && state.Tie.CategoryID != nil && *prize.CategoryID == *state.Tie.CategoryID
			if !match {
				continue
			}
			if state.WinnerID != nil {
				winner = *state.WinnerID
			} else if len(state.Tie.TeamIDs) > 0 {
				winner = state.Tie.TeamIDs[0]
			}
		}
		if winner == uuid.Nil {
			t.Fatal("fixture declared prize lacks actual native tied contenders")
		}
		amount := []string{"100.00", "50.00"}[index]
		awards = append(awards, results.Award{PrizeID: prize.ID, Name: prize.Name, CategoryID: prize.CategoryID, TeamID: winner, Amount: amount, Currency: "GBP"})
	}
	return awards
}

func (r *lifecycleJourney) denyPrematureResultConfirmation(t *testing.T, item *lifecycleJourneyEvent, page results.Page) results.Page {
	t.Helper()
	path := "/api/v1/events/" + item.view.ID.String() + "/results"
	body := map[string]any{"request_id": uuid.New(), "version": page.Round.Version, "action": "confirm", "source_binding": page.Round.Source.Binding, "awards": lifecycleSyntheticAwards(t, page), "reason": "Negative proof: unresolved required judge ballots cannot confirm valid declared awards"}
	if err := r.owner.call(t.Context(), http.MethodPost, path, page.Revision, body, nil, http.StatusConflict); err != nil {
		t.Fatal(err)
	}
	after := r.currentResults(t, item)
	if after.Round.Version != page.Round.Version || after.Round.Confirmation != nil || after.Preview.Binding != page.Preview.Binding {
		t.Fatal("denied premature award confirmation mutated native preparation")
	}
	return after
}
