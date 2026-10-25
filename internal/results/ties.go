package results

import (
	"math/big"
	"slices"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/qualification"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

func sourceTies(source Preview) ([]Tie, error) {
	out := []Tie{}
	for start := 0; start < len(source.Tallies); {
		end := start + 1
		for end < len(source.Tallies) && source.Tallies[end].Rank == source.Tallies[start].Rank {
			end++
		}
		if end-start > 1 {
			ids := make([]uuid.UUID, 0, end-start)
			for _, team := range source.Tallies[start:end] {
				ids = append(ids, team.TeamID)
			}
			out = append(out, makeTie(source.Binding, "final", nil, ids))
		}
		start = end
	}
	if source.Criteria.Mode == rubric.AllTeams {
		categories, err := categoryContenders(source)
		if err != nil {
			return nil, err
		}
		for _, category := range categories {
			if len(category.TeamIDs) > 1 {
				id := category.CategoryID
				out = append(out, makeTie(source.Binding, "category", &id, category.TeamIDs))
			}
		}
	}
	return out, nil
}

func makeTie(binding, kind string, category *uuid.UUID, ids []uuid.UUID) Tie {
	ids = slices.Clone(ids)
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	tie := Tie{Kind: kind, CategoryID: category, TeamIDs: ids}
	tie.ID = digest(struct {
		Binding string
		Tie     Tie
	}{binding, tie})
	return tie
}

func ratio(value Exact) (*big.Rat, error) {
	numerator, n := new(big.Int).SetString(value.Numerator, 10)
	denominator, d := new(big.Int).SetString(value.Denominator, 10)
	if !n || !d || denominator.Sign() <= 0 {
		return nil, ErrIncomplete
	}
	return new(big.Rat).SetFrac(numerator, denominator), nil
}

// categoryContenders preserves all-team category scope. In qualification mode
// the exact native signed panel source owns categories, including non-finalists.
func categoryContenders(source Preview) ([]qualification.CategoryProposal, error) {
	if source.Criteria.Mode == rubric.MentorQualification {
		if source.Selection == nil {
			return nil, ErrIncomplete
		}
		return source.Selection.Categories, nil
	}
	out := make([]qualification.CategoryProposal, 0, len(source.Criteria.Categories))
	for _, category := range source.Criteria.Categories {
		var maximum *big.Rat
		ids := make([]uuid.UUID, 0, len(source.Tallies))
		for _, team := range source.Tallies {
			index := slices.IndexFunc(
				team.Categories,
				func(v CategoryAverage) bool { return v.CategoryID == category.ID },
			)
			if index < 0 {
				return nil, ErrIncomplete
			}
			value, err := ratio(team.Categories[index].Average)
			if err != nil {
				return nil, err
			}
			switch {
			case maximum == nil || value.Cmp(maximum) > 0:
				maximum = value
				ids = append(ids[:0], team.TeamID)
			case value.Cmp(maximum) == 0:
				ids = append(ids, team.TeamID)
			}
		}
		if len(ids) == 0 {
			return nil, ErrIncomplete
		}
		slices.SortFunc(ids, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
		out = append(out, qualification.CategoryProposal{CategoryID: category.ID, TeamIDs: ids})
	}
	return out, nil
}
