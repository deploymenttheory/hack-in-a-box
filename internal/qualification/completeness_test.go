package qualification

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/feedback"
	"github.com/deploymenttheory/hack-in-a-box/internal/moderation"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissionfreeze"
)

func qualificationGapAction(f *fixture) (*feedback.Service, uuid.UUID, feedback.ActionView) {
	f.t.Helper()
	native := &feedback.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	id := uuid.New()
	p, err := native.Change(
		f.t.Context(),
		f.owner.ID,
		f.event,
		id,
		0,
		uuid.New(),
		feedback.Command{
			Action: "save",
			Input: &feedback.Input{
				TeamID:   f.team,
				Category: "competition_output",
				Title:    "Native output evidence review",
				Body:     "Support action requires separate formal evidence review",
				Sources:  []feedback.Source{},
			},
		},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, action := range []string{"review", "publish"} {
		p, err = native.Change(
			f.t.Context(),
			f.owner.ID,
			f.event,
			id,
			p.CurrentVersion,
			uuid.New(),
			feedback.Command{Action: action},
		)
		if err != nil {
			f.t.Fatal(err)
		}
	}
	task, err := native.ChangeAction(
		f.t.Context(),
		f.owner.ID,
		f.event,
		id,
		uuid.New(),
		0,
		uuid.New(),
		feedback.ActionCommand{
			Action: "details",
			Input: &feedback.ActionInput{
				Reviewed: true,
				Title:    "Review frozen build evidence",
				Body:     "Operational completion is separate from formal resolution",
				OwnerID:  f.member.ID,
				DueAt:    f.now.Add(time.Hour),
			},
		},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	return native, id, task
}

func TestPendingIndependentReviewGapAndIncompleteExplicitValuesCannotSignFinalCard(t *testing.T) {
	f := setup(t)
	cp := f.configure("mentor_final")
	f.freeze()
	f.now = f.config.SubmissionCutoff
	huddles := f.huddles()
	s := Service{
		Pool:       f.pool,
		Now:        func() time.Time { return f.now },
		Cards:      &f.s,
		Moderation: huddles,
		Freeze:     &submissionfreeze.Service{Pool: f.pool, Now: func() time.Time { return f.now }},
	}
	f.submit(cp, f.primary.ID, true)
	h := f.create(huddles, cp)
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
	assertBlocked := func() {
		t.Helper()
		p, err = s.Read(t.Context(), f.owner.ID, f.event, cp.Definition.RunID)
		if err != nil {
			t.Fatal(err)
		}
		if p.Teams[0].Source != nil || p.Teams[0].CanSaveCard || p.Teams[0].CanSignCard {
			t.Fatal("incomplete independent evidence advertised final card", p.Teams)
		}
		_, err = s.Change(
			t.Context(),
			f.owner.ID,
			f.event,
			cp.Definition.RunID,
			f.revision(),
			Command{
				RequestID:     uuid.New(),
				Version:       p.Round.Version,
				Action:        "save_card",
				Reason:        "Attempt final before required source complete",
				FreezeBinding: qualificationBinding(p, f.team),
				Card:          &CardInput{TeamID: f.team, Scores: explicitScores(cp, 7)},
			},
		)
		if !errors.Is(err, access.ErrConflict) {
			t.Fatal("incomplete native source allowed moderated final", err)
		}
	}
	assertBlocked() // Required second independent card is still absent.
	f.submit(cp, f.second.ID, true)
	h = f.change(huddles, h, f.second.ID, moderation.Command{Action: "complete_second_review"})
	h = f.change(huddles, h, f.owner.ID, moderation.Command{Action: "open"})
	native, id, task := qualificationGapAction(f)
	gap := uuid.New()
	h = f.change(
		huddles,
		h,
		f.owner.ID,
		moderation.Command{
			Action:        "add_gap",
			GapID:         gap,
			ActionID:      task.Action.ID,
			ActionVersion: task.Action.Version,
			Reason:        "Required formal supporting evidence is unresolved",
		},
	)
	assertBlocked()
	if _, err = huddles.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		f.team,
		h.Definition.ID,
		f.revision(),
		moderation.Command{
			Action:  "sign_off",
			Version: h.Version,
			Reason:  "Attempt unresolved gap signoff",
		},
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("unresolved gap signed native source", err)
	}
	task, err = native.ChangeAction(
		t.Context(),
		f.member.ID,
		f.event,
		id,
		task.Action.ID,
		task.Action.Version,
		uuid.New(),
		feedback.ActionCommand{Action: "status", Status: "done"},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertBlocked() // A done support action does not resolve formal evidence.
	h = f.change(
		huddles,
		h,
		f.owner.ID,
		moderation.Command{
			Action:        "resolve_gap",
			GapID:         gap,
			ActionVersion: task.Action.Version,
			Evidence: []scorecards.EvidenceReference{
				{Kind: "note", Note: "Independent formal review of frozen evidence"},
			},
			Reason: "Explicit formal gap resolution",
		},
	)
	f.change(
		huddles,
		h,
		f.owner.ID,
		moderation.Command{
			Action: "sign_off",
			Reason: "Complete native independent final evidence",
		},
	)
	p, err = s.Read(t.Context(), f.owner.ID, f.event, cp.Definition.RunID)
	if err != nil {
		t.Fatal(err)
	}
	for index, scores := range [][]scorecards.CategoryScore{explicitScores(cp, 7)[:1], {{CategoryID: cp.Criteria.Categories[0].ID, Pending: true}}} {
		p, err = s.Change(
			t.Context(),
			f.owner.ID,
			f.event,
			cp.Definition.RunID,
			f.revision(),
			Command{
				RequestID:     uuid.New(),
				Version:       p.Round.Version,
				Action:        "save_card",
				Reason:        "Explicit incomplete draft retained for further review",
				FreezeBinding: qualificationBinding(p, f.team),
				Card:          &CardInput{TeamID: f.team, Version: int64(index), Scores: scores},
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if p.Teams[0].CanSignCard {
			t.Fatal("incomplete draft advertised signoff")
		}
		_, err = s.Change(
			t.Context(),
			f.owner.ID,
			f.event,
			cp.Definition.RunID,
			f.revision(),
			Command{
				RequestID:     uuid.New(),
				Version:       p.Round.Version,
				Action:        "sign_card",
				Reason:        "Attempt incomplete moderated card",
				FreezeBinding: qualificationBinding(p, f.team),
				TeamID:        f.team,
				CardVersion:   p.Teams[0].CardVersion,
			},
		)
		if !errors.Is(err, access.ErrConflict) {
			t.Fatal("partial/pending explicit values signed", err)
		}
	}
	// A rejected signoff cannot create an immutable signed decision or receipt.
	var signed int
	if err = f.pool.QueryRow(t.Context(), "SELECT count(*) FROM mentor_qualification_rounds WHERE event_id=$1 AND document->'cards' @> '[{\"state\":\"signed\"}]'::jsonb", f.event).
		Scan(&signed); err != nil ||
		signed != 0 {
		t.Fatal("incomplete source retained signed final", signed, err)
	}
}
