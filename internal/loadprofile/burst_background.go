package loadprofile

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type BurstReadiness struct {
	Started time.Time `json:"started"`
	Ready   time.Time `json:"all_native_people_read_ready"`
	People  int       `json:"distinct_successful_native_people"`
}

// runBurstWithBackground waits for a genuine accepted native read from every
// cohort person before releasing the registration barrier. The same ordinary
// read streams keep running through the burst. Login/account counts alone do
// not prove that all 1000 people supplied background traffic.
func runBurstWithBackground(ctx context.Context, config RuntimeConfig, burst BurstConfig,
	actors []Actor, tasks []RegistrationTask, reads Action, writes NativeWrites,
	metrics *Metrics, accepted *WriteLedger,
) (BurstResult, BurstReadiness, error) {
	var result BurstResult
	readiness := BurstReadiness{Started: time.Now()}
	ready := make(chan struct{})
	seen := make(map[uuid.UUID]bool, len(actors))
	var mu sync.Mutex
	read := func(_ context.Context, actor Actor, sequence uint64) (Measurement, *AcceptedWrite, error) {
		// Stop scheduling with the background context, but drain a read already
		// sent under the outer run context and unchanged native client deadline.
		measurement, write, err := reads(ctx, actor, sequence)
		if err == nil && measurement.Outcome == Accepted {
			mu.Lock()
			seen[actor.Session.Person()] = true
			if len(seen) == len(actors) && readiness.Ready.IsZero() {
				readiness.Ready = time.Now()
				close(ready)
			}
			mu.Unlock()
		}
		return measurement, write, err
	}
	backgroundContext, stop := context.WithCancel(ctx)
	finished := make(chan error, 1)
	go func() {
		_, err := RunStage(backgroundContext, StageConfig{
			Profile: burst.Profile,
			Phase:   BurstPhase, Duration: time.Hour, Interval: config.Interval,
		}, actors, read, metrics, accepted)
		finished <- err
	}()
	// Every actor's first offset is below Interval and its native request has
	// the existing I/O deadline. This setup bound does not alter any SLO.
	deadline := time.NewTimer(config.Interval + NativeRequestTimeout)
	defer deadline.Stop()
	var readinessErr, backgroundErr error
	ended := false
	select {
	case <-ready:
	case backgroundErr = <-finished:
		ended = true
		readinessErr = fmt.Errorf(
			"%w: native background stage ended before cohort readiness",
			errLoadStage,
		)
	case <-ctx.Done():
		readinessErr = fmt.Errorf("native background readiness interrupted: %w", ctx.Err())
	case <-deadline.C:
		readinessErr = fmt.Errorf(
			"%w: full native cohort background readiness incomplete",
			errLoadStage,
		)
	}
	var burstErr error
	if readinessErr == nil {
		result, burstErr = RunRegistrationBurst(ctx, burst, tasks, writes, accepted)
	}
	stop()
	if !ended {
		backgroundErr = <-finished
	}
	mu.Lock()
	readiness.People = len(seen)
	mu.Unlock()
	if errors.Is(backgroundErr, context.Canceled) {
		backgroundErr = nil // Intentional background stop; actual HTTP errors stay measured.
	}
	return result, readiness, errors.Join(readinessErr, burstErr, backgroundErr)
}
