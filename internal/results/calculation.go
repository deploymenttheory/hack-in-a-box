// Package results owns final tallies, award confirmation and explicit publication.
// Judge cards and moderated mentor selections retain their native source owners.
package results

import (
	"errors"
	"math/big"
	"slices"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

// ErrIncomplete means the complete required assessment is not available. It is
// never permission to reduce a denominator, insert a zero or choose a tied team.
var ErrIncomplete = errors.New("complete current required judge assessment needed")

// Exact preserves the exact base-10 score sum/divisor, including recurring
// averages. Display is presentation only: comparisons never use rounded text.
type Exact struct {
	Numerator   string `json:"numerator"`
	Denominator string `json:"denominator"`
	Display     string `json:"display"`
}
type CategoryAverage struct {
	CategoryID uuid.UUID `json:"category_id"`
	Average    Exact     `json:"average"`
}
type TeamTally struct {
	TeamID         uuid.UUID         `json:"team_id"`
	RequiredJudges int               `json:"required_judges"`
	Categories     []CategoryAverage `json:"categories"`
	Total          Exact             `json:"total"`
	Rank           int               `json:"rank"`
}
type JudgeSource struct {
	RunID       uuid.UUID
	Criteria    rubric.PublicDefinition
	Reference   rubric.Reference
	Assignments []judging.Assignment
	Cards       []judging.Card
}

func exact(value *big.Rat) Exact {
	return Exact{
		Numerator:   value.Num().String(),
		Denominator: value.Denom().String(),
		Display:     value.FloatString(2),
	}
}

// Calculate accepts the native completion owner's exact required assignment set.
// Native completion remains responsible for current authority, frozen evidence
// and source eligibility. This pure calculator also rejects missing/extra authors,
// duplicate cards, wrong references and incomplete/out-of-range category values.
func Calculate(source JudgeSource) ([]TeamTally, error) {
	if source.RunID == uuid.Nil || source.Reference.ID == uuid.Nil ||
		source.Reference.Version < 1 ||
		source.Reference.Digest == "" ||
		(rubric.Definition{PublicDefinition: source.Criteria}).Validate() != nil ||
		len(source.Assignments) == 0 {
		return nil, ErrIncomplete
	}
	cards := map[uuid.UUID]map[uuid.UUID]judging.Card{}
	for _, card := range source.Cards {
		if card.RunID != source.RunID || card.TeamID == uuid.Nil || card.AuthorID == uuid.Nil ||
			card.State != "submitted" ||
			card.Rubric != source.Reference {
			return nil, ErrIncomplete
		}
		if cards[card.TeamID] == nil {
			cards[card.TeamID] = map[uuid.UUID]judging.Card{}
		}
		if _, duplicate := cards[card.TeamID][card.AuthorID]; duplicate {
			return nil, ErrIncomplete
		}
		cards[card.TeamID][card.AuthorID] = card
	}
	out := make([]TeamTally, 0, len(source.Assignments))
	totals := map[uuid.UUID]*big.Rat{}
	for _, assignment := range source.Assignments {
		if assignment.TeamID == uuid.Nil || totals[assignment.TeamID] != nil ||
			assignment.Missing != 0 ||
			len(assignment.JudgeIDs) < source.Criteria.RequiredJudgeCount ||
			len(cards[assignment.TeamID]) != len(assignment.JudgeIDs) {
			return nil, ErrIncomplete
		}
		team, total, err := calculateTeam(source.Criteria, assignment, cards[assignment.TeamID])
		if err != nil {
			return nil, err
		}
		out = append(out, team)
		totals[team.TeamID] = total
	}
	if len(cards) != len(source.Assignments) {
		return nil, ErrIncomplete
	}
	slices.SortFunc(out, func(a, b TeamTally) int {
		if order := totals[b.TeamID].Cmp(totals[a.TeamID]); order != 0 {
			return order
		}
		return slices.Compare(a.TeamID[:], b.TeamID[:])
	})
	for i := range out {
		out[i].Rank = i + 1
		if i > 0 && totals[out[i].TeamID].Cmp(totals[out[i-1].TeamID]) == 0 {
			out[i].Rank = out[i-1].Rank
		}
	}
	return out, nil
}

func calculateTeam(
	criteria rubric.PublicDefinition,
	assignment judging.Assignment,
	cards map[uuid.UUID]judging.Card,
) (TeamTally, *big.Rat, error) {
	out := TeamTally{
		TeamID:         assignment.TeamID,
		RequiredJudges: len(assignment.JudgeIDs),
		Categories:     []CategoryAverage{},
	}
	sums := map[uuid.UUID]int64{}
	seen := map[uuid.UUID]bool{}
	for _, author := range assignment.JudgeIDs {
		card, exists := cards[author]
		if author == uuid.Nil || seen[author] || !exists ||
			len(card.Scores) != len(criteria.Categories) {
			return TeamTally{}, nil, ErrIncomplete
		}
		seen[author] = true
		scores := map[uuid.UUID]bool{}
		for _, score := range card.Scores {
			index := slices.IndexFunc(
				criteria.Categories,
				func(category rubric.Category) bool { return category.ID == score.CategoryID },
			)
			if index < 0 || score.Value == nil || scores[score.CategoryID] {
				return TeamTally{}, nil, ErrIncomplete
			}
			category := criteria.Categories[index]
			if *score.Value < category.MinScore || *score.Value > category.MaxScore {
				return TeamTally{}, nil, ErrIncomplete
			}
			scores[score.CategoryID] = true
			sums[score.CategoryID] += int64(*score.Value)
		}
	}
	total := new(big.Rat)
	for _, category := range criteria.Categories {
		average := new(big.Rat).SetFrac64(sums[category.ID], int64(len(assignment.JudgeIDs)))
		out.Categories = append(
			out.Categories,
			CategoryAverage{CategoryID: category.ID, Average: exact(average)},
		)
		weighted := new(big.Rat).Mul(average, new(big.Rat).SetInt64(int64(category.Weight)))
		total.Add(total, weighted)
	}
	out.Total = exact(total)
	return out, total, nil
}
