package moderation

import (
	"errors"
	"sync"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/google/uuid"
)

func TestDifferentHuddlesCannotCompeteForOneNativeFinalRun(t *testing.T) {
	f, s, first := finalContext(t, true)
	second := f.create(s, first.Checkpoint)
	second = f.change(s, second, f.second.ID, Command{Action: "complete_second_review"})
	second = f.change(s, second, f.owner.ID, Command{Action: "open"})
	revision := f.revision()
	type result struct {
		h   Huddle
		err error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, h := range []Huddle{first, second} {
		wg.Go(func() {
			signed, err := s.Change(t.Context(), f.owner.ID, f.event, f.team, h.Definition.ID, revision, Command{Action: "sign_off", Version: h.Version, Reason: "One canonical final decision"})
			results <- result{signed, err}
		})
	}
	wg.Wait()
	close(results)
	var winner Huddle
	conflicts := 0
	for r := range results {
		if r.err == nil {
			winner = r.h
		} else if errors.Is(r.err, access.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(r.err)
		}
	}
	if winner.SignoffID == nil || conflicts != 1 {
		t.Fatal("competing final decision", winner, conflicts)
	}
	loser := first
	if winner.Definition.ID == first.Definition.ID {
		loser = second
	}
	if _, err := s.Change(t.Context(), f.owner.ID, f.event, f.team, loser.Definition.ID, f.revision(), Command{Action: "sign_off", Version: loser.Version, Reason: "Attempt competing current-generation decision"}); !errors.Is(err, access.ErrConflict) {
		t.Fatal("different huddle created competing final proof", err)
	}
	var proofs, decisions int
	if err := f.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM mentor_scorecard_signoffs WHERE event_id=$1),(SELECT count(*) FROM moderation_final_decisions WHERE event_id=$1)`, f.event).Scan(&proofs, &decisions); err != nil || proofs != 1 || decisions != 1 {
		t.Fatal("orphan/duplicate canonical proof", proofs, decisions, err)
	}
	before := f.revision()
	retried, err := s.Change(t.Context(), f.owner.ID, f.event, f.team, winner.Definition.ID, revision, Command{Action: "sign_off", Version: winner.SignoffContext.Version, Reason: winner.Reason})
	if err != nil || retried.Version != winner.Version || retried.SignoffID == nil || *retried.SignoffID != *winner.SignoffID || f.revision() != before {
		t.Fatal("uncertain retry changed immutable decision", retried, err)
	}
}

func TestExplicitNewNativeRunPreservesOriginalCanonicalFinalDecision(t *testing.T) {
	f, s, h := finalContext(t, true)
	h = f.change(s, h, f.owner.ID, Command{Action: "sign_off", Reason: "First complete canonical final source"})
	old := h
	run := h.Checkpoint.Definition.RunID
	f.rubricChange(rubric.Command{Action: "restart_run", Version: f.criteria.Reference.Version, Purpose: "mentor_final", PreviousRunID: &run})
	var replacement uuid.UUID
	for _, r := range f.criteria.Runs {
		if r.Purpose == "mentor_final" && r.ID != run {
			replacement = r.ID
		}
	}
	if replacement == uuid.Nil {
		t.Fatal("native restart did not create a new run")
	}
	definition := h.Checkpoint.Definition
	definition.ID = uuid.New()
	definition.RunID = replacement
	cp, err := f.s.Configure(t.Context(), f.owner.ID, f.event, f.revision(), scorecards.CheckpointCommand{Definition: definition, Reason: "Explicit fresh final checkpoint after native restart"})
	if err != nil {
		t.Fatal(err)
	}
	f.submit(cp, f.primary.ID, true)
	f.submit(cp, f.second.ID, true)
	h = f.create(s, cp)
	h = f.change(s, h, f.second.ID, Command{Action: "complete_second_review"})
	h = f.change(s, h, f.owner.ID, Command{Action: "open"})
	h = f.change(s, h, f.owner.ID, Command{Action: "sign_off", Reason: "Fresh independent cards on explicit replacement run"})
	if h.SignoffID == nil || *h.SignoffID == *old.SignoffID {
		t.Fatal("new run reused old proof")
	}
	var decisions, oldHistory int
	if err = f.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM moderation_final_decisions WHERE event_id=$1),(SELECT count(*) FROM moderation_huddle_versions WHERE event_id=$1 AND id=$2 AND document->>'state'='signed' AND document->>'signoff_id'=$3)`, f.event, old.Definition.ID, old.SignoffID.String()).Scan(&decisions, &oldHistory); err != nil || decisions != 2 || oldHistory != 1 {
		t.Fatal("restart rewrote or lost canonical history", decisions, oldHistory, err)
	}
}
