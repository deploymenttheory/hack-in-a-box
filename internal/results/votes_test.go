package results

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
)

func voteSource() (Tie, []judging.Assignment, []uuid.UUID) {
	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	tie := makeTie("actual-reviewed-source", "final", nil, []uuid.UUID{teamA, teamB})
	assignments := []judging.Assignment{
		{TeamID: teamA, JudgeIDs: ids},
		{TeamID: teamB, JudgeIDs: ids},
	}
	return tie, assignments, ids
}

func ballot(tie Tie, judge, team uuid.UUID) Vote {
	return Vote{
		TieID:    tie.ID,
		JudgeID:  judge,
		TeamID:   team,
		Reason:   "Reviewed frozen evidence",
		Evidence: []VoteEvidence{{TeamID: team, Note: "Explicit judge comparison"}},
	}
}

func TestFinalTieRequiresEveryJudgeBeforeStrictMajorityCanResolve(t *testing.T) {
	tie, assignments, ids := voteSource()
	votes := []Vote{ballot(tie, ids[0], teamA), ballot(tie, ids[1], teamA)}
	states, err := resolveJudgeTies([]Tie{tie}, assignments, votes)
	if err != nil || states[0].WinnerID != nil ||
		states[0].Blocker != "all_required_judge_votes_required" {
		t.Fatalf("missing judge ignored: %+v %v", states, err)
	}
	votes = append(votes, ballot(tie, ids[2], teamB))
	states, err = resolveJudgeTies([]Tie{tie}, assignments, votes)
	if err != nil || states[0].WinnerID == nil || *states[0].WinnerID != teamA ||
		states[0].VotesReceived != 3 {
		t.Fatalf("complete strict majority not resolved: %+v %v", states, err)
	}
}

func TestSplitFinalVotesStayUnresolvedUntilANewRecordedVote(t *testing.T) {
	tie, assignments, ids := voteSource()
	for i := range assignments {
		assignments[i].JudgeIDs = ids[:2]
	}
	votes := []Vote{ballot(tie, ids[0], teamA), ballot(tie, ids[1], teamB)}
	states, err := resolveJudgeTies([]Tie{tie}, assignments, votes)
	if err != nil || states[0].WinnerID != nil ||
		states[0].Blocker != "recorded_judge_majority_required" {
		t.Fatalf("split silently resolved: %+v %v", states, err)
	}
	// The transactional command owner retains the prior ballot in immutable
	// version history; only an explicit later vote changes this current input.
	votes[1] = ballot(tie, ids[1], teamA)
	states, err = resolveJudgeTies([]Tie{tie}, assignments, votes)
	if err != nil || states[0].WinnerID == nil || *states[0].WinnerID != teamA {
		t.Fatalf("new complete vote did not resolve: %+v %v", states, err)
	}
}

func TestPartialAssignmentsCannotSilentlyReduceFinalVotingPanel(t *testing.T) {
	tie, assignments, ids := voteSource()
	assignments[1].JudgeIDs = ids[:2]
	states, err := resolveJudgeTies(
		[]Tie{tie},
		assignments,
		[]Vote{ballot(tie, ids[0], teamA), ballot(tie, ids[1], teamA)},
	)
	if err != nil || states[0].WinnerID != nil || len(states[0].RequiredJudgeIDs) != 3 ||
		states[0].Blocker != "complete_panel_assignment_required" {
		t.Fatalf("partial panel silently reduced: %+v %v", states, err)
	}
}

func TestFinalVoteRejectsDuplicateOutsiderStaleAndUnreasonedBallots(t *testing.T) {
	for _, name := range []string{"duplicate", "outsider", "stale", "no reason", "no evidence", "wrong candidate"} {
		t.Run(name, func(t *testing.T) {
			tie, assignments, ids := voteSource()
			v := ballot(tie, ids[0], teamA)
			votes := []Vote{v}
			switch name {
			case "duplicate":
				votes = append(votes, v)
			case "outsider":
				votes[0].JudgeID = uuid.New()
			case "stale":
				votes[0].TieID = "superseded-source"
			case "no reason":
				votes[0].Reason = ""
			case "no evidence":
				votes[0].Evidence = nil
			case "wrong candidate":
				votes[0].TeamID = uuid.New()
			}
			got, err := resolveJudgeTies([]Tie{tie}, assignments, votes)
			if !errors.Is(err, ErrIncomplete) || got != nil {
				t.Fatalf("invalid ballot accepted: %+v %v", got, err)
			}
		})
	}
}
