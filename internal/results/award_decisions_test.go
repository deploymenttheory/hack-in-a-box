package results

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/deploymenttheory/hack-in-a-box/internal/qualification"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

func awardDecisionFixture() (Preview, []Award) {
	category := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	first := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	second := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	criteria := rubric.Default(rubric.AllTeams, category)
	criteria.Categories = criteria.Categories[:1]
	criteria.Categories[0].ID = category
	criteria.Prizes = []rubric.Prize{
		{ID: uuid.New(), Name: "Overall winner"},
		{ID: uuid.New(), Name: "Demo award", CategoryID: &category},
	}
	source := Preview{
		JudgeRunID: uuid.New(),
		Reference:  rubric.Reference{ID: uuid.New(), Version: 1, Digest: criteria.PublicDigest()},
		Binding:    "actual-current-source",
		Criteria:   criteria.PublicDefinition,
		Tallies: []TeamTally{
			{
				TeamID:         second,
				RequiredJudges: 1,
				Rank:           2,
				Total:          Exact{Numerator: "7", Denominator: "1", Display: "7.00"},
				Categories: []CategoryAverage{
					{
						CategoryID: category,
						Average:    Exact{Numerator: "7", Denominator: "1", Display: "7.00"},
					},
				},
			},
			{
				TeamID:         first,
				RequiredJudges: 1,
				Rank:           1,
				Total:          Exact{Numerator: "8", Denominator: "1", Display: "8.00"},
				Categories: []CategoryAverage{
					{
						CategoryID: category,
						Average:    Exact{Numerator: "8", Denominator: "1", Display: "8.00"},
					},
				},
			},
		},
	}
	awards := []Award{
		{
			PrizeID:  criteria.Prizes[0].ID,
			Name:     "Overall winner",
			TeamID:   first,
			Amount:   "125.001",
			Currency: "GBP",
		},
		{PrizeID: criteria.Prizes[1].ID, Name: "Demo award", CategoryID: &category, TeamID: first},
	}
	return source, awards
}

func TestAwardDecisionAllowsOneExactWinnerSeveralConfiguredPrizes(t *testing.T) {
	source, awards := awardDecisionFixture()
	if err := ValidateAwards(source, nil, awards); err != nil {
		t.Fatal("one team can win overall and category prizes, with exact metadata", err)
	}
}

func TestAwardDecisionUsesFullPrecisionAndNeverPrizeNameOrArrayPlacement(t *testing.T) {
	source, awards := awardDecisionFixture()
	// 100/21=4.7619... and81/17=4.7647... both display4.76; the second exact value wins.
	source.Tallies[0].Total = Exact{Numerator: "81", Denominator: "17", Display: "4.76"}
	source.Tallies[0].Categories[0].Average = source.Tallies[0].Total
	source.Tallies[0].RequiredJudges = 17
	source.Tallies[1].Total = Exact{Numerator: "100", Denominator: "21", Display: "4.76"}
	source.Tallies[1].Categories[0].Average = source.Tallies[1].Total
	source.Tallies[1].RequiredJudges = 21
	source.Tallies[0].Rank = 1
	source.Tallies[1].Rank = 1 // An incorrect display-derived rank cannot decide an award.
	for i := range awards {
		awards[i].TeamID = source.Tallies[0].TeamID
	}
	if err := ValidateAwards(source, nil, awards); err != nil {
		t.Fatal("display rounding created a tie", err)
	}
	awards[0].TeamID = source.Tallies[1].TeamID
	if err := ValidateAwards(source, nil, awards); err == nil {
		t.Fatal("lower exact total silently won")
	}

	source, awards = awardDecisionFixture()
	prize := rubric.Prize{ID: uuid.New(), Name: "Second place"}
	source.Criteria.Prizes = append(source.Criteria.Prizes, prize)
	source.Reference.Digest = (rubric.Definition{PublicDefinition: source.Criteria}).PublicDigest()
	awards = append(awards, Award{PrizeID: prize.ID, Name: prize.Name, TeamID: awards[0].TeamID})
	if err := ValidateAwards(source, nil, awards); err != nil {
		t.Fatal("distinct overall prizes cannot share the overall winner", err)
	}
	awards[2].TeamID = source.Tallies[0].TeamID
	if err := ValidateAwards(source, nil, awards); err == nil {
		t.Fatal("prize name/position invented runner-up placement")
	}
}

func TestAwardDecisionRejectsIncompleteOrChangedPrizeMetadata(t *testing.T) {
	cases := map[string]func([]Award) []Award{
		"missing":               func(a []Award) []Award { return a[:1] },
		"duplicate":             func(a []Award) []Award { a[1] = a[0]; return a },
		"foreign prize":         func(a []Award) []Award { a[0].PrizeID = uuid.New(); return a },
		"changed name":          func(a []Award) []Award { a[0].Name = "Unpublished prize"; return a },
		"changed category":      func(a []Award) []Award { a[0].CategoryID = a[1].CategoryID; return a },
		"missing category":      func(a []Award) []Award { a[1].CategoryID = nil; return a },
		"foreign winner":        func(a []Award) []Award { a[0].TeamID = uuid.New(); return a },
		"negative":              func(a []Award) []Award { a[0].Amount = "-1"; return a },
		"exponent":              func(a []Award) []Award { a[0].Amount = "1e3"; return a },
		"not a decimal":         func(a []Award) []Award { a[0].Amount = "NaN"; return a },
		"oversized amount":      func(a []Award) []Award { a[0].Amount = strings.Repeat("9", 65); return a },
		"missing amount":        func(a []Award) []Award { a[0].Amount = ""; return a },
		"missing currency":      func(a []Award) []Award { a[0].Currency = ""; return a },
		"noncanonical currency": func(a []Award) []Award { a[0].Currency = "gbp"; return a },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			source, awards := awardDecisionFixture()
			if err := ValidateAwards(source, nil, change(awards)); err == nil {
				t.Fatal("invalid complete decision accepted")
			}
		})
	}
}

func TestAwardDecisionPreservesTheMaximumExactPublishedAmount(t *testing.T) {
	source, awards := awardDecisionFixture()
	awards[0].Amount = strings.Repeat("9", 64)
	if err := ValidateAwards(source, nil, awards); err != nil {
		t.Fatal("bounded exact metadata was rounded or rejected", err)
	}
}

func nativeAwardTieCase(t *testing.T) (Preview, []Award, []TieState, []judging.Assignment, []Vote) {
	t.Helper()
	source, awards := awardDecisionFixture()
	source.Criteria.RequiredJudgeCount = 3
	source.Reference.Digest = (rubric.Definition{PublicDefinition: source.Criteria}).PublicDigest()
	judges := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	assignments := []judging.Assignment{}
	for i := range source.Tallies {
		source.Tallies[i].RequiredJudges = 3
		source.Tallies[i].Rank = 1
		source.Tallies[i].Total = Exact{Numerator: "8", Denominator: "1", Display: "8.00"}
		source.Tallies[i].Categories[0].Average = source.Tallies[i].Total
		assignments = append(
			assignments,
			judging.Assignment{TeamID: source.Tallies[i].TeamID, JudgeIDs: judges},
		)
	}
	var err error
	source.Ties, err = sourceTies(source)
	if err != nil {
		t.Fatal(err)
	}
	votes := []Vote{}
	for _, tie := range source.Ties {
		for i, judge := range judges {
			choice := awards[0].TeamID
			if i == 2 {
				choice = source.Tallies[0].TeamID
			}
			votes = append(votes, ballot(tie, judge, choice))
		}
	}
	states, err := resolveJudgeTies(source.Ties, assignments, votes)
	if err != nil {
		t.Fatal(err)
	}
	return source, awards, states, assignments, votes
}

func TestAwardDecisionRequiresExactNativeStrictMajorityAndEveryRequiredJudge(t *testing.T) {
	source, awards, states, assignments, votes := nativeAwardTieCase(t)
	if err := ValidateAwards(source, states, awards); err != nil {
		t.Fatal("actual complete native majority did not authorize its winner", err)
	}
	missing, err := resolveJudgeTies(source.Ties, assignments, votes[:len(votes)-1])
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidateAwards(source, missing, awards); !errors.Is(err, ErrIncomplete) {
		t.Fatal("two votes cannot replace all three required judges", err)
	}
	for _, name := range []string{"missing state", "stale binding", "changed candidate", "changed winner", "duplicate state", "blocked state", "incomplete judges"} {
		t.Run(name, func(t *testing.T) {
			source, awards, states, _, _ := nativeAwardTieCase(t)
			switch name {
			case "missing state":
				states = states[:1]
			case "stale binding":
				states[0].Tie.ID = "stale-source"
			case "changed candidate":
				states[0].Tie.TeamIDs = slices.Clone(states[0].Tie.TeamIDs)
				states[0].Tie.TeamIDs[0] = uuid.New()
			case "changed winner":
				id := uuid.New()
				states[0].WinnerID = &id
			case "duplicate state":
				states[1] = states[0]
			case "blocked state":
				states[0].Blocker = "recorded_judge_majority_required"
			case "incomplete judges":
				states[0].RequiredJudgeIDs = states[0].RequiredJudgeIDs[:2]
				states[0].VotesReceived = 2
			}
			if err := ValidateAwards(source, states, awards); !errors.Is(err, ErrIncomplete) {
				t.Fatal("invalid native tie projection accepted", err)
			}
		})
	}
}

func TestAwardDecisionSplitNativeFinalVotesRemainUnresolved(t *testing.T) {
	source, awards, _, assignments, _ := nativeAwardTieCase(t)
	third := source.Tallies[0]
	third.TeamID = uuid.New()
	source.Tallies = append(source.Tallies, third)
	assignments = append(
		assignments,
		judging.Assignment{TeamID: third.TeamID, JudgeIDs: assignments[0].JudgeIDs},
	)
	var err error
	source.Ties, err = sourceTies(source)
	if err != nil {
		t.Fatal(err)
	}
	votes := []Vote{}
	for _, tie := range source.Ties {
		for i, judge := range assignments[0].JudgeIDs {
			votes = append(votes, ballot(tie, judge, tie.TeamIDs[i]))
		}
	}
	states, err := resolveJudgeTies(source.Ties, assignments, votes)
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidateAwards(source, states, awards); !errors.Is(err, ErrIncomplete) {
		t.Fatal("split votes silently chose a UUID winner", err)
	}
}

func TestAwardDecisionUsesSignedQualificationCategoryWinnerOutsideFreshFinal(t *testing.T) {
	source, awards := awardDecisionFixture()
	source.Criteria.Mode = rubric.MentorQualification
	source.Criteria.FinalistCount = 2
	source.Reference.Digest = (rubric.Definition{PublicDefinition: source.Criteria}).PublicDigest()
	category := source.Criteria.Categories[0].ID
	nonFinalist := uuid.New()
	source.Selection = &qualification.Selection{
		RunID:         uuid.New(),
		RoundVersion:  3,
		Reference:     source.Reference,
		SignedBinding: "exact-private-panel-signoff",
		SignerID:      uuid.New(),
		SignedAt:      time.Date(2027, 1, 2, 3, 4, 5, 0, time.UTC),
		FinalistIDs:   []uuid.UUID{source.Tallies[0].TeamID, source.Tallies[1].TeamID},
		Categories: []qualification.CategoryProposal{
			{CategoryID: category, TeamIDs: []uuid.UUID{nonFinalist}},
		},
	}
	source.FinalistsBinding = digest(source.Selection)
	awards[1].TeamID = nonFinalist
	if err := ValidateAwards(source, nil, awards); err != nil {
		t.Fatal("native non-finalist category winner excluded", err)
	}
	source.Selection.Categories[0].TeamIDs = append(
		source.Selection.Categories[0].TeamIDs,
		awards[0].TeamID,
	)
	source.FinalistsBinding = digest(source.Selection)
	// A final judge state cannot repair an unresolved qualification category.
	tie := makeTie(source.Binding, "category", &category, source.Selection.Categories[0].TeamIDs)
	judge := uuid.New()
	if err := ValidateAwards(
		source,
		[]TieState{
			{
				Tie:              tie,
				RequiredJudgeIDs: []uuid.UUID{judge},
				VotesReceived:    1,
				WinnerID:         &nonFinalist,
			},
		},
		awards,
	); !errors.Is(
		err,
		ErrIncomplete,
	) {
		t.Fatal("qualification category reinterpreted as final judge tie", err)
	}
	source.Selection.Categories[0].TeamIDs = []uuid.UUID{nonFinalist}
	source.Selection.SignedBinding = ""
	source.FinalistsBinding = digest(source.Selection)
	if err := ValidateAwards(source, nil, awards); !errors.Is(err, ErrIncomplete) {
		t.Fatal("unsigned qualification category became award authority", err)
	}
}

func TestAwardDecisionBlocksIncompleteSourceAndDoesNotMutateInputs(t *testing.T) {
	source, awards, states, _, _ := nativeAwardTieCase(t)
	before, _ := json.Marshal([]any{source, states, awards})
	if err := ValidateAwards(source, states, awards); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal([]any{source, states, awards})
	if string(before) != string(after) {
		t.Fatal("validation mutated native source/votes/reviewed decision")
	}
	for _, name := range []string{"blocker", "no teams", "missing judge", "changed public rubric", "invalid exact ratio"} {
		t.Run(name, func(t *testing.T) {
			source, awards := awardDecisionFixture()
			switch name {
			case "blocker":
				source.Blockers = []string{"current_required_judge_assessment_needed"}
			case "no teams":
				source.Tallies = nil
			case "missing judge":
				source.Tallies[0].RequiredJudges = 0
			case "changed public rubric":
				source.Criteria.Prizes[0].Name = "Unpublished change"
			case "invalid exact ratio":
				source.Tallies[0].Total.Denominator = "0"
			}
			if err := ValidateAwards(source, nil, awards); !errors.Is(err, ErrIncomplete) {
				t.Fatal("incomplete native source accepted", err)
			}
		})
	}
}
