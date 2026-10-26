package app

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/results"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/google/uuid"
)

func lifecycleTieState(t *testing.T, page results.Page, id string) results.TieState {
	t.Helper()
	for _, state := range page.TieStates {
		if state.Tie.ID == id {
			return state
		}
	}
	t.Fatal("native result lost an original tied candidate source")
	return results.TieState{}
}

func lifecycleOnlyUnresolvedTie(t *testing.T, page results.Page, id, blocker string, votes int) {
	t.Helper()
	unresolved := 0
	for _, state := range page.TieStates {
		if state.Blocker == "" && state.WinnerID != nil {
			continue
		}
		unresolved++
		if state.Tie.ID != id || state.Blocker != blocker || state.VotesReceived != votes || state.WinnerID != nil {
			t.Fatal("negative confirmation has an unexpected native blocker")
		}
	}
	if unresolved != 1 {
		t.Fatal("negative final confirmation did not isolate its intended ballot blocker")
	}
}

func (r *lifecycleJourney) decideJudgeResults(t *testing.T) {
	t.Helper()
	for index := range r.events {
		item := &r.events[index]
		if item.judgeFinal == nil || !item.judgeFinal.DeliberationOpened {
			t.Fatal("results lack actual complete fresh judge deliberation")
		}
		receipt := &lifecycleResultDecisionReceipt{RunID: item.judgeRun}
		item.resultDecision = receipt
		page := r.currentResults(t, item)
		if len(page.Preview.Blockers) != 0 || page.Preview.Binding == "" || page.Preview.JudgeRunID != item.judgeRun || len(page.Preview.Criteria.Prizes) != 2 {
			t.Fatal("native result source lacks current judging or predeclared prizes")
		}
		page = r.changeResults(t, item, page, map[string]any{"action": "prepare", "source_binding": page.Preview.Binding})
		expectedTies := 1
		if page.Preview.Criteria.Mode == rubric.AllTeams {
			expectedTies += len(page.Preview.Criteria.Categories)
		} else if page.Preview.Criteria.Mode != rubric.MentorQualification {
			t.Fatal("unrecognised native assessment mode")
		}
		if !page.Current || page.Round.Source.Binding != page.Preview.Binding || page.Round.Confirmation != nil || len(page.Round.Votes) != 0 || len(page.TieStates) != expectedTies {
			t.Fatal("native result preparation inferred ballots or confirmation")
		}
		receipt.SourceBinding = page.Preview.Binding
		if page.Preview.Criteria.Mode == rubric.AllTeams {
			receipt.AdvisoryScope = "current_category_ties"
			page = r.adviseCurrentCategoryTie(t, item, page)
		} else {
			receipt.AdvisoryScope = "none_categories_already_signed_by_qualification"
			r.verifyQualificationAdvisoryScope(t, item, page)
		}
		tallies := page.Preview.Tallies
		judges := map[uuid.UUID]bool{}
		for _, actor := range []string{"judge-a", "judge-b", "judge-c"} {
			judges[item.cohort.staff[actor].person] = true
		}
		for _, state := range page.TieStates {
			if len(state.RequiredJudgeIDs) != 3 || len(state.Tie.TeamIDs) < 3 || state.VotesReceived != 0 {
				t.Fatal("native tie does not require the actual complete three-judge panel")
			}
			for _, id := range state.RequiredJudgeIDs {
				if !judges[id] {
					t.Fatal("mentor or organiser entered required final judge ballots")
				}
			}
		}
		first := page.TieStates[0]
		// Settle every other seat first. The following negative confirmations then
		// have exactly one cause: a missing ballot, followed by a complete split.
		for _, state := range page.TieStates[1:] {
			for _, actor := range []string{"judge-a", "judge-b", "judge-c"} {
				r.voteResult(t, item, item.cohort.staff[actor], state.Tie.ID, state.Tie.TeamIDs[0])
			}
		}
		for at, actor := range []string{"judge-a", "judge-b"} {
			r.voteResult(t, item, item.cohort.staff[actor], first.Tie.ID, first.Tie.TeamIDs[at])
		}
		page = r.currentResults(t, item)
		lifecycleOnlyUnresolvedTie(t, page, first.Tie.ID, "all_required_judge_votes_required", 2)
		page = r.denyPrematureResultConfirmation(t, item, page)
		receipt.MissingJudgeConfirmationDenied = true
		r.voteResult(t, item, item.cohort.staff["judge-c"], first.Tie.ID, first.Tie.TeamIDs[2])
		oldThird := receipt.Votes[len(receipt.Votes)-1]
		page = r.currentResults(t, item)
		lifecycleOnlyUnresolvedTie(t, page, first.Tie.ID, "recorded_judge_majority_required", 3)
		page = r.denyPrematureResultConfirmation(t, item, page)
		receipt.SplitConfirmationDenied = true
		r.voteResult(t, item, item.cohort.staff["judge-c"], first.Tie.ID, first.Tie.TeamIDs[0])
		page = r.currentResults(t, item)
		resolved := lifecycleTieState(t, page, first.Tie.ID)
		if resolved.VotesReceived != 3 || resolved.WinnerID == nil || *resolved.WinnerID != first.Tie.TeamIDs[0] || resolved.Blocker != "" {
			t.Fatal("explicit current judge revote did not produce a strict two-of-three majority")
		}
		receipt.ExplicitRevoteMajority = true
		var original results.Recovery
		if err := item.cohort.staff["judge-c"].call(t.Context(), http.MethodGet, "/api/v1/events/"+item.view.ID.String()+"/results/requests/"+oldThird.RequestID.String(), -1, nil, &original, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(original.Receipt, &oldThird) {
			t.Fatal("new ballot overwrote original actor-bound acknowledgement")
		}
		receipt.OriginalVoteSurvivesRevote = true
		for _, state := range page.TieStates {
			if state.WinnerID == nil || state.Blocker != "" || state.VotesReceived != len(state.RequiredJudgeIDs) {
				t.Fatal("final confirmation lacks a complete current judge-majority decision")
			}
		}
		if len(page.Round.Votes) != len(page.TieStates)*3 || !reflect.DeepEqual(page.Preview.Tallies, tallies) || page.Preview.Binding != receipt.SourceBinding {
			t.Fatal("new ballots duplicated voters or changed independent numeric scoring")
		}
		awards := lifecycleSyntheticAwards(t, page)
		page = r.changeResults(t, item, page, map[string]any{"action": "confirm", "source_binding": page.Preview.Binding, "awards": awards})
		confirmed := page.Round.Confirmation
		if confirmed == nil || confirmed.ID == uuid.Nil || confirmed.JudgeApprovalID == uuid.Nil || confirmed.SourceBinding != receipt.SourceBinding || confirmed.DecisionBinding == "" || !reflect.DeepEqual(confirmed.Awards, awards) {
			t.Fatal("separate native award confirmation changed source or implicitly published results")
		}
		lifecycleAssertReleaseKinds(t, item, page.Announcements, false)
		id, approval := confirmed.ID, confirmed.JudgeApprovalID
		receipt.ConfirmationID = &id
		receipt.JudgeApprovalID = &approval
		receipt.DecisionBinding = confirmed.DecisionBinding
		receipt.Awards = confirmed.Awards
		var member *lifecycleClient
		for _, team := range item.teams.Teams {
			if team.ID == item.judgeFinal.EligibleTeamIDs[0] {
				member = r.participant(t, team.Published[0].ID)
			}
		}
		if member == nil {
			t.Fatal("current result recipient missing from actual frozen cohort")
		}
		var released struct {
			Items []results.Announcement `json:"items"`
		}
		if err := member.call(t.Context(), http.MethodGet, "/api/v1/events/"+item.view.ID.String()+"/results/announcements", -1, nil, &released, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		lifecycleAssertReleaseKinds(t, item, released.Items, false)
		receipt.AnnouncementsBeforeRelease = len(released.Items)
		r.persistWorkProgress(t)
	}
}
