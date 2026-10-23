package moderation

import (
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
)

func finalContext(t *testing.T, complete bool) (*fixture, *Service, Huddle) {
	t.Helper()
	f := setup(t)
	s := f.huddles()
	cp := f.configure("mentor_final")
	f.freeze()
	f.now = f.config.SubmissionCutoff
	f.submit(cp, f.primary.ID, complete)
	f.submit(cp, f.second.ID, true)
	h := f.create(s, cp)
	h = f.change(s, h, f.second.ID, Command{Action: "complete_second_review"})
	h = f.change(s, h, f.owner.ID, Command{Action: "open"})
	return f, s, h
}

func TestNativeSignoffAndHuddleDecisionRollbackAtomically(t *testing.T) {
	f, s, h := finalContext(t, true)
	f.exec(
		`CREATE FUNCTION synthetic_reject_signed_huddle()RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.document->>'state'='signed' THEN RAISE EXCEPTION 'synthetic final history persistence failure';END IF;RETURN NEW;END $$`,
	)
	f.exec(
		`CREATE TRIGGER synthetic_reject_signed BEFORE INSERT ON moderation_huddle_versions FOR EACH ROW EXECUTE FUNCTION synthetic_reject_signed_huddle()`,
	)
	if _, err := s.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		f.team,
		h.Definition.ID,
		f.revision(),
		Command{
			Action:  "sign_off",
			Version: h.Version,
			Reason:  "Failure must roll back native signoff",
		},
	); err == nil {
		t.Fatal("forced huddle persistence failure ignored")
	}
	var retained int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM mentor_scorecard_signoffs WHERE event_id=$1`, f.event).
		Scan(&retained); err != nil ||
		retained != 0 {
		t.Fatal("orphan native signoff escaped outer transaction", retained, err)
	}
	current, err := s.Read(t.Context(), f.owner.ID, f.event, f.team, h.Definition.ID)
	if err != nil || current.Version != h.Version || current.State != "open" {
		t.Fatal("failed finalisation changed huddle", err)
	}
}

func TestConcurrentSignoffHasOneNativeProofAndOneDecision(t *testing.T) {
	f, s, h := finalContext(t, true)
	revision := f.revision()
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			_, err := s.Change(
				t.Context(),
				f.owner.ID,
				f.event,
				f.team,
				h.Definition.ID,
				revision,
				Command{
					Action:  "sign_off",
					Version: h.Version,
					Reason:  "Concurrent explicit final decision",
				},
			)
			results <- err
		})
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, access.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	// The second identical request is an uncertain-outcome retry, not a new decision.
	if successes != 2 || conflicts != 0 {
		t.Fatalf("concurrent signoff result %d/%d", successes, conflicts)
	}
	var proofs, decisions int
	if err := f.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM mentor_scorecard_signoffs WHERE event_id=$1),(SELECT count(*) FROM moderation_huddle_versions WHERE event_id=$1 AND document->>'state'='signed')`, f.event).
		Scan(&proofs, &decisions); err != nil ||
		proofs != 1 ||
		decisions != 1 {
		t.Fatal("duplicate signoff proof/history", proofs, decisions, err)
	}
}

func TestPendingIndependentFinalCardCannotBeCompletedByModerationProposal(t *testing.T) {
	f, s, h := finalContext(t, false)
	cards, err := s.Panel(t.Context(), f.owner.ID, f.event, f.team, h.Definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	var pending scorecards.Card
	for _, c := range cards {
		if c.AuthorID == f.primary.ID {
			pending = c
		}
	}
	value := pending.Criteria.Categories[0].MinScore
	proposal := uuid.New()
	h = f.change(
		s,
		h,
		f.owner.ID,
		Command{
			Action:        "propose_change",
			ProposalID:    proposal,
			Card:          pending.Reference,
			CategoryID:    pending.Scores[0].CategoryID,
			ProposedValue: &value,
			Evidence: []scorecards.EvidenceReference{
				{Kind: "note", Note: "A proposal is not the author's independent review"},
			},
			Reason: "Do not silently replace pending independent scores",
		},
	)
	h = f.change(
		s,
		h,
		f.owner.ID,
		Command{
			Action:     "accept_change",
			ProposalID: proposal,
			Reason:     "Record proposal while independent review remains incomplete",
		},
	)
	if _, err = s.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		f.team,
		h.Definition.ID,
		f.revision(),
		Command{
			Action:  "sign_off",
			Version: h.Version,
			Reason:  "Pending categories must block native finalisation",
		},
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("accepted proposal filled independent pending card", err)
	}
}

func TestSubmittedCardChangeAndCurrentRoleLossInvalidatePanelContext(t *testing.T) {
	f, s, h := finalContext(t, true)
	cards, err := s.Panel(t.Context(), f.owner.ID, f.event, f.team, h.Definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	var own scorecards.Card
	for _, c := range cards {
		if c.AuthorID == f.primary.ID {
			own = c
		}
	}
	command := f.command(h.Checkpoint, "submit", true)
	command.Version = own.Reference.Version
	if _, err = f.s.Change(
		t.Context(),
		f.primary.ID,
		f.event,
		f.team,
		f.revision(),
		command,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Panel(
		t.Context(),
		f.owner.ID,
		f.event,
		f.team,
		h.Definition.ID,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("huddle silently adopted a later independent review", err)
	}
	f.exec(
		`UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mentor'`,
		f.event,
		f.second.ID,
	)
	if _, err = s.History(
		t.Context(),
		f.second.ID,
		f.event,
		f.team,
		h.Definition.ID,
		nil,
		20,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("role loss retained private decision history", err)
	}
}

func TestInactiveDisbandedOrWithdrawnFrozenOutputCannotFinalise(t *testing.T) {
	for _, state := range []string{"disbanded", "inactive", "withdrawn"} {
		t.Run(state, func(t *testing.T) {
			f, s, h := finalContext(t, true)
			switch state {
			case "disbanded":
				f.exec(
					`UPDATE hack_teams SET state='disbanded' WHERE event_id=$1 AND id=$2`,
					f.event,
					f.team,
				)
			case "inactive":
				f.exec(
					`UPDATE competition_entries SET active=false WHERE event_id=$1 AND team_id=$2`,
					f.event,
					f.team,
				)
			case "withdrawn":
				f.exec(
					`UPDATE submission_states SET outcome='withdrawn' WHERE event_id=$1 AND team_id=$2`,
					f.event,
					f.team,
				)
			}
			if _, err := s.Change(
				t.Context(),
				f.owner.ID,
				f.event,
				f.team,
				h.Definition.ID,
				f.revision(),
				Command{
					Action:  "sign_off",
					Version: h.Version,
					Reason:  "Unused output must not be finalised",
				},
			); err == nil {
				t.Fatal("invalid competition output finalised")
			}
			var count int
			if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM mentor_scorecard_signoffs WHERE event_id=$1`, f.event).
				Scan(&count); err != nil ||
				count != 0 {
				t.Fatal("invalid output retained final proof", count, err)
			}
		})
	}
}
