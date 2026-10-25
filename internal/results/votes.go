package results

import (
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
)

type TieState struct {
	Tie              Tie         `json:"tie"`
	RequiredJudgeIDs []uuid.UUID `json:"required_judge_ids"`
	VotesReceived    int         `json:"votes_received"`
	WinnerID         *uuid.UUID  `json:"winner_id"`
	Blocker          string      `json:"blocker"`
}

// resolveJudgeTies requires every judge assigned to the tied entries to vote.
// Each judge must be assigned to all candidates; partial assignments block
// resolution rather than silently reducing the voting panel. Missing ballots, split votes or
// no common authorised panel remain blockers; an organiser never casts a final
// tie decision merely by holding the organiser role. Current native completion
// has already checked each required judge's assignment, conflicts and evidence.
func resolveJudgeTies(
	ties []Tie,
	assignments []judging.Assignment,
	votes []Vote,
) ([]TieState, error) {
	out := make([]TieState, 0, len(ties))
	known := map[string]bool{}
	for _, tie := range ties {
		known[tie.ID] = true
		ids, complete := requiredTieJudges(tie.TeamIDs, assignments)
		state := TieState{Tie: tie, RequiredJudgeIDs: ids}
		if !complete {
			state.Blocker = "complete_panel_assignment_required"
			out = append(out, state)
			continue
		}
		ballots := map[uuid.UUID]bool{}
		counts := map[uuid.UUID]int{}
		for _, vote := range votes {
			if vote.TieID != tie.ID {
				continue
			}
			if !slices.Contains(ids, vote.JudgeID) || !slices.Contains(tie.TeamIDs, vote.TeamID) ||
				ballots[vote.JudgeID] ||
				strings.TrimSpace(vote.Reason) == "" ||
				len(vote.Evidence) == 0 {
				return nil, ErrIncomplete
			}
			ballots[vote.JudgeID] = true
			counts[vote.TeamID]++
		}
		state.VotesReceived = len(ballots)
		if len(ballots) != len(ids) {
			state.Blocker = "all_required_judge_votes_required"
		} else {
			for _, candidate := range tie.TeamIDs {
				if counts[candidate] > len(ids)/2 {
					winner := candidate
					state.WinnerID = &winner
					break
				}
			}
			if state.WinnerID == nil {
				state.Blocker = "recorded_judge_majority_required"
			}
		}
		out = append(out, state)
	}
	for _, vote := range votes {
		if !known[vote.TieID] {
			return nil, ErrIncomplete
		}
	}
	return out, nil
}

func requiredTieJudges(teams []uuid.UUID, assignments []judging.Assignment) ([]uuid.UUID, bool) {
	ids := []uuid.UUID{}
	if len(teams) < 2 {
		return ids, false
	}
	for _, team := range teams {
		index := slices.IndexFunc(
			assignments,
			func(v judging.Assignment) bool { return v.TeamID == team },
		)
		if index < 0 {
			return ids, false
		}
		ids = append(ids, assignments[index].JudgeIDs...)
	}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	ids = slices.Compact(ids)
	if len(ids) == 0 {
		return ids, false
	}
	for _, team := range teams {
		index := slices.IndexFunc(
			assignments,
			func(v judging.Assignment) bool { return v.TeamID == team },
		)
		for _, id := range ids {
			if !slices.Contains(assignments[index].JudgeIDs, id) {
				return ids, false
			}
		}
	}
	return ids, true
}
