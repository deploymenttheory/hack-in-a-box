package app

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/deploymenttheory/hack-in-a-box/internal/results"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestNativeMentorCategoryAdvisoryNeverCountsAsJudgeVote(t *testing.T) {
	f, second := newTiedResultsFixture(t)
	r, ctx := f.runtime, t.Context()
	s := f.service
	preparations := 0
	prepare := func() results.Page {
		preparations++
		p, e := s.Read(ctx, r.owner.ID, r.eventID)
		resultsSourceRequire(t, e)
		t.Logf(
			"prepare %d revision=%d version=%d binding=%s blockers=%v",
			preparations,
			p.Revision,
			p.Round.Version,
			p.Preview.Binding,
			p.Preview.Blockers,
		)
		p, e = s.Change(
			ctx,
			r.owner.ID,
			r.eventID,
			p.Revision,
			results.Command{
				RequestID:     uuid.New(),
				Version:       p.Round.Version,
				Action:        "prepare",
				SourceBinding: p.Preview.Binding,
				Reason:        "Prepare native category deliberation",
			},
		)
		resultsSourceRequire(t, e)
		return p
	}
	initial := prepare()
	own, e := s.MentorAdvisories(ctx, r.mentor.ID, r.eventID)
	resultsSourceRequire(t, e)
	if len(own.Targets) != len(initial.Preview.Criteria.Categories) {
		t.Fatal("mentor target scope", own)
	}
	target := own.Targets[0]
	if target.TeamID != r.teamID || target.CategoryName == "" || target.SubmissionRevision < 1 ||
		target.Own != nil {
		t.Fatal("mentor received wrong frozen context", target)
	}
	raw, e := json.Marshal(own)
	resultsSourceRequire(t, e)
	for _, secret := range []string{resultsMentorPrivateNote, "PRIVATE-INDEPENDENT-JUDGE-PROSE", `"scores":`, `"rank":`, second.String()} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("mentor projection disclosed unrelated/private source", secret)
		}
	}
	command := func(p results.AdvisoryPage, views string) results.AdvisoryCommand {
		for _, v := range p.Targets {
			if v.TieID == target.TieID && v.TeamID == target.TeamID {
				return results.AdvisoryCommand{
					RequestID:          uuid.New(),
					Action:             "advise",
					Version:            v.Version,
					SourceBinding:      p.SourceBinding,
					TieID:              v.TieID,
					TeamID:             v.TeamID,
					SubmissionRevision: v.SubmissionRevision,
					Views:              views,
					Reason:             "Own deliberately shared category observation",
				}
			}
		}
		t.Fatal("current target missing")
		return results.AdvisoryCommand{}
	}
	c := command(
		own,
		"ADVISORY-ORIGINAL: observed a clear recovery journey; judges decide independently.",
	)
	accepted, e := s.Advise(ctx, r.mentor.ID, r.eventID, own.Revision, c)
	resultsSourceRequire(t, e)
	replay, e := s.Advise(ctx, r.mentor.ID, r.eventID, own.Revision, c)
	resultsSourceRequire(t, e)
	if !reflect.DeepEqual(accepted.Receipt, replay.Receipt) || accepted.Receipt.Version != 1 {
		t.Fatal("lost exact original advisory acknowledgement")
	}
	recovery, e := s.MentorAdvisoryReceipt(ctx, r.mentor.ID, r.eventID, c.RequestID)
	resultsSourceRequire(t, e)
	if !reflect.DeepEqual(recovery.Receipt, accepted.Receipt) {
		t.Fatal("safe receipt mismatch")
	}
	changed := c
	changed.Views = "Different advice under same request"
	if _, e = s.Advise(
		ctx,
		r.mentor.ID,
		r.eventID,
		own.Revision,
		changed,
	); !errors.Is(
		e,
		access.ErrConflict,
	) {
		t.Fatal("changed intent reuse accepted", e)
	}
	after, e := s.Read(ctx, r.owner.ID, r.eventID)
	resultsSourceRequire(t, e)
	if !reflect.DeepEqual(initial.Preview, after.Preview) ||
		!reflect.DeepEqual(initial.Round, after.Round) ||
		len(after.Advisories) != 1 {
		t.Fatal("advice changed native score/vote/confirmation source", after)
	}
	judge, e := s.Ballots(ctx, f.judges[0].ID, r.eventID)
	resultsSourceRequire(t, e)
	for _, tie := range judge.Ties {
		if tie.State.VotesReceived != 0 || tie.State.WinnerID != nil ||
			len(tie.State.RequiredJudgeIDs) != 2 {
			t.Fatal("advice counted as ballot", tie)
		}
		if tie.State.Tie.ID == target.TieID && len(tie.Advisories) != 1 {
			t.Fatal("judge missing scoped advisory", tie)
		}
		if tie.State.Tie.Kind == "final" && len(tie.Advisories) != 0 {
			t.Fatal("category advisory widened to overall tie")
		}
	}
	outsider := testdb.Person(t, r.pool)
	resultsSourceRequire(
		t,
		dbgen.New(r.pool).
			SetMembership(ctx, dbgen.SetMembershipParams{EventID: r.eventID, PersonID: outsider.ID, Role: "mentor", Active: true}),
	)
	other, e := s.MentorAdvisories(ctx, outsider.ID, r.eventID)
	resultsSourceRequire(t, e)
	if len(other.Targets) != 0 {
		t.Fatal("unassigned mentor received private targets", other)
	}
	if _, e = s.MentorAdvisoryReceipt(
		ctx,
		outsider.ID,
		r.eventID,
		c.RequestID,
	); !errors.Is(
		e,
		access.ErrForbidden,
	) {
		t.Fatal("another mentor obtained original receipt", e)
	}
	if _, e = s.Advise(
		ctx,
		outsider.ID,
		r.eventID,
		accepted.Revision,
		c,
	); !errors.Is(
		e,
		access.ErrForbidden,
	) {
		t.Fatal("another mentor replaced contribution", e)
	}
	if _, e = s.MentorAdvisories(
		ctx,
		f.judges[0].ID,
		r.eventID,
	); !errors.Is(
		e,
		access.ErrForbidden,
	) {
		t.Fatal("judge read mentor editor", e)
	}
	foreign := command(accepted, "Foreign team")
	foreign.TeamID = second
	if _, e = s.Advise(
		ctx,
		r.mentor.ID,
		r.eventID,
		accepted.Revision,
		foreign,
	); !errors.Is(
		e,
		access.ErrForbidden,
	) {
		t.Fatal("mentor advised unassigned team", e)
	}
	final := command(accepted, "Overall vote masquerading as advice")
	for _, tie := range initial.Preview.Ties {
		if tie.Kind == "final" {
			final.TieID = tie.ID
		}
	}
	if _, e = s.Advise(
		ctx,
		r.mentor.ID,
		r.eventID,
		accepted.Revision,
		final,
	); !errors.Is(
		e,
		access.ErrForbidden,
	) {
		t.Fatal("overall tie accepted advisory", e)
	}
	correction := command(accepted, "ADVISORY-CORRECTED: record the observed limitation as well.")
	corrected, e := s.Advise(ctx, r.mentor.ID, r.eventID, accepted.Revision, correction)
	resultsSourceRequire(t, e)
	var current *results.MentorAdvisory
	for _, v := range corrected.Targets {
		if v.TieID == target.TieID {
			current = v.Own
		}
	}
	if current == nil || current.Version != 2 || current.PreviousVersion != 1 ||
		current.AuthorID != r.mentor.ID ||
		current.Views != correction.Views {
		t.Fatal("correction provenance missing", current)
	}
	var retained int
	resultsSourceRequire(
		t,
		r.pool.QueryRow(ctx, "SELECT count(*) FROM result_mentor_advisory_versions WHERE event_id=$1 AND tie_id=$2 AND document::text LIKE '%ADVISORY-%'", r.eventID, target.TieID).
			Scan(&retained),
	)
	if retained != 2 {
		t.Fatal("original correction history erased", retained)
	}
	if _, e = r.pool.Exec(
		ctx,
		"DELETE FROM result_mentor_advisory_versions WHERE event_id=$1",
		r.eventID,
	); e == nil {
		t.Fatal("advisory history mutable")
	}
	if _, e = s.Advise(
		ctx,
		r.mentor.ID,
		r.eventID,
		corrected.Revision,
		command(accepted, "Stale correction"),
	); !errors.Is(
		e,
		access.ErrConflict,
	) {
		t.Fatal("stale version accepted", e)
	}
	// Two competing corrections from one reviewed revision serialize; precisely
	// one is retained. Neither may add a vote or alter the result version.
	first, secondCommand := command(
		corrected,
		"Concurrent first",
	), command(
		corrected,
		"Concurrent second",
	)
	var group sync.WaitGroup
	outcomes := make(chan error, 2)
	for _, input := range []results.AdvisoryCommand{first, secondCommand} {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := s.Advise(ctx, r.mentor.ID, r.eventID, corrected.Revision, input)
			outcomes <- err
		}()
	}
	group.Wait()
	close(outcomes)
	successes, conflicts := 0, 0
	for err := range outcomes {
		if err == nil {
			successes++
		} else if errors.Is(err, access.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal("concurrent correction escaped native revision fence", successes, conflicts)
	}
	// Only actual judge ballots settle the tie: one ballot is incomplete; a
	// full split stays unresolved; a deliberate replacement yields strict majority.
	vote := func(judge int, team uuid.UUID) results.JudgeTie {
		p, err := s.Ballots(ctx, f.judges[judge].ID, r.eventID)
		resultsSourceRequire(t, err)
		p, err = s.Vote(
			ctx,
			f.judges[judge].ID,
			r.eventID,
			p.Revision,
			results.Command{
				RequestID:     uuid.New(),
				Version:       p.Version,
				Action:        "vote",
				SourceBinding: p.SourceBinding,
				TieID:         target.TieID,
				TeamID:        team,
				Evidence: []results.VoteEvidence{
					{TeamID: team, DeliverableID: &f.deliverable},
				},
				Reason: "Independent judge deliberation",
			},
		)
		resultsSourceRequire(t, err)
		for _, tie := range p.Ties {
			if tie.State.Tie.ID == target.TieID {
				return tie
			}
		}
		t.Fatal("native ballot absent")
		return results.JudgeTie{}
	}
	partial := vote(0, r.teamID)
	if partial.State.WinnerID != nil || partial.State.VotesReceived != 1 {
		t.Fatal("advice completed judge panel", partial)
	}
	split := vote(1, second)
	if split.State.WinnerID != nil || split.State.Blocker != "recorded_judge_majority_required" {
		t.Fatal("advice resolved split", split)
	}
	majority := vote(1, r.teamID)
	if majority.State.WinnerID == nil || *majority.State.WinnerID != r.teamID ||
		majority.State.VotesReceived != 2 {
		t.Fatal("independent majority absent", majority)
	}
	// Current author role loss removes advisory from every native current read.
	resultsSourceRequire(
		t,
		dbgen.New(r.pool).
			SetMembership(ctx, dbgen.SetMembershipParams{EventID: r.eventID, PersonID: r.mentor.ID, Role: "mentor", Active: false}),
	)
	if _, e = s.MentorAdvisories(ctx, r.mentor.ID, r.eventID); !errors.Is(e, access.ErrForbidden) {
		t.Fatal("revoked mentor read editor", e)
	}
	if recovered, e := s.MentorAdvisoryReceipt(
		ctx,
		r.mentor.ID,
		r.eventID,
		c.RequestID,
	); !errors.Is(e, access.ErrForbidden) ||
		recovered.Receipt != nil {
		t.Fatal("revoked mentor read receipt", recovered, e)
	}
	p, e := s.Read(ctx, r.owner.ID, r.eventID)
	resultsSourceRequire(t, e)
	if len(p.Advisories) != 0 {
		t.Fatal("revoked author advisory remained current")
	}
	// Source correction cannot resurrect a previous binding, even after the
	// mentor's role is restored. Safe ACK metadata is not private source data.
	resultsSourceRequire(
		t,
		dbgen.New(r.pool).
			SetMembership(ctx, dbgen.SetMembershipParams{EventID: r.eventID, PersonID: r.mentor.ID, Role: "mentor", Active: true}),
	)
	f.reopen(t, 0)
	if p, e := s.MentorAdvisories(ctx, r.mentor.ID, r.eventID); e == nil || len(p.Targets) != 0 {
		t.Fatal("stale source advisory remained available", p, e)
	}
	recovered, e := s.MentorAdvisoryReceipt(ctx, r.mentor.ID, r.eventID, c.RequestID)
	resultsSourceRequire(t, e)
	if !reflect.DeepEqual(recovered.Receipt, accepted.Receipt) {
		t.Fatal("source teardown lost original safe acknowledgement")
	}
	f.card(t, 0, 6, "submit")
	f.round(t, judging.RoundCommand{Action: "open_deliberation"})
	next := prepare()
	if next.Preview.Binding == initial.Preview.Binding || len(next.Advisories) != 0 {
		t.Fatal("old advisory reanimated under corrected native source", next)
	}
	fresh, e := s.MentorAdvisories(ctx, r.mentor.ID, r.eventID)
	resultsSourceRequire(t, e)
	for _, target := range fresh.Targets {
		if target.Own != nil || target.Version != 0 {
			t.Fatal("old correction reanimated", target)
		}
	}
}
