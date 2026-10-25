package qualification

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/moderation"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissionfreeze"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
)

func nativeManagedFinal(f *fixture) *submissionfreeze.Service {
	f.t.Helper()
	ctx := f.t.Context()
	deliverable := uuid.New()
	defs := templates.EmptyDefinitions()
	defs.Deliverables = []templates.Deliverable{
		{
			ID:            deliverable,
			Title:         "Actual synthetic managed build",
			Required:      true,
			EvidenceKinds: []string{"repository"},
			Audience:      []string{"mentor", "judge", "organiser"},
		},
	}
	raw, _ := json.Marshal(defs)
	if err := dbgen.New(f.pool).
		SaveEventDefinitions(ctx, dbgen.SaveEventDefinitionsParams{EventID: f.event, Definitions: raw}); err != nil {
		f.t.Fatal(err)
	}
	name := "qualified-" + f.team.String()
	f.exec(
		"INSERT INTO team_repositories(event_id,team_id,organization_id,organization,name,app_id,installation_id,template_repository,requested_by,github_repository_id,state)VALUES($1,$2,99,'synthetic-org',$3,42,55,'template',$4,701,'ready')",
		f.event,
		f.team,
		name,
		f.owner.ID,
	)
	native := submissions.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	page, err := native.Read(ctx, f.member.ID, f.event, f.team)
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err = native.Submit(
		ctx,
		f.member.ID,
		f.event,
		f.team,
		page.Revision,
		submissions.Command{
			Evidence: []submissions.Evidence{
				{
					DeliverableID: deliverable,
					Kind:          "repository",
					Label:         "Actual synthetic managed final",
					URL:           "https://github.com/synthetic-org/" + name,
				},
			},
		},
	); err != nil {
		f.t.Fatal(err)
	}
	f.now = f.config.SubmissionCutoff
	freeze := &submissionfreeze.Service{Pool: f.pool, Now: func() time.Time { return f.now }}
	queue, err := jobs.New(
		f.pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{submissionfreeze.HandlerKind: freeze},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	freeze.Jobs = queue
	if err = freeze.Schedule(ctx, f.event); err != nil {
		f.t.Fatal(err)
	}
	var operation uuid.UUID
	if err = f.pool.QueryRow(ctx, "SELECT id FROM operations WHERE event_id=$1 AND target_id=$2 AND kind=$3", f.event, f.team, submissionfreeze.HandlerKind).
		Scan(&operation); err != nil {
		f.t.Fatal(err)
	}
	// This invokes the real registered capture handler on controlled domain time;
	// it does not claim a future wall-clock job lease or a live provider effect.
	if outcome := freeze.Deliver(
		ctx,
		jobs.Delivery{ID: operation, EventID: f.event, TargetID: f.team},
	); outcome != jobs.Delivered {
		f.t.Fatal("native unavailable-App capture", outcome)
	}
	source, err := freeze.Read(ctx, f.owner.ID, f.event, f.team)
	if err != nil || source.Status != "unresolved" || source.Current != nil ||
		source.Source == nil {
		f.t.Fatal("actual managed source fixture", source, err)
	}
	return freeze
}

func nativeFinalProof(f *fixture, cp scorecards.Checkpoint, huddles *moderation.Service) {
	f.t.Helper()
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
			Reason: "Actual complete fresh independent final proof",
		},
	)
}

func explicitScores(cp scorecards.Checkpoint, value int) []scorecards.CategoryScore {
	out := []scorecards.CategoryScore{}
	for _, category := range cp.Criteria.Categories {
		v := value
		out = append(
			out,
			scorecards.CategoryScore{
				CategoryID: category.ID,
				Value:      &v,
				Evidence: []scorecards.EvidenceReference{
					{Kind: "note", Note: "Explicit newly reviewed moderated final evidence"},
				},
			},
		)
	}
	return out
}

func reviewedCard(f *fixture, s Service, run uuid.UUID, p Page, value int) Page {
	f.t.Helper()
	teamIndex := slices.IndexFunc(
		p.Teams,
		func(team TeamStatus) bool { return team.TeamID == f.team },
	)
	if teamIndex < 0 {
		f.t.Fatal("team missing from current qualification cohort")
	}
	source := p.Teams[teamIndex].Source
	var decision *uuid.UUID
	if source != nil {
		decision = source.RepositoryDecisionID
	}
	p, err := s.Change(
		f.t.Context(),
		f.owner.ID,
		f.event,
		run,
		f.revision(),
		Command{
			RequestID:            uuid.New(),
			Version:              p.Round.Version,
			Action:               "save_card",
			Reason:               "Explicitly reviewed final values for captured build",
			FreezeBinding:        qualificationBinding(p, f.team),
			RepositoryDecisionID: decision,
			Card: &CardInput{
				TeamID: f.team,
				Scores: explicitScores(scorecards.Checkpoint{Criteria: p.Criteria}, value),
			},
		},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	p, err = s.Change(
		f.t.Context(),
		f.owner.ID,
		f.event,
		run,
		f.revision(),
		Command{
			RequestID:            uuid.New(),
			Version:              p.Round.Version,
			Action:               "sign_card",
			Reason:               "Complete explicit moderated card",
			TeamID:               f.team,
			CardVersion:          p.Teams[teamIndex].CardVersion,
			FreezeBinding:        qualificationBinding(p, f.team),
			RepositoryDecisionID: decision,
		},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	return p
}

func TestManagedFreezeRequiresExplicitEligibilityAndFreshProofAfterAmendment(t *testing.T) {
	f := setup(t)
	cp := f.configure("mentor_final")
	freeze := nativeManagedFinal(f)
	huddles := f.huddles()
	nativeFinalProof(f, cp, huddles)
	s := Service{
		Pool:       f.pool,
		Now:        func() time.Time { return f.now },
		Cards:      &f.s,
		Moderation: huddles,
		Freeze:     freeze,
	}
	p, err := s.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		cp.Definition.RunID,
		f.revision(),
		panelCommand(f),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Teams[0].CanDecideEligibility || p.Teams[0].CanSaveCard ||
		!slices.Contains(p.Teams[0].Blockers, "freeze_eligibility_decision_required") {
		t.Fatal("unresolved managed final silently became eligible to score", p.Teams)
	}
	oldBinding := qualificationBinding(p, f.team)
	for _, decision := range []string{"ineligible", "eligible"} {
		p, err = s.Change(
			t.Context(),
			f.owner.ID,
			f.event,
			cp.Definition.RunID,
			f.revision(),
			Command{
				RequestID:     uuid.New(),
				Version:       p.Round.Version,
				Action:        "decide_eligibility",
				TeamID:        f.team,
				FreezeBinding: qualificationBinding(p, f.team),
				Decision:      decision,
				Reason:        "Explicit panel eligibility review of unresolved managed evidence",
				Evidence: []scorecards.EvidenceReference{
					{
						Kind: "note",
						Note: "Recorded supervised eligibility decision; code receipt remains unresolved",
					},
				},
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if (decision == "eligible") != p.Teams[0].Eligible {
			t.Fatal("explicit eligibility was inferred", p.Teams)
		}
	}
	p = reviewedCard(f, s, cp.Definition.RunID, p, 3)
	oldCard := p.Round.Cards[0]
	oldReceipt := *p.Receipt
	source, err := freeze.Read(t.Context(), f.owner.ID, f.event, f.team)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = freeze.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		f.team,
		submissionfreeze.Command{
			RequestID:          uuid.New(),
			Action:             "reconcile",
			ExpectedRevision:   source.Revision,
			SourceID:           source.Source.ID,
			NativeRevision:     source.NativeRevision,
			SubmissionRevision: *source.CurrentSubmissionRevision,
			SHA:                strings.Repeat("a", 40),
			Reason:             "Actual reviewed independent build evidence",
			ExternalEvidence:   "Explicit synthetic independent historical cutoff record",
		},
	); err != nil {
		t.Fatal(err)
	}
	current, err := s.Read(t.Context(), f.owner.ID, f.event, cp.Definition.RunID)
	if err != nil || qualificationBinding(current, f.team) == oldBinding ||
		len(current.Round.Cards) != 0 ||
		current.Teams[0].CanSaveCard ||
		current.Teams[0].CanSignCard {
		t.Fatal(
			"new same-final code identity silently reused earlier moderated values",
			current,
			err,
		)
	}
	if _, err = s.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		cp.Definition.RunID,
		f.revision(),
		Command{
			RequestID:            uuid.New(),
			Version:              current.Round.Version,
			Action:               "save_card",
			Reason:               "Attempt to retarget an older signed source",
			FreezeBinding:        qualificationBinding(current, f.team),
			RepositoryDecisionID: current.Teams[0].Source.RepositoryDecisionID,
			Card: &CardInput{
				TeamID:  f.team,
				Version: oldCard.Version,
				Scores:  oldCard.Scores,
			},
		},
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("old canonical source silently retargeted", err)
	}
	oldRun := cp.Definition.RunID
	criteria, err := f.rubric.Configuration(t.Context(), f.owner.ID, f.event, 0)
	if err != nil {
		t.Fatal(err)
	}
	criteria, err = f.rubric.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		criteria.Revision,
		rubric.Command{
			Action:        "restart_run",
			Purpose:       "mentor_final",
			PreviousRunID: &oldRun,
			Version:       criteria.Reference.Version,
			Reason:        "Explicit fresh independent final review after code evidence correction",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	var newRun uuid.UUID
	for _, run := range criteria.Runs {
		if run.Purpose == "mentor_final" {
			newRun = run.ID
		}
	}
	zone, _ := time.LoadLocation(f.config.Timezone)
	freshCP, err := f.s.Configure(
		t.Context(),
		f.owner.ID,
		f.event,
		f.revision(),
		scorecards.CheckpointCommand{
			Definition: scorecards.CheckpointDefinition{
				ID:            uuid.New(),
				Name:          "Fresh reviewed final",
				RunID:         newRun,
				OpensWallTime: f.config.SubmissionCutoff.In(zone).Format("2006-01-02T15:04:05"),
				ClosesWallTime: f.config.LiveEnd.Add(time.Hour).
					In(zone).
					Format("2006-01-02T15:04:05"),
			},
			Reason: "Explicit fresh native review context",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	nativeFinalProof(f, freshCP, huddles)
	fresh, err := s.Change(t.Context(), f.owner.ID, f.event, newRun, f.revision(), panelCommand(f))
	if err != nil {
		t.Fatal(err)
	}
	fresh = reviewedCard(f, s, newRun, fresh, 5)
	if len(fresh.Round.Cards) != 1 ||
		fresh.Round.Cards[0].Source.SignoffID == oldCard.Source.SignoffID ||
		fresh.Round.Cards[0].Source.FreezeStatus != "completed" ||
		*fresh.Round.Cards[0].Scores[0].Value != 5 {
		t.Fatal("fresh correction carried old proof or scores", fresh.Round.Cards)
	}
	// Historical authority must never restore current scoring or permit replay.
	if _, err = s.Read(
		t.Context(),
		f.owner.ID,
		f.event,
		oldRun,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("superseded run regained current reader", err)
	}
	if _, err = s.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		oldRun,
		f.revision(),
		Command{
			RequestID: uuid.New(),
			Action:    "propose",
			Version:   oldReceipt.Version,
			Reason:    "Attempt current selection from retained history",
		},
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("superseded run regained command authority", err)
	}
	history, err := s.History(t.Context(), f.owner.ID, f.event, oldRun, nil, 20)
	if err != nil || len(history.Items) == 0 {
		t.Fatal("explicit correction erased authorised old history", history, err)
	}
	found := false
	for _, item := range history.Items {
		if item.Current {
			t.Fatal("superseded selection became current")
		}
		for _, card := range item.Snapshot.Cards {
			if card.Version == oldCard.Version && card.Source.FreezeBinding == oldBinding &&
				*card.Scores[0].Value == 3 {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("old exact-source values not retained in authorised history")
	}
	ack, err := s.OriginalReceipt(t.Context(), f.owner.ID, f.event, oldRun, oldReceipt.RequestID)
	if err != nil || ack.Receipt == nil || *ack.Receipt != oldReceipt {
		t.Fatal("native run correction lost immutable original ACK", ack, err)
	}
	completedCard := fresh.Round.Cards[0]
	source, err = freeze.Read(t.Context(), f.owner.ID, f.event, f.team)
	if err != nil || source.Current == nil {
		t.Fatal("completed native source missing", source, err)
	}
	prior := source.Current.ID
	if _, err = freeze.Change(t.Context(), f.owner.ID, f.event, f.team, submissionfreeze.Command{
		RequestID:          uuid.New(),
		Action:             "amend",
		ExpectedRevision:   source.Revision,
		SourceID:           source.Source.ID,
		NativeRevision:     source.NativeRevision,
		SubmissionRevision: *source.CurrentSubmissionRevision,
		PriorDecisionID:    &prior,
		SHA:                strings.Repeat("b", 40),
		Reason:             "Explicit reviewed amendment of the same native final",
		ExternalEvidence:   "Independent synthetic historical record corrects the cutoff SHA",
	}); err != nil {
		t.Fatal(err)
	}
	stale, err := s.Read(t.Context(), f.owner.ID, f.event, newRun)
	if err != nil || len(stale.Round.Cards) != 0 || stale.Current || stale.Teams[0].CanSaveCard ||
		stale.Teams[0].CanSignCard ||
		qualificationBinding(stale, f.team) == completedCard.Source.FreezeBinding {
		t.Fatal("completed SHA amendment silently reused moderated values", stale, err)
	}
	retained, err := s.History(t.Context(), f.owner.ID, f.event, newRun, nil, 20)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, item := range retained.Items {
		for _, card := range item.Snapshot.Cards {
			if card.Source.FreezeBinding == completedCard.Source.FreezeBinding &&
				*card.Scores[0].Value == 5 {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("SHA amendment erased original historical values")
	}
	f.exec(
		"DELETE FROM event_memberships WHERE event_id=$1 AND person_id=$2",
		f.event,
		f.primary.ID,
	)
	if _, err = s.History(
		t.Context(),
		f.primary.ID,
		f.event,
		oldRun,
		nil,
		20,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("role loss retained superseded private history", err)
	}
	if _, err = s.OriginalReceipt(
		t.Context(),
		f.primary.ID,
		f.event,
		oldRun,
		oldReceipt.RequestID,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("role loss retained historical ACK authority", err)
	}
}
