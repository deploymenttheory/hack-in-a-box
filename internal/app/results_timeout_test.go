package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
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
	tracer := &scoringQueryDelay{
		delay: 6 * time.Millisecond, names: map[string]int{}, locked: make(chan struct{}),
	}
	cfg := r.pool.Config()
	cfg.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	original := r.pool
	r.pool = pool
	surface := r.surface(t, "admin")
	r.pool = original
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
