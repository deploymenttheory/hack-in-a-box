package qualification

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/moderation"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissionfreeze"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestNonFinalistCanWinCategoryAndOneTeamCanWinMultipleWithoutCarryingCheckpointScores(
	t *testing.T,
) {
	values := [][]int{
		{10, 9, 10, 9, 9},
		{9, 8, 9, 8, 8},
		{8, 7, 8, 7, 7},
		{7, 6, 7, 6, 6},
		{1, 10, 1, 1, 1},
	}
	f, service, cp, page := signedQualificationCohort(t, values)
	var err error
	p := page.Round.Proposal
	if p == nil || len(p.Blockers) > 0 || len(p.FinalistIDs) != 3 || len(p.ReserveIDs) != 1 ||
		p.Rankings[0].Total != 47 {
		t.Fatal("incorrect complete proposal", p)
	}
	if !slices.Equal(
		p.FinalistIDs,
		[]uuid.UUID{f.teams[0].team, f.teams[1].team, f.teams[2].team},
	) ||
		p.ReserveIDs[0] != f.teams[3].team {
		t.Fatal("configured total selection wrong", p)
	}
	if p.Categories[1].TeamIDs[0] != f.teams[4].team ||
		p.Categories[0].TeamIDs[0] != f.teams[0].team ||
		p.Categories[2].TeamIDs[0] != f.teams[0].team {
		t.Fatal("category awards were restricted to finalists or one per team", p.Categories)
	}
	page, err = service.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		cp.Definition.RunID,
		f.revision(),
		Command{
			RequestID: uuid.New(),
			Version:   page.Round.Version,
			Action:    "sign_off",
			Reason:    "Panel signs exact current proposal; no award publication",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !page.Current || page.Round.Signoff == nil {
		t.Fatal("exact selection not current")
	}
	for index, team := range f.teams {
		err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
			return service.AuthorizeFinalistInTransaction(t.Context(), tx, f.event, team.team)
		})
		if index < 3 && err != nil || index >= 3 && !errors.Is(err, access.ErrForbidden) {
			t.Fatalf("fresh final eligibility for team%d: %v", index, err)
		}
	}
}

func TestModeratedCardRequiresNativeSignedFinalAndExplicitCompleteEvidence(t *testing.T) {
	f := setup(t)
	huddles := f.huddles()
	cp := f.configure("mentor_final")
	f.freeze()
	f.now = f.config.SubmissionCutoff
	f.submit(cp, f.primary.ID, true)
	f.submit(cp, f.second.ID, true)
	h := f.create(huddles, cp)
	h = f.change(huddles, h, f.second.ID, moderation.Command{Action: "complete_second_review"})
	h = f.change(huddles, h, f.owner.ID, moderation.Command{Action: "open"})
	service := Service{
		Pool:       f.pool,
		Now:        func() time.Time { return f.now },
		Cards:      &f.s,
		Moderation: huddles,
		Freeze:     &submissionfreeze.Service{Pool: f.pool, Now: func() time.Time { return f.now }},
	}
	run := cp.Definition.RunID
	page, err := service.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		run,
		f.revision(),
		Command{
			RequestID: uuid.New(),
			Action:    "configure_panel",
			Reason:    "Explicit current mentor selection panel",
			Panel: &Panel{
				MentorIDs:   []uuid.UUID{f.primary.ID, f.second.ID},
				OrganiserID: f.owner.ID,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	scores := []scorecards.CategoryScore{}
	for _, category := range cp.Criteria.Categories {
		value := category.MinScore
		scores = append(
			scores,
			scorecards.CategoryScore{
				CategoryID: category.ID,
				Value:      &value,
				Evidence: []scorecards.EvidenceReference{
					{Kind: "note", Note: "Explicit final moderated evidence"},
				},
			},
		)
	}
	command := Command{
		RequestID:     uuid.New(),
		Version:       page.Round.Version,
		FreezeBinding: qualificationBinding(page, f.team),
		Action:        "save_card",
		Reason:        "Explicitly moderated final values",
		Card:          &CardInput{TeamID: f.team, Scores: scores},
	}
	if _, err = service.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		run,
		f.revision(),
		command,
	); err == nil {
		t.Fatal("unsigned huddle became a moderated final card")
	}
	h = f.change(
		huddles,
		h,
		f.owner.ID,
		moderation.Command{Action: "sign_off", Reason: "Canonical native independent final proof"},
	)
	stale := command
	stale.FreezeBinding = "a different reviewed freeze identity"
	if _, err = service.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		run,
		f.revision(),
		stale,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("unreviewed freeze identity accepted final moderated values", err)
	}
	page, err = service.Change(t.Context(), f.owner.ID, f.event, run, f.revision(), command)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Round.Cards) != 1 || page.Round.Cards[0].State != "draft" ||
		page.Round.Cards[0].Source.SignoffID != *h.SignoffID ||
		page.Round.Cards[0].Source.FreezeStatus != "not_applicable" {
		t.Fatal("wrong moderated source", page.Round.Cards)
	}
	page, err = service.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		run,
		f.revision(),
		Command{
			RequestID:     uuid.New(),
			Version:       page.Round.Version,
			FreezeBinding: qualificationBinding(page, f.team),
			Action:        "sign_card",
			TeamID:        f.team,
			CardVersion:   page.Round.Cards[0].Version,
			Reason:        "Reviewed complete moderated final card",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if page.Round.Cards[0].State != "signed" {
		t.Fatal("complete final card was not signed")
	}
}

func qualificationBinding(page Page, team uuid.UUID) string {
	for _, item := range page.Teams {
		if item.TeamID == team {
			return item.FreezeBinding
		}
	}
	return ""
}

func signedQualificationCohort(
	t *testing.T,
	values [][]int,
) (*fixture, Service, scorecards.Checkpoint, Page) {
	t.Helper()
	f := setupPool(t, testdb.New(t, true), len(values))
	huddles := f.huddles()
	cp := f.configure("mentor_final")
	f.now = f.config.SubmissionCutoff
	service := Service{
		Pool:       f.pool,
		Now:        func() time.Time { return f.now },
		Cards:      &f.s,
		Moderation: huddles,
		Freeze:     &submissionfreeze.Service{Pool: f.pool, Now: func() time.Time { return f.now }},
	}
	for _, team := range f.teams {
		f.useTeam(team)
		f.freeze()
	}
	page, err := service.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		cp.Definition.RunID,
		f.revision(),
		Command{
			RequestID: uuid.New(),
			Action:    "configure_panel",
			Reason:    "All eligible competing teams",
			Panel: &Panel{
				MentorIDs:   []uuid.UUID{f.primary.ID, f.second.ID},
				OrganiserID: f.owner.ID,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	for index, team := range f.teams {
		f.useTeam(team)
		f.submit(cp, f.primary.ID, true)
		f.submit(cp, f.second.ID, true)
		h := f.create(huddles, cp)
		h = f.change(huddles, h, f.second.ID, moderation.Command{Action: "complete_second_review"})
		h = f.change(huddles, h, f.owner.ID, moderation.Command{Action: "open"})
		f.change(
			huddles,
			h,
			f.owner.ID,
			moderation.Command{
				Action: "sign_off",
				Reason: "Complete actual independent frozen final source",
			},
		)
		scores := []scorecards.CategoryScore{}
		for categoryIndex, category := range cp.Criteria.Categories {
			value := values[index][categoryIndex]
			scores = append(
				scores,
				scorecards.CategoryScore{
					CategoryID: category.ID,
					Value:      &value,
					Evidence: []scorecards.EvidenceReference{
						{Kind: "note", Note: "Explicit moderated frozen-final category evidence"},
					},
				},
			)
		}
		page, err = service.Change(
			t.Context(),
			f.owner.ID,
			f.event,
			cp.Definition.RunID,
			f.revision(),
			Command{
				RequestID:     uuid.New(),
				Version:       page.Round.Version,
				FreezeBinding: qualificationBinding(page, team.team),
				Action:        "save_card",
				Reason:        "Explicit moderated values, not checkpoint aggregation",
				Card:          &CardInput{TeamID: team.team, Scores: scores},
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		card := page.Round.Cards[slices.IndexFunc(page.Round.Cards, func(card FinalCard) bool { return card.TeamID == team.team })]
		page, err = service.Change(
			t.Context(),
			f.owner.ID,
			f.event,
			cp.Definition.RunID,
			f.revision(),
			Command{
				RequestID:     uuid.New(),
				Version:       page.Round.Version,
				FreezeBinding: qualificationBinding(page, team.team),
				Action:        "sign_card",
				TeamID:        team.team,
				CardVersion:   card.Version,
				Reason:        "Explicit complete final mentor signoff",
			},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	page, err = service.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		cp.Definition.RunID,
		f.revision(),
		Command{
			RequestID: uuid.New(),
			Version:   page.Round.Version,
			Action:    "propose",
			Reason:    "Compare complete final mentor cards",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	return f, service, cp, page
}
