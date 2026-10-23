package reviewdashboard

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/feedback"
	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/deploymenttheory/hack-in-a-box/internal/moderation"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
	"github.com/google/uuid"
)

func dashboard(f *fixture) Service {
	clock := func() time.Time { return f.now }
	shared := &feedback.Service{Pool: f.pool, Clock: clock}
	huddles := &moderation.Service{Pool: f.pool, Clock: clock, Cards: &f.s, Actions: moderation.FeedbackActions{Service: shared}}
	f.s.Moderation = huddles
	return Service{Pool: f.pool, Clock: clock, Help: help.Service{Pool: f.pool, Clock: clock}, Feedback: shared, Cards: &f.s, Moderation: huddles}
}
func TestDashboardKeepsOperationalRiskIndependentFromPrivateAssessment(t *testing.T) {
	f := setup(t)
	s := dashboard(f)
	if _, err := f.s.Change(t.Context(), f.primary.ID, f.event, f.team, f.revision(), f.command(f.checkpoint, "submit", true)); err != nil {
		t.Fatal(err)
	}
	read := func(actor uuid.UUID) (Page, error) { return s.Read(t.Context(), actor, f.event, uuid.Nil, &f.team, 25) }
	page, err := read(f.owner.ID)
	if err != nil || len(page.Teams) != 1 {
		t.Fatal(page, err)
	}
	team := page.Teams[0]
	if team.Support.Risk != "unknown" || team.Assessment.Live.Submitted != 1 || team.Assessment.Live.Missing != 1 || team.Assessment.Live.Complete {
		t.Fatal("unknown operational state inferred from assessment or missing treated as zero", team)
	}
	raw, _ := json.Marshal(page)
	for _, private := range []string{"PRIVATE-SCORE-EVIDENCE-CANARY", "PRIVATE-JUDGE-CANARY", `"scores":`, `"criteria":`, `"evidence":`, `"total":`} {
		if strings.Contains(string(raw), private) {
			t.Fatal("coordination disclosed private assessment", private)
		}
	}
	page, err = read(f.second.ID)
	if err != nil || len(page.Teams) != 1 || page.Teams[0].Assessment.Live.Submitted != 0 || page.Teams[0].Assessment.Live.Missing != 1 || len(page.Moderators) != 0 {
		t.Fatal("pre-huddle mentor saw other's independent completion", page, err)
	}
	if _, err = s.Preview(t.Context(), f.primary.ID, f.event, f.team); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("mentor gained judge-preview authority", err)
	}
	judge := testdb.Person(t, f.pool)
	f.exec(`INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'judge')`, f.event, judge.ID)
	for _, actor := range []uuid.UUID{judge.ID, f.member.ID} {
		if _, err = read(actor); !errors.Is(err, access.ErrForbidden) {
			t.Fatal("unassigned judge or competitor gained coordination", err)
		}
	}
	f.exec(`UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2`, f.event, f.second.ID)
	if _, err = read(f.second.ID); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("revoked mentor retained dashboard", err)
	}
	f.now = f.config.SubmissionCutoff
	page, err = read(f.owner.ID)
	if err != nil || page.Teams[0].Assessment.OutputState != "not_submitted" || page.Teams[0].Assessment.Availability != "not_applicable" || page.Teams[0].Assessment.Live.RequiredReviews != 0 {
		t.Fatal("not-submitted output displayed awaiting scores", page, err)
	}
	agenda, err := s.Huddles(t.Context(), f.owner.ID, f.event, f.team, uuid.Nil, 25)
	if err != nil || agenda.Availability != "not_applicable" || len(agenda.Agenda.Items) != 0 {
		t.Fatal("not-submitted output has active agenda", agenda, err)
	}
	preview, err := s.Preview(t.Context(), f.owner.ID, f.event, f.team)
	if err != nil || preview.Output.Outcome != "not_submitted" || preview.Output.FrozenRevision != nil {
		t.Fatal("factual preview invented assessed frozen output", preview, err)
	}
}

func TestDashboardRequestedTeamFilterPrecedesBoundAndIncludesDisbandedWithoutScores(t *testing.T) {
	f := setup(t)
	s := dashboard(f)
	f.exec(`INSERT INTO hack_teams(event_id,id,name,state) SELECT $1,md5('support-dashboard-'||n)::uuid,'Earlier archived team','disbanded' FROM generate_series(1,1006)n`, f.event)
	requested := uuid.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff")
	f.exec(`INSERT INTO hack_teams(event_id,id,name,state)VALUES($1,$2,'Requested archived team','disbanded')`, f.event, requested)
	page, err := s.Read(t.Context(), f.owner.ID, f.event, uuid.Nil, &requested, 1)
	if err != nil || len(page.Teams) != 1 || page.Teams[0].ID != requested || page.NextCursor != nil || page.Teams[0].Assessment.OutputState != "disbanded" || page.Teams[0].Assessment.Availability != "not_applicable" {
		t.Fatal("team filter applied after bound or inactive output awaiting scores", page, err)
	}
	preview, err := s.Preview(t.Context(), f.owner.ID, f.event, requested)
	if err != nil || preview.Output.Outcome != "not_entered" || len(preview.Output.Artifacts) != 0 {
		t.Fatal("absent entry invented output or returned infrastructure error", preview, err)
	}
	if _, err = s.Read(t.Context(), f.primary.ID, f.event, uuid.Nil, &requested, 1); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("mentor gained unassigned historical team", err)
	}
}
