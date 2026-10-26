package loadprofile

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
)

type RegistrationTask struct {
	Actor Actor
	Input registration.Application
}

type BurstConfig struct {
	Profile     string
	Event, Form uuid.UUID
	FormVersion int32
}

type BurstResult struct {
	Started           time.Time     `json:"started"`
	Finished          time.Time     `json:"finished"`
	Elapsed           time.Duration `json:"monotonic_elapsed_ns"`
	ActionStartSpread time.Duration `json:"driver_action_start_spread_ns"`
	Attempted         int           `json:"attempted_actions"`
	Accepted          int           `json:"original_acknowledgements"`
	FailedActions     int           `json:"failed_actions"`
	MaximumInflight   int64         `json:"maximum_inflight_actions"`
	Interrupted       bool          `json:"interrupted"`
	Completed         bool          `json:"completed"`
}

func (c BurstConfig) validate(tasks []RegistrationTask) error {
	if (c.Profile != "release" && c.Profile != "smoke") || c.Event == uuid.Nil ||
		c.Form == uuid.Nil || c.FormVersion < 1 || len(tasks) < 1 || len(tasks) > 200 ||
		(c.Profile == "release" && len(tasks) != 200) {
		return fmt.Errorf(
			"%w: declared profile and exact published registration source required",
			errLoadStage,
		)
	}
	actors := make([]Actor, len(tasks))
	applications := make(map[uuid.UUID]bool, len(tasks))
	for index, task := range tasks {
		if task.Input.ID == uuid.Nil || applications[task.Input.ID] ||
			task.Input.FormID != c.Form || task.Input.FormVersion != c.FormVersion {
			return fmt.Errorf(
				"%w: distinct registration identities on the frozen form required",
				errLoadStage,
			)
		}
		applications[task.Input.ID] = true
		actors[index] = task.Actor
	}
	_, err := cohortIdentities(actors)
	return err
}

// RunRegistrationBurst releases all declared actors from one barrier. It has
// no pacing, retry, catch-up or admission bypass. The release fixture selects
// 200 existing cross-event actors; separate read-only background work keeps the
// complete 1000-person cohort active. Every physical command uses normal native
// registration and retains its original response/uncertainty and immutable
// candidate. Accepted waitlist entries remain accepted writes.
func RunRegistrationBurst(
	ctx context.Context,
	cfg BurstConfig,
	tasks []RegistrationTask,
	writes NativeWrites,
	ledger *WriteLedger,
) (BurstResult, error) {
	var result BurstResult
	if err := cfg.validate(tasks); err != nil {
		return result, err
	}
	if writes.Attempts == nil || writes.Metrics == nil || ledger == nil {
		return result, fmt.Errorf(
			"%w: complete burst measurement and write ledgers required",
			errLoadStage,
		)
	}
	if err := ctx.Err(); err != nil {
		result.Interrupted = true
		return result, fmt.Errorf("registration burst not started: %w", err)
	}
	var ready, workers sync.WaitGroup
	var inflight, maximum atomic.Int64
	var mu sync.Mutex
	var firstError error
	var firstStarted, lastStarted time.Time
	start := make(chan struct{})
	ready.Add(len(tasks))
	for _, task := range tasks {
		workers.Go(func() {
			ready.Done()
			<-start
			at := time.Now()
			active := inflight.Add(1)
			for old := maximum.Load(); active > old; old = maximum.Load() {
				if maximum.CompareAndSwap(old, active) {
					break
				}
			}
			task.Actor.Phase = BurstPhase
			measurement, accepted, err := writes.Apply(ctx, task.Actor, cfg.Event, task.Input)
			inflight.Add(-1)
			if accepted != nil {
				if ledgerErr := ledger.Accept(*accepted); ledgerErr != nil && err == nil {
					err = ledgerErr
				}
			}
			if metricErr := writes.Metrics.Observe(measurement); metricErr != nil && err == nil {
				err = metricErr
			}
			mu.Lock()
			defer mu.Unlock()
			if firstStarted.IsZero() || at.Before(firstStarted) {
				firstStarted = at
			}
			if at.After(lastStarted) {
				lastStarted = at
			}
			result.Attempted++
			if accepted != nil {
				result.Accepted++
			}
			if err != nil || measurement.Outcome != Accepted {
				result.FailedActions++
			}
			if firstError == nil && err != nil {
				firstError = err
			}
		})
	}
	ready.Wait()
	result.Started = time.Now()
	close(start)
	workers.Wait()
	result.Finished = time.Now()
	result.Elapsed = result.Finished.Sub(result.Started)
	result.ActionStartSpread = lastStarted.Sub(firstStarted)
	result.MaximumInflight = maximum.Load()
	result.Interrupted = ctx.Err() != nil
	result.Completed = !result.Interrupted && result.Attempted == len(tasks)
	if result.Interrupted {
		return result, fmt.Errorf("registration burst interrupted: %w", ctx.Err())
	}
	return result, firstError
}
