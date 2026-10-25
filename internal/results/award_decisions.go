package results

import (
	"math/big"
	"regexp"
	"slices"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

var (
	awardAmount   = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`)
	awardCurrency = regexp.MustCompile(`^[A-Z]{3}$`)
)

// ValidateAwards validates the full reviewed list against the exact native
// preview and native strict-majority TieStates. It grants no reader, confirmation
// or publication permission. Amount/currency are explicit confirmation metadata:
// public rubric prizes do not contain those fields. No placement can be inferred
// from a prize's name or array position; category-less prizes target the overall winner.
func ValidateAwards(source Preview, states []TieState, awards []Award) error {
	normalized, err := awardSource(source)
	if err != nil {
		return err
	}
	winners, err := awardTieWinners(normalized, states)
	if err != nil {
		return err
	}
	categories, err := categoryContenders(normalized)
	if err != nil {
		return err
	}
	if len(awards) != len(source.Criteria.Prizes) {
		return access.ErrInvalid
	}
	seen := map[uuid.UUID]bool{}
	for _, award := range awards {
		index := slices.IndexFunc(
			source.Criteria.Prizes,
			func(p rubric.Prize) bool { return p.ID == award.PrizeID },
		)
		if index < 0 || seen[award.PrizeID] || award.TeamID == uuid.Nil ||
			!validAwardMoney(award.Amount, award.Currency) {
			return access.ErrInvalid
		}
		prize := source.Criteria.Prizes[index]
		if prize.Name != award.Name || !sameAwardCategory(prize.CategoryID, award.CategoryID) {
			return access.ErrInvalid
		}
		seen[award.PrizeID] = true
		contenders := []uuid.UUID{}
		kind := "final"
		if prize.CategoryID == nil {
			for _, team := range normalized.Tallies {
				if team.Rank == 1 {
					contenders = append(contenders, team.TeamID)
				}
			}
		} else {
			kind = "category"
			for _, category := range categories {
				if category.CategoryID == *prize.CategoryID {
					contenders = category.TeamIDs
				}
			}
		}
		if len(contenders) == 0 {
			return ErrIncomplete
		}
		winner := contenders[0]
		if len(contenders) > 1 {
			// Qualification categories have one native signed winner. A judge vote
			// cannot reinterpret an unresolved mentor category proposal.
			if source.Criteria.Mode == rubric.MentorQualification && prize.CategoryID != nil {
				return ErrIncomplete
			}
			winner = winners[makeTie(source.Binding, kind, prize.CategoryID, contenders).ID]
		}
		if winner == uuid.Nil {
			return ErrIncomplete
		}
		if award.TeamID != winner {
			return access.ErrInvalid
		}
	}
	return nil
}

func validAwardMoney(amount, currency string) bool {
	if amount == "" && currency == "" {
		return true
	}
	return len(amount) <= 64 && awardAmount.MatchString(amount) &&
		awardCurrency.MatchString(currency)
}

func sameAwardCategory(a, b *uuid.UUID) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func awardSource(source Preview) (Preview, error) {
	if len(source.Blockers) > 0 || source.JudgeRunID == uuid.Nil || source.Binding == "" ||
		len(source.Tallies) == 0 ||
		source.Reference.ID == uuid.Nil ||
		source.Reference.Version < 1 {
		return Preview{}, ErrIncomplete
	}
	definition := rubric.Definition{PublicDefinition: source.Criteria}
	if definition.Validate() != nil || definition.PublicDigest() != source.Reference.Digest {
		return Preview{}, ErrIncomplete
	}
	values := map[uuid.UUID]*big.Rat{}
	for _, team := range source.Tallies {
		total, err := ratio(team.Total)
		if err != nil || total.Sign() < 0 || team.TeamID == uuid.Nil ||
			values[team.TeamID] != nil ||
			team.RequiredJudges < source.Criteria.RequiredJudgeCount ||
			len(team.Categories) != len(source.Criteria.Categories) {
			return Preview{}, ErrIncomplete
		}
		seen := map[uuid.UUID]bool{}
		for _, value := range team.Categories {
			at := slices.IndexFunc(
				source.Criteria.Categories,
				func(c rubric.Category) bool { return c.ID == value.CategoryID },
			)
			average, err := ratio(value.Average)
			if at < 0 || seen[value.CategoryID] || err != nil {
				return Preview{}, ErrIncomplete
			}
			c := source.Criteria.Categories[at]
			if average.Cmp(big.NewRat(int64(c.MinScore), 1)) < 0 ||
				average.Cmp(big.NewRat(int64(c.MaxScore), 1)) > 0 {
				return Preview{}, ErrIncomplete
			}
			seen[value.CategoryID] = true
		}
		values[team.TeamID] = total
	}
	if source.Criteria.Mode == rubric.MentorQualification {
		if err := awardQualificationSource(source); err != nil {
			return Preview{}, err
		}
	}
	// Rebuild ranks from exact ratios, never caller/display ordering or rounded text.
	source.Tallies = slices.Clone(source.Tallies)
	slices.SortFunc(
		source.Tallies,
		func(a, b TeamTally) int { return values[b.TeamID].Cmp(values[a.TeamID]) },
	)
	for i := range source.Tallies {
		source.Tallies[i].Rank = i + 1
		if i > 0 && values[source.Tallies[i].TeamID].Cmp(values[source.Tallies[i-1].TeamID]) == 0 {
			source.Tallies[i].Rank = source.Tallies[i-1].Rank
		}
	}
	return source, nil
}

func awardQualificationSource(source Preview) error {
	selection := source.Selection
	if selection == nil || selection.RunID == uuid.Nil || selection.RoundVersion < 1 ||
		selection.Reference != source.Reference ||
		selection.SignedBinding == "" ||
		selection.SignerID == uuid.Nil ||
		selection.SignedAt.IsZero() ||
		source.FinalistsBinding != digest(selection) ||
		len(selection.Categories) != len(source.Criteria.Categories) {
		return ErrIncomplete
	}
	seen := map[uuid.UUID]bool{}
	for _, category := range selection.Categories {
		if seen[category.CategoryID] ||
			!slices.ContainsFunc(
				source.Criteria.Categories,
				func(c rubric.Category) bool { return c.ID == category.CategoryID },
			) ||
			len(category.TeamIDs) != 1 ||
			category.TeamIDs[0] == uuid.Nil {
			return ErrIncomplete
		}
		seen[category.CategoryID] = true
	}
	for _, team := range source.Tallies {
		if !slices.Contains(selection.FinalistIDs, team.TeamID) {
			return ErrIncomplete
		}
	}
	return nil
}

func awardTieWinners(source Preview, states []TieState) (map[string]uuid.UUID, error) {
	expected, err := sourceTies(source)
	if err != nil {
		return nil, err
	}
	if len(expected) != len(source.Ties) || len(states) != len(expected) {
		return nil, ErrIncomplete
	}
	winners := map[string]uuid.UUID{}
	for _, tie := range expected {
		if !slices.ContainsFunc(
			source.Ties,
			func(actual Tie) bool { return sameAwardTie(actual, tie) },
		) {
			return nil, ErrIncomplete
		}
		matches := 0
		for _, state := range states {
			if !sameAwardTie(state.Tie, tie) {
				continue
			}
			matches++
			if state.Blocker != "" || state.WinnerID == nil ||
				!slices.Contains(tie.TeamIDs, *state.WinnerID) ||
				len(state.RequiredJudgeIDs) < source.Criteria.RequiredJudgeCount ||
				state.VotesReceived != len(state.RequiredJudgeIDs) {
				return nil, ErrIncomplete
			}
			seen := map[uuid.UUID]bool{}
			for _, judge := range state.RequiredJudgeIDs {
				if judge == uuid.Nil || seen[judge] {
					return nil, ErrIncomplete
				}
				seen[judge] = true
			}
			for _, id := range tie.TeamIDs {
				at := slices.IndexFunc(
					source.Tallies,
					func(t TeamTally) bool { return t.TeamID == id },
				)
				if at < 0 || source.Tallies[at].RequiredJudges != len(state.RequiredJudgeIDs) {
					return nil, ErrIncomplete
				}
			}
			// WinnerID is supplied only by the native recorded strict-majority resolver;
			// aggregate TieState deliberately contains no raw vote payload to reinterpret.
			winners[tie.ID] = *state.WinnerID
		}
		if matches != 1 {
			return nil, ErrIncomplete
		}
	}
	return winners, nil
}

func sameAwardTie(a, b Tie) bool {
	return a.ID == b.ID && a.Kind == b.Kind && sameAwardCategory(a.CategoryID, b.CategoryID) &&
		slices.Equal(a.TeamIDs, b.TeamIDs)
}
