package judging

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
)

func (f *fixture) mentor(person uuid.UUID) {
	f.t.Helper()
	f.exec(
		"INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'mentor')",
		f.event,
		person,
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
		person,
	)
	f.exec(
		"INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at)VALUES($1,$2,$3,'primary','published',$4,$5)",
		f.event,
		f.team,
		person,
		f.dates.LiveStart,
		f.dates.LiveEnd,
	)
}

func (f *fixture) assertNativeVisible(person uuid.UUID, count int) {
	f.t.Helper()
	page, err := (submissions.Service{Pool: f.pool, Clock: f.s.now}).Read(
		f.t.Context(),
		person,
		f.event,
		f.team,
	)
	if err != nil || page.Final == nil || len(page.Final.Evidence) != count {
		f.t.Fatal("mixed-audience native fixture", page.Final, err)
	}
}

func noHiddenJudgeEvidence(t *testing.T, value any, hidden uuid.UUID) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), hidden.String()) ||
		strings.Contains(string(raw), "PRIVATE-HIDDEN-ARTIFACT-NOTE") {
		t.Error("current projection disclosed hidden deliverable identity or its note", string(raw))
	}
}

func TestApprovedMentorSummaryUsesRecipientDeliverableAudience(t *testing.T) {
	f := setup(t, false, false, true)
	f.mentor(f.first.ID)
	f.assertNativeVisible(f.first.ID, 2)
	f.assertNativeVisible(f.second.ID, 1)
	cmd := SummaryCommand{
		RequestID:          uuid.New(),
		Action:             "save",
		SubmissionRevision: 1,
		Reason:             "Dedicated judge-facing facts for explicit review",
		Facts: []SummaryFact{
			{DeliverableID: f.deliverable, Fact: "Visible demonstration completed its workflow"},
			{DeliverableID: f.hiddenDeliverable, Fact: "PRIVATE-HIDDEN-ARTIFACT-NOTE"},
		},
	}
	if _, err := f.s.ChangeSummary(t.Context(), f.first.ID, f.event, f.team, 0, cmd); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ChangeSummary(
		t.Context(),
		f.owner.ID,
		f.event,
		f.team,
		1,
		SummaryCommand{
			RequestID:          uuid.New(),
			Action:             "approve",
			SubmissionRevision: 1,
			Reason:             "Reviewed exact factual version",
		},
	); err != nil {
		t.Fatal(err)
	}
	var before string
	if err := f.pool.QueryRow(t.Context(), "SELECT document::text FROM judge_factual_summaries WHERE event_id=$1 AND version=2", f.event).
		Scan(&before); err != nil {
		t.Fatal(err)
	}
	brief, err := f.s.Brief(t.Context(), f.second.ID, f.event, f.run, f.team)
	if err != nil {
		t.Fatal(err)
	}
	noHiddenJudgeEvidence(t, brief, f.hiddenDeliverable)
	if brief.MentorSummary == nil || len(brief.MentorSummary.Facts) != 1 ||
		brief.MentorSummary.Facts[0].DeliverableID != f.deliverable {
		t.Error("visible summary fact missing", brief.MentorSummary)
	}
	var after string
	if err = f.pool.QueryRow(t.Context(), "SELECT document::text FROM judge_factual_summaries WHERE event_id=$1 AND version=2", f.event).
		Scan(&after); err != nil ||
		before != after {
		t.Fatal("projection changed immutable summary", err)
	}
}

func TestJudgeCardProjectionsRecheckEachDeliverableAfterPartialRoleLoss(t *testing.T) {
	f := setup(t, false, false, true)
	f.mentor(f.first.ID)
	f.assertNativeVisible(f.first.ID, 2)
	f.assertNativeVisible(f.second.ID, 1)
	scores := f.scores(7)
	for i := range scores {
		scores[i].Evidence = append(
			scores[i].Evidence,
			Evidence{DeliverableID: &f.hiddenDeliverable, Note: "PRIVATE-HIDDEN-ARTIFACT-NOTE"},
		)
	}
	if _, err := f.card(f.first.ID, 0, "submit", scores); err != nil {
		t.Fatal(err)
	}
	if _, err := f.card(f.second.ID, 0, "submit", f.scores(6)); err != nil {
		t.Fatal(err)
	}
	r, err := f.s.Round(t.Context(), f.owner.ID, f.event, f.run)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ChangeRound(
		t.Context(),
		f.owner.ID,
		f.event,
		f.run,
		r.Revision,
		RoundCommand{
			RequestID: uuid.New(),
			Action:    "open_deliberation",
			Reason:    "All required judges submitted",
		},
	); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err = f.pool.QueryRow(t.Context(), "SELECT document::text FROM judge_cards WHERE event_id=$1 AND author_id=$2", f.event, f.first.ID).
		Scan(&stored); err != nil {
		t.Fatal(err)
	}
	peer, err := f.s.Brief(t.Context(), f.second.ID, f.event, f.run, f.team)
	if err != nil {
		t.Fatal(err)
	}
	noHiddenJudgeEvidence(t, peer.SubmittedCards, f.hiddenDeliverable)
	f.exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mentor'",
		f.event,
		f.first.ID,
	)
	f.assertNativeVisible(f.first.ID, 1)
	own, err := f.s.Brief(t.Context(), f.first.ID, f.event, f.run, f.team)
	if err != nil {
		t.Fatal(err)
	}
	noHiddenJudgeEvidence(t, own, f.hiddenDeliverable)
	if own.OwnCard == nil || len(own.OwnCard.Scores) == 0 ||
		len(own.OwnCard.Scores[0].Evidence) == 0 {
		t.Error("visible authored evidence disappeared", own.OwnCard)
	}
	if err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		history, e := f.s.OwnHistory(t.Context(), tx, f.first.ID, f.event, f.run, f.team)
		if e != nil {
			return e
		}
		noHiddenJudgeEvidence(t, history, f.hiddenDeliverable)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var after string
	if err = f.pool.QueryRow(t.Context(), "SELECT document::text FROM judge_cards WHERE event_id=$1 AND author_id=$2", f.event, f.first.ID).
		Scan(&after); err != nil ||
		stored != after ||
		!strings.Contains(after, "PRIVATE-HIDDEN-ARTIFACT-NOTE") {
		t.Fatal("stored authored card rewritten", err)
	}
}
