package app

import (
	"errors"
	"reflect"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/deploymenttheory/hack-in-a-box/internal/results"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
	"github.com/google/uuid"
)

func newTiedResultsFixture(t *testing.T) (*resultsSourceFixture, uuid.UUID) {
	t.Helper()
	second := uuid.New()
	f := newResultsSourceFixtureWithSetup(t, rubric.AllTeams, nil, func(f *resultsSourceFixture, surface rehearsalSurface) {
		r := f.runtime
		competitor := testdb.Person(t, r.pool)
		for _, statement := range []struct {
			query string
			args  []any
		}{
			{"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')", []any{r.eventID, competitor.ID}},
			{"INSERT INTO hack_teams(event_id,id,name,state,frozen,published_captain_id) VALUES($1,$2,'Second real finalist','published',true,$3)", []any{r.eventID, second, competitor.ID}},
			{"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')", []any{r.eventID, second, competitor.ID}},
		} {
			_, err := r.pool.Exec(t.Context(), statement.query, statement.args...)
			resultsSourceRequire(t, err)
		}
		resultsSourceRequire(t, dbgen.New(r.pool).CreateCompetitionEntry(t.Context(), dbgen.CreateCompetitionEntryParams{EventID: r.eventID, TeamID: second}))
		p, err := surface.deps.Submissions.Read(t.Context(), competitor.ID, r.eventID, second)
		resultsSourceRequire(t, err)
		_, err = surface.deps.Submissions.Submit(t.Context(), competitor.ID, r.eventID, second, p.Revision, submissions.Command{Reason: "Actual second frozen build", Evidence: []submissions.Evidence{{DeliverableID: f.deliverable, Kind: "url", Label: "Second build", URL: "https://synthetic.invalid/second-result"}}})
		resultsSourceRequire(t, err)
	})
	f.round(t, judging.RoundCommand{Action: "assign", TeamID: second, JudgeIDs: []uuid.UUID{f.judges[0].ID, f.judges[1].ID}})
	for i, value := range []int{6, 8} {
		brief, err := f.admin.deps.Judging.Brief(t.Context(), f.judges[i].ID, f.runtime.eventID, f.judgeRun, second)
		resultsSourceRequire(t, err)
		scores := []judging.Score{}
		for _, category := range brief.Criteria.Categories {
			v := value
			scores = append(scores, judging.Score{CategoryID: category.ID, Value: &v, Evidence: []judging.Evidence{{DeliverableID: &f.deliverable}}})
		}
		_, err = f.admin.deps.Judging.ChangeCard(t.Context(), f.judges[i].ID, f.runtime.eventID, f.judgeRun, second, 0, judging.CardCommand{RequestID: uuid.New(), Action: "submit", SubmissionRevision: brief.SubmissionRevision, RepositoryDecisionID: brief.Repository.DecisionID, Scores: scores, Reason: "Independent actual tied assessment"})
		resultsSourceRequire(t, err)
	}
	f.complete(t)
	return f, second
}

func TestFinalJudgeBallotsRequireEveryoneThenStrictMajorityAndExplicitRevote(t *testing.T) {
	f, second := newTiedResultsFixture(t)
	ctx := t.Context()
	eventID := f.runtime.eventID
	actor := f.runtime.owner.ID
	p, err := f.service.Read(ctx, actor, eventID)
	resultsSourceRequire(t, err)
	p, err = f.service.Change(ctx, actor, eventID, p.Revision, results.Command{RequestID: uuid.New(), Version: p.Round.Version, Action: "prepare", SourceBinding: p.Preview.Binding, Reason: "Prepare actual exact tie"})
	resultsSourceRequire(t, err)
	var tie results.Tie
	for _, candidate := range p.Preview.Ties {
		if candidate.Kind == "final" {
			tie = candidate
		}
	}
	if tie.ID == "" {
		t.Fatal("native exact tie absent")
	}
	vote := func(index int, team uuid.UUID, reason string) results.BallotPage {
		t.Helper()
		current, e := f.service.Ballots(ctx, f.judges[index].ID, eventID)
		resultsSourceRequire(t, e)
		c := results.Command{RequestID: uuid.New(), Version: current.Version, Action: "vote", SourceBinding: current.SourceBinding, TieID: tie.ID, TeamID: team, Reason: reason, Evidence: []results.VoteEvidence{{TeamID: team, DeliverableID: &f.deliverable, Note: "PRIVATE-OWN-BALLOT-" + reason}}}
		accepted, e := f.service.Vote(ctx, f.judges[index].ID, eventID, current.Revision, c)
		resultsSourceRequire(t, e)
		replay, e := f.service.Vote(ctx, f.judges[index].ID, eventID, current.Revision, c)
		resultsSourceRequire(t, e)
		if !reflect.DeepEqual(replay.Receipt, accepted.Receipt) {
			t.Fatal("replay lost original ballot receipt")
		}
		recovery, e := f.service.OriginalReceipt(ctx, f.judges[index].ID, eventID, c.RequestID)
		resultsSourceRequire(t, e)
		if !reflect.DeepEqual(recovery.Receipt, accepted.Receipt) {
			t.Fatal("own safe receipt unavailable")
		}
		c.Reason = "Altered intent"
		if _, e = f.service.Vote(ctx, f.judges[index].ID, eventID, current.Revision, c); !errors.Is(e, access.ErrConflict) {
			t.Fatal("key reused with changed ballot", e)
		}
		return accepted
	}
	find := func(page results.BallotPage) results.JudgeTie {
		t.Helper()
		for _, item := range page.Ties {
			if item.State.Tie.ID == tie.ID {
				return item
			}
		}
		t.Fatal("assigned final tie absent")
		return results.JudgeTie{}
	}
	first := find(vote(0, f.runtime.teamID, "first"))
	if first.State.WinnerID != nil || first.State.Blocker != "all_required_judge_votes_required" {
		t.Fatal("partial panel resolved tie", first)
	}
	split := find(vote(1, second, "split"))
	if split.State.WinnerID != nil || split.State.Blocker != "recorded_judge_majority_required" || split.OwnVote.JudgeID != f.judges[1].ID {
		t.Fatal("split resolved or another judge's prose returned", split)
	}
	majority := find(vote(1, f.runtime.teamID, "explicit-revote"))
	if majority.State.WinnerID == nil || *majority.State.WinnerID != f.runtime.teamID || majority.State.Blocker != "" {
		t.Fatal("explicit majority absent", majority)
	}
	var historical int
	resultsSourceRequire(t, f.runtime.pool.QueryRow(ctx, "SELECT count(*) FROM result_versions WHERE event_id=$1 AND document::text LIKE '%PRIVATE-OWN-BALLOT-split%'", eventID).Scan(&historical))
	if historical == 0 {
		t.Fatal("previous explicit vote history erased")
	}
	outsider := testdb.Person(t, f.runtime.pool)
	if _, err = f.service.Ballots(ctx, outsider.ID, eventID); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("outsider read ballots", err)
	}
	f.reopen(t, 0)
	stale, err := f.service.Read(ctx, actor, eventID)
	resultsSourceRequire(t, err)
	if stale.Round.Reason != "" || len(stale.Round.Votes) != 0 || stale.Round.Confirmation != nil {
		t.Fatal("unavailable source retained private decision prose", stale)
	}
	if page, err := f.service.Ballots(ctx, f.judges[1].ID, eventID); err == nil || len(page.Ties) != 0 {
		t.Fatal("stale private ballot source survived reopen", page, err)
	}
}
