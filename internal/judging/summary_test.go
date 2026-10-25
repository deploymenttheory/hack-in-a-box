package judging

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestFactualMentorBriefRequiresExplicitReviewAndCurrentSourceAuthority(t *testing.T) {
	f := setup(t, true)
	freeze := f.freeze()
	decision := f.decide(freeze, "reconcile", strings.Repeat("a", 40))
	mentor := testdb.Person(t, f.pool)
	f.exec(
		"INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'mentor')",
		f.event,
		mentor.ID,
	)
	f.exec(
		"INSERT INTO policy_resources(event_id,id,kind)VALUES($1,$2,'team') ON CONFLICT DO NOTHING",
		f.event,
		f.team,
	)
	f.exec(
		"INSERT INTO resource_assignments(event_id,resource_id,person_id)VALUES($1,$2,$3)",
		f.event,
		f.team,
		mentor.ID,
	)
	f.exec(
		"INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at)VALUES($1,$2,$3,'primary','published',$4,$5)",
		f.event,
		f.team,
		mentor.ID,
		f.dates.LiveStart,
		f.dates.LiveEnd,
	)
	p, err := (submissions.Service{Pool: f.pool, Clock: f.s.now}).Read(
		t.Context(),
		f.owner.ID,
		f.event,
		f.team,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Final.Document.Evidence) == 0 {
		t.Fatal("fixture lacks final evidence")
	}
	request := uuid.New()
	c := SummaryCommand{
		RequestID:            request,
		Action:               "save",
		SubmissionRevision:   p.Final.Revision,
		RepositoryDecisionID: &decision.Decision.ID,
		Facts: []SummaryFact{
			{
				DeliverableID: p.Final.Document.Evidence[0].DeliverableID,
				Fact:          "Mentor observed this submitted demonstration complete its stated workflow.",
			},
		},
		Reason: "Separate judge-facing factual handoff; private assessment excluded",
	}
	ack, err := f.s.ChangeSummary(t.Context(), mentor.ID, f.event, f.team, 0, c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ChangeSummary(
		t.Context(),
		mentor.ID,
		f.event,
		f.team,
		1,
		SummaryCommand{
			RequestID:            uuid.New(),
			Action:               "approve",
			SubmissionRevision:   p.Final.Revision,
			RepositoryDecisionID: &decision.Decision.ID,
			Reason:               "Self approval",
		},
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("mentor self-approved", err)
	}
	before, err := f.s.Brief(t.Context(), f.first.ID, f.event, f.run, f.team)
	if err != nil || before.MentorSummary != nil {
		t.Fatal("unapproved mentor content exposed", err)
	}
	approve := SummaryCommand{
		RequestID:            uuid.New(),
		Action:               "approve",
		SubmissionRevision:   p.Final.Revision,
		RepositoryDecisionID: &decision.Decision.ID,
		Reason:               "Reviewed only final evidence-linked facts; no private notes, qualification values or ranks",
	}
	if _, err = f.s.ChangeSummary(
		t.Context(),
		f.owner.ID,
		f.event,
		f.team,
		0,
		approve,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("stale approval accepted", err)
	}
	if _, err = f.s.ChangeSummary(
		t.Context(),
		f.owner.ID,
		f.event,
		f.team,
		1,
		approve,
	); err != nil {
		t.Fatal(err)
	}
	brief, err := f.s.Brief(t.Context(), f.first.ID, f.event, f.run, f.team)
	if err != nil || brief.MentorSummary == nil || brief.MentorSummary.AuthorID != mentor.ID ||
		brief.MentorSummary.ApprovedBy == nil ||
		len(brief.MentorSummary.Facts) != 1 {
		t.Fatal("approved factual handoff absent", brief.MentorSummary, err)
	}
	recovered, err := f.s.Summary(t.Context(), mentor.ID, f.event, f.team, request)
	if err != nil || recovered.Version != 2 || recovered.Receipt == nil ||
		recovered.Receipt.Revision != ack.Revision {
		t.Fatal("current page replaced original command outcome", recovered, err)
	}
	replay, err := f.s.ChangeSummary(t.Context(), mentor.ID, f.event, f.team, 0, c)
	if err != nil || replay != ack {
		t.Fatal("original exact summary command not recovered", replay, err)
	}
	f.decide(freeze, "amend", strings.Repeat("b", 40))
	changed, err := f.s.Brief(t.Context(), f.first.ID, f.event, f.run, f.team)
	if err != nil || changed.MentorSummary != nil {
		t.Fatal("summary silently adopted changed code", changed.MentorSummary, err)
	}
	page, err := f.s.Summary(t.Context(), mentor.ID, f.event, f.team, request)
	if err != nil || page.Summary != nil || page.Version != 2 || page.Receipt == nil {
		t.Fatal("stale summary leaked or lost safe original receipt", page, err)
	}
	f.exec(
		"INSERT INTO mentor_conflicts(event_id,mentor_id,competitor_id,kind)VALUES($1,$2,$3,'reporting')",
		f.event,
		mentor.ID,
		f.member.ID,
	)
	if _, err = f.s.Summary(
		t.Context(),
		mentor.ID,
		f.event,
		f.team,
		request,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("declared reporting conflict retained factual authority", err)
	}
	f.exec("DELETE FROM mentor_conflicts WHERE event_id=$1 AND mentor_id=$2", f.event, mentor.ID)
	f.exec("DELETE FROM mentor_assignments WHERE event_id=$1 AND person_id=$2", f.event, mentor.ID)
	if _, err = f.s.Summary(
		t.Context(),
		mentor.ID,
		f.event,
		f.team,
		request,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("removed mentor retained summary/history authority", err)
	}
	if _, err = f.pool.Exec(
		t.Context(),
		"DELETE FROM judge_factual_summaries WHERE event_id=$1",
		f.event,
	); err == nil {
		t.Fatal("authored factual versions mutable")
	}
}
