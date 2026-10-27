package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/qualification"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
)

type scoringQueryDelay struct {
	calls     atomic.Int64
	enabled   atomic.Bool
	delay     time.Duration
	mu        sync.Mutex
	names     map[string]int
	locked    chan struct{}
	lockOnce  sync.Once
	holderPID atomic.Uint32
}

type scoringLockTraceKey struct{}

func (d *scoringQueryDelay) TraceQueryStart(
	ctx context.Context,
	_ *pgx.Conn,
	query pgx.TraceQueryStartData,
) context.Context {
	if d.enabled.Load() {
		d.calls.Add(1)
		name := "unnamed"
		if strings.HasPrefix(query.SQL, "-- name: ") {
			name = strings.Fields(strings.TrimPrefix(query.SQL, "-- name: "))[0]
		}
		d.mu.Lock()
		d.names[name]++
		d.mu.Unlock()
		timer := time.NewTimer(d.delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
		}
		if name == "LockEvent" {
			return context.WithValue(ctx, scoringLockTraceKey{}, true)
		}
	}
	return ctx
}

func (d *scoringQueryDelay) TraceQueryEnd(
	ctx context.Context,
	conn *pgx.Conn,
	result pgx.TraceQueryEndData,
) {
	if ctx.Value(scoringLockTraceKey{}) == true && result.Err == nil {
		d.lockOnce.Do(func() { d.holderPID.Store(conn.PgConn().PID()); close(d.locked) })
	}
}

type scoringWorkerObservation struct {
	blocked bool
	elapsed time.Duration
	err     error
}

// Recreate the observed causal seam: native worker settlement waits behind the
// actual API event transaction. Observe PostgreSQL's real lock graph rather
// than inserting an artificial blocking transaction or forging a queue receipt.
func observeScoringMaintenance(
	t *testing.T,
	r *liveRehearsal,
	tracer *scoringQueryDelay,
) <-chan scoringWorkerObservation {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	t.Cleanup(cancel)
	done := make(chan scoringWorkerObservation, 1)
	go func() {
		defer cancel()
		select {
		case <-tracer.locked:
		case <-ctx.Done():
			done <- scoringWorkerObservation{err: ctx.Err()}
			return
		}
		began := time.Now()
		worker := make(chan error, 1)
		go func() { worker <- (submissions.Service{Pool: r.pool, Clock: r.clock.read}).Maintain(ctx) }()
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		observed := false
		for {
			select {
			case err := <-worker:
				done <- scoringWorkerObservation{blocked: observed, elapsed: time.Since(began), err: err}
				return
			case <-ctx.Done():
				done <- scoringWorkerObservation{blocked: observed, elapsed: time.Since(began), err: ctx.Err()}
				return
			case <-ticker.C:
				var waiting bool
				if err := r.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND $1::integer=ANY(pg_blocking_pids(pid)))", int32(tracer.holderPID.Load())).
					Scan(&waiting); err != nil {
					done <- scoringWorkerObservation{err: err}
					return
				}
				observed = observed || waiting
			}
		}
	}()
	return done
}

// The real native five-team HTTP card-save path, with a fixed 4ms database
// round-trip cost. The router keeps its ordinary production request timeout;
// this does not retry a failed intent or substitute a fake handler/source.
func TestQualificationFiveTeamSaveWithDatabaseRoundTripCost(t *testing.T) {
	verifyQualificationSaveWithQueryCost(t, 4*time.Millisecond)
}

func TestDiagnosticQualificationQueryCensus(t *testing.T) {
	if os.Getenv("HACKBOX_SCORING_QUERY_CENSUS") != "1" {
		t.Skip("explicit diagnostic census only; not round-trip regression evidence")
	}
	verifyQualificationSaveWithQueryCost(t, 0)
}

func verifyQualificationSaveWithQueryCost(t *testing.T, delay time.Duration) {
	t.Helper()
	f := newQualificationFinalCohort(t, 5)
	r := f.runtime
	page, err := f.admin.deps.Qualification.Read(t.Context(), r.owner.ID, r.eventID, f.run)
	if err != nil {
		t.Fatal(err)
	}
	page, err = f.admin.deps.Qualification.Change(
		t.Context(),
		r.owner.ID,
		r.eventID,
		f.run,
		page.Revision,
		qualification.Command{
			RequestID: uuid.New(),
			Version:   page.Round.Version,
			Action:    "configure_panel",
			Reason:    "Explicit intended native panel",
			Panel: &qualification.Panel{
				MentorIDs:   []uuid.UUID{r.mentor.ID, f.second.ID},
				OrganiserID: r.owner.ID,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	tracer := &scoringQueryDelay{delay: delay, names: map[string]int{}, locked: make(chan struct{})}
	poolConfig := r.pool.Config()
	poolConfig.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(t.Context(), poolConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	original := r.pool
	r.pool = pool
	surface := r.surface(t, "public")
	r.pool = original
	team := page.Teams[0]
	command := qualification.Command{
		RequestID:     uuid.New(),
		Version:       page.Round.Version,
		Action:        "save_card",
		Reason:        "Native moderated card under bounded database round-trip cost",
		FreezeBinding: team.FreezeBinding,
		Card:          &qualification.CardInput{TeamID: team.TeamID, Scores: f.scores},
	}
	encoded, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	var input contract.QualificationCommand
	if err = json.Unmarshal(encoded, &input); err != nil {
		t.Fatal(err)
	}
	request := qualificationHTTPRequest(t)
	var worker <-chan scoringWorkerObservation
	if delay > 0 {
		worker = observeScoringMaintenance(t, r, tracer)
	}
	tracer.enabled.Store(true)
	began := time.Now()
	status, raw := request(
		surface,
		r.owner,
		"POST",
		fmt.Sprintf("/api/v1/events/%s/mentor-qualification/%s", r.eventID, f.run),
		input,
		page.Revision,
		true,
	)
	tracer.enabled.Store(false)
	if worker != nil {
		observed := <-worker
		t.Logf(
			"native settlement worker blocked_by_api=%t elapsed=%s",
			observed.blocked,
			observed.elapsed,
		)
		if observed.err != nil || !observed.blocked {
			t.Errorf("native API/worker lock contention was not verified: %v", observed.err)
		}
	}
	t.Logf(
		"native save status=%d elapsed=%s database_queries=%d",
		status,
		time.Since(began),
		tracer.calls.Load(),
	)
	tracer.mu.Lock()
	type queryCount struct {
		name  string
		count int
	}
	var counts []queryCount
	for name, count := range tracer.names {
		counts = append(counts, queryCount{name, count})
	}
	tracer.mu.Unlock()
	sort.Slice(counts, func(i, j int) bool { return counts[i].count > counts[j].count })
	for i, item := range counts {
		if i == 15 {
			break
		}
		t.Logf("query=%s calls=%d", item.name, item.count)
	}
	if status != 200 {
		t.Fatalf("native card save exceeded its unchanged request budget or failed: HTTP%d", status)
	}
	var accepted qualification.Page
	if err = json.Unmarshal(raw, &accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.Receipt == nil || accepted.Receipt.RequestID != command.RequestID ||
		accepted.Receipt.ExpectedRevision != page.Revision ||
		accepted.Receipt.ExpectedVersion != command.Version {
		t.Fatal("native save lost exact original acknowledgement")
	}
}
