package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/deploymenttheory/hack-in-a-box/internal/results"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestNativeMentorCategoryAdvisoryNeverCountsAsJudgeVote(t *testing.T) {
	f, second := newTiedResultsFixture(t, func(d *rubric.Definition) {
		category := d.Categories[0].ID
		d.Prizes = []rubric.Prize{
			{ID: uuid.New(), Name: "Native category advisory award", CategoryID: &category},
		}
	})
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
	// Both independent HTTP portals retain current resource policy, strict
	// contract validation, CSRF and no-store; API success is the native owner.
	request := qualificationHTTPRequest(t)
	path := fmt.Sprintf("/api/v1/events/%s/results/mentor-advisories", r.eventID)
	for _, surface := range []rehearsalSurface{f.admin, f.public} {
		status, raw := request(surface, r.mentor, "GET", path, nil, 0, true)
		if status != 200 {
			t.Fatal("native advisory HTTP read", status, string(raw))
		}
		resultAPIDecode[contract.ResultAdvisoryPage](t, "ResultAdvisoryPage", raw)
		status, raw = request(surface, r.mentor, "POST", path, c, own.Revision, true)
		if status != 200 {
			t.Fatal("native original advisory replay HTTP", status, string(raw))
		}
		got := resultAPIDecode[contract.ResultAdvisoryPage](t, "ResultAdvisoryPage", raw)
		if got.Receipt == nil || got.Receipt.RequestId != c.RequestID {
			t.Fatal("HTTP lost original ACK")
		}
		status, _ = request(surface, r.mentor, "POST", path, c, own.Revision, false)
		if status != 400 {
			t.Fatal(
				"missing required CSRF header was not rejected by the canonical contract",
				status,
			)
		}
		match := fmt.Sprintf("\"%d\"", own.Revision)
		status, _ = resultAPIPost(t, surface, r.mentor, path, c, &match, strings.Repeat("x", 32))
		if status != 403 {
			t.Fatal("invalid advisory CSRF token accepted", status)
		}
		status, _ = request(surface, f.judges[0], "GET", path, nil, 0, true)
		if status != 403 {
			t.Fatal("judge obtained mentor editor", status)
		}
		status, raw = request(
			surface,
			f.judges[0],
			"GET",
			fmt.Sprintf("/api/v1/events/%s/results/ballots", r.eventID),
			nil,
			0,
			true,
		)
		if status != 200 {
			t.Fatal("judge native advisory ballot HTTP", status, string(raw))
		}
		resultAPIDecode[contract.ResultBallotPage](t, "ResultBallotPage", raw)
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
	assertAssignedMentorAdvisoryIsolation(t, f, outsider, c, len(own.Targets))
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
	// A new current conflict and loss of published coverage remove all advice
	// access immediately; neither a role nor old source permits contribution.
	conflict := dbgen.SetMentorConflictParams{
		EventID:      r.eventID,
		MentorID:     r.mentor.ID,
		CompetitorID: r.competitor.ID,
		Kind:         "reporting",
	}
	resultsSourceRequire(t, dbgen.New(r.pool).SetMentorConflict(ctx, conflict))
	conflictPage, err := s.MentorAdvisories(ctx, r.mentor.ID, r.eventID)
	resultsSourceRequire(t, err)
	if len(conflictPage.Targets) != 0 {
		t.Fatal("conflicted mentor retained target", conflictPage)
	}
	if _, err = s.MentorAdvisoryReceipt(
		ctx,
		r.mentor.ID,
		r.eventID,
		c.RequestID,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("conflicted original receipt read", err)
	}
	resultsSourceRequire(
		t,
		dbgen.New(r.pool).DeleteMentorConflict(ctx, dbgen.DeleteMentorConflictParams(conflict)),
	)
	assignments, err := dbgen.New(r.pool).MentorAssignments(ctx, r.eventID)
	resultsSourceRequire(t, err)
	_, err = r.pool.Exec(
		ctx,
		"DELETE FROM mentor_assignments WHERE event_id=$1 AND person_id=$2 AND snapshot='published'",
		r.eventID,
		r.mentor.ID,
	)
	resultsSourceRequire(t, err)
	uncovered, err := s.MentorAdvisories(ctx, r.mentor.ID, r.eventID)
	resultsSourceRequire(t, err)
	if len(uncovered.Targets) != 0 {
		t.Fatal("removed coverage retained target")
	}
	without, err := s.Read(ctx, r.owner.ID, r.eventID)
	resultsSourceRequire(t, err)
	if len(without.Advisories) != 0 {
		t.Fatal("removed coverage advisory remained current")
	}
	for _, a := range assignments {
		if a.PersonID == r.mentor.ID && a.Snapshot == "published" {
			_, err = r.pool.Exec(
				ctx,
				"INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at)VALUES($1,$2,$3,$4,$5,$6,$7)",
				a.EventID,
				a.TeamID,
				a.PersonID,
				a.Slot,
				a.Snapshot,
				a.StartsAt,
				a.EndsAt,
			)
			resultsSourceRequire(t, err)
		}
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
	// Complete every required judge ballot while the other category ties have
	// no mentor advisory. Optional input absence cannot gate actual confirmation.
	for _, tie := range initial.Preview.Ties {
		if tie.ID == target.TieID {
			continue
		}
		for i := range f.judges {
			ballots, err := s.Ballots(ctx, f.judges[i].ID, r.eventID)
			resultsSourceRequire(t, err)
			_, err = s.Vote(
				ctx,
				f.judges[i].ID,
				r.eventID,
				ballots.Revision,
				results.Command{
					RequestID:     uuid.New(),
					Version:       ballots.Version,
					Action:        "vote",
					SourceBinding: ballots.SourceBinding,
					TieID:         tie.ID,
					TeamID:        r.teamID,
					Evidence: []results.VoteEvidence{
						{TeamID: r.teamID, DeliverableID: &f.deliverable},
					},
					Reason: "All required judges independently resolve each category",
				},
			)
			resultsSourceRequire(t, err)
		}
	}
	confirmPage, err := s.Read(ctx, r.owner.ID, r.eventID)
	resultsSourceRequire(t, err)
	prize := confirmPage.Preview.Criteria.Prizes[0]
	confirmed, err := s.Change(
		ctx,
		r.owner.ID,
		r.eventID,
		confirmPage.Revision,
		results.Command{
			RequestID:     uuid.New(),
			Version:       confirmPage.Round.Version,
			Action:        "confirm",
			SourceBinding: confirmPage.Preview.Binding,
			Awards: []results.Award{
				{
					PrizeID:    prize.ID,
					Name:       prize.Name,
					CategoryID: prize.CategoryID,
					TeamID:     r.teamID,
				},
			},
			Reason: "Native confirmation without mandatory mentor input",
		},
	)
	resultsSourceRequire(t, err)
	currentOwn, err := s.MentorAdvisories(ctx, r.mentor.ID, r.eventID)
	resultsSourceRequire(t, err)
	later, err := s.Advise(
		ctx,
		r.mentor.ID,
		r.eventID,
		currentOwn.Revision,
		command(
			currentOwn,
			"Later attributed advisory correction, never a rewrite of the earlier decision.",
		),
	)
	resultsSourceRequire(t, err)
	unchanged, err := s.Read(ctx, r.owner.ID, r.eventID)
	resultsSourceRequire(t, err)
	if !reflect.DeepEqual(confirmed.Round, unchanged.Round) ||
		unchanged.Round.Confirmation == nil ||
		unchanged.Round.Confirmation.DecisionBinding != confirmed.Round.Confirmation.DecisionBinding ||
		later.Receipt.Version < 4 {
		t.Fatal("later optional views rewrote confirmed history", unchanged)
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

func assertAssignedMentorAdvisoryIsolation(
	t *testing.T,
	f *resultsSourceFixture,
	other dbgen.Person,
	original results.AdvisoryCommand,
	targetCount int,
) {
	t.Helper()
	r, ctx := f.runtime, t.Context()
	published, err := dbgen.New(r.pool).MentorAssignments(ctx, r.eventID)
	resultsSourceRequire(t, err)
	for _, assignment := range published {
		if assignment.PersonID == r.mentor.ID && assignment.Snapshot == "published" {
			_, err = r.pool.Exec(
				ctx,
				"INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at)VALUES($1,$2,$3,'second','published',$4,$5)",
				r.eventID,
				r.teamID,
				other.ID,
				assignment.StartsAt,
				assignment.EndsAt,
			)
			resultsSourceRequire(t, err)
		}
	}
	assigned, err := f.service.MentorAdvisories(ctx, other.ID, r.eventID)
	resultsSourceRequire(t, err)
	if len(assigned.Targets) != targetCount {
		t.Fatal("current second mentor did not receive own category targets")
	}
	for _, target := range assigned.Targets {
		if target.Own != nil || target.Version != 0 {
			t.Fatal("another mentor's prose became own contribution", target)
		}
	}
	encoded, err := json.Marshal(assigned)
	resultsSourceRequire(t, err)
	if strings.Contains(string(encoded), "ADVISORY-ORIGINAL") {
		t.Fatal("assigned other mentor read private prose")
	}
	if _, err = f.service.MentorAdvisoryReceipt(
		ctx,
		other.ID,
		r.eventID,
		original.RequestID,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("assigned second mentor read another author's original ACK", err)
	}
	if _, err = f.service.Advise(
		ctx,
		other.ID,
		r.eventID,
		assigned.Revision,
		original,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("assigned second mentor edited another author's contribution", err)
	}
}
