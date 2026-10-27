package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/results"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

// Reaches the native preparation operation which exceeded the unchanged HTTP
// budget in two connected event runs. Current source, explicit preparation,
// event serialization and exact command acknowledgement remain mandatory.
func TestResultPreparationWithDatabaseRoundTripCost(t *testing.T) {
	f := newResultsSourceFixture(t, rubric.MentorQualification)
	f.complete(t)
	r := f.runtime
	request := qualificationHTTPRequest(t)
	path := fmt.Sprintf("/api/v1/events/%s/results", r.eventID)
	status, raw := request(f.admin, r.owner, "GET", path, nil, 0, true)
	if status != 200 {
		t.Fatalf("native result source HTTP%d", status)
	}
	page := resultAPIDecode[contract.ResultPage](t, "ResultPage", raw)
	if page.Preview.Binding == "" || len(page.Preview.Blockers) != 0 {
		t.Fatal("native preparation lacks its actual complete independent judge source")
	}
	tracer, surface := resultCostSurface(t, r, "admin")
	command := contract.ResultCommand{
		RequestId: uuid.New(), Action: "prepare", Version: page.Round.Version,
		SourceBinding: page.Preview.Binding, Reason: "Explicit preparation under native request budget",
	}
	worker := observeScoringMaintenance(t, r, tracer)
	tracer.enabled.Store(true)
	began := time.Now()
	status, raw = request(surface, r.owner, "POST", path, command, page.Revision, true)
	elapsed := time.Since(began)
	tracer.enabled.Store(false)
	observed := <-worker
	t.Logf("native preparation HTTP%d elapsed=%s queries=%d worker_blocked_by_api=%t worker_elapsed=%s",
		status, elapsed, tracer.calls.Load(), observed.blocked, observed.elapsed)
	if observed.err != nil || !observed.blocked {
		t.Fatalf("native maintenance contention not verified: %v", observed.err)
	}
	if status != 200 {
		t.Fatalf("native result preparation exceeded unchanged request deadline: HTTP%d", status)
	}
	accepted := resultAPIDecode[contract.ResultPage](t, "ResultPage", raw)
	if !accepted.Current || accepted.Receipt == nil || accepted.Receipt.RequestId != command.RequestId ||
		accepted.Receipt.Action != "prepare" || accepted.Round.Source.Binding != page.Preview.Binding ||
		accepted.Round.Confirmation != nil || len(accepted.Announcements) != 0 {
		t.Fatal("bounded preparation lost exact intent/source or inferred confirmation/publication")
	}
}

func resultCostSurface(t *testing.T, r *liveRehearsal, name string) (*scoringQueryDelay, rehearsalSurface) {
	t.Helper()
	tracer := &scoringQueryDelay{delay: 6 * time.Millisecond, names: map[string]int{}, locked: make(chan struct{})}
	cfg := r.pool.Config()
	cfg.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	original := r.pool
	r.pool = pool
	surface := r.surface(t, name)
	r.pool = original
	return tracer, surface
}

func TestResultBallotWithDatabaseRoundTripCost(t *testing.T) {
	f, second := newTiedResultsFixture(t)
	r := f.runtime
	current, err := f.service.Read(t.Context(), r.owner.ID, r.eventID)
	resultsSourceRequire(t, err)
	_, err = f.service.Change(t.Context(), r.owner.ID, r.eventID, current.Revision, results.Command{
		RequestID: uuid.New(), Version: current.Round.Version, Action: "prepare",
		SourceBinding: current.Preview.Binding, Reason: "Explicit tied source for native ballot budget",
	})
	resultsSourceRequire(t, err)
	ballots, err := f.service.Ballots(t.Context(), f.judges[0].ID, r.eventID)
	resultsSourceRequire(t, err)
	if len(ballots.Ties) == 0 {
		t.Fatal("native ballot fixture lacks a real tie")
	}
	tracer, surface := resultCostSurface(t, r, "public")
	command := contract.ResultVoteCommand{
		RequestId: uuid.New(), Action: "vote", Version: ballots.Version,
		SourceBinding: ballots.SourceBinding, TieId: ballots.Ties[0].State.Tie.ID, TeamId: r.teamID,
		Reason: "Independent native ballot under the existing request budget",
		Evidence: []contract.ResultVoteEvidence{
			{TeamId: r.teamID, DeliverableId: &f.deliverable, Note: "Reviewed first frozen output"},
			{TeamId: second, DeliverableId: &f.deliverable, Note: "Reviewed competing frozen output"},
		},
	}
	request := qualificationHTTPRequest(t)
	worker := observeScoringMaintenance(t, r, tracer)
	tracer.enabled.Store(true)
	began := time.Now()
	status, raw := request(surface, f.judges[0], "POST", fmt.Sprintf("/api/v1/events/%s/results/ballots", r.eventID), command, ballots.Revision, true)
	elapsed := time.Since(began)
	tracer.enabled.Store(false)
	observed := <-worker
	t.Logf("native ballot HTTP%d elapsed=%s queries=%d worker_blocked_by_api=%t worker_elapsed=%s", status, elapsed, tracer.calls.Load(), observed.blocked, observed.elapsed)
	if observed.err != nil || !observed.blocked {
		t.Fatalf("native maintenance contention not verified: %v", observed.err)
	}
	if status != 200 {
		t.Fatalf("native ballot exceeded unchanged request deadline: HTTP%d", status)
	}
	accepted := resultAPIDecode[contract.ResultBallotPage](t, "ResultBallotPage", raw)
	if accepted.Receipt == nil || accepted.Receipt.RequestId != command.RequestId || accepted.SourceBinding != ballots.SourceBinding {
		t.Fatal("bounded ballot lost its current source or exact acknowledgement")
	}
	for _, tie := range accepted.Ties {
		if tie.State.Tie.Id == command.TieId {
			if tie.OwnVote == nil || tie.OwnVote.JudgeId != f.judges[0].ID || tie.OwnVote.TeamId != r.teamID || tie.State.VotesReceived != 1 || tie.State.WinnerId != nil {
				t.Fatal("bounded own ballot inferred another judge or a premature majority")
			}
			return
		}
	}
	t.Fatal("bounded ballot omitted the actual reviewed tie")
}
