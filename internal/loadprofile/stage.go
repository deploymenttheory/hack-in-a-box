package loadprofile

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

var errLoadStage = errors.New("invalid load stage")

type Actor struct {
	ID      string
	Session *Session
}

// Action owns native domain parsing, expected-conflict classification and
// original-response reconciliation identity. Returning a current read instead
// of an original ACK cannot establish an AcceptedWrite.
type Action func(context.Context, Actor, uint64) (Measurement, *AcceptedWrite, error)

type StageConfig struct {
	Profile            string
	Duration, Interval time.Duration
}

func (c StageConfig) Validate(actors int) error {
	if c.Profile != "release" && c.Profile != "smoke" {
		return fmt.Errorf(
			"%w: profile must be release or explicitly non-release smoke",
			errLoadStage,
		)
	}
	if c.Duration <= 0 || c.Duration > time.Hour || c.Interval <= 0 || c.Interval >= time.Minute {
		return fmt.Errorf(
			"%w: bounded positive stage duration and sub-minute per-actor interval required",
			errLoadStage,
		)
	}
	if actors < 1 {
		return fmt.Errorf("%w: authenticated actor cohort required", errLoadStage)
	}
	if c.Profile == "release" && (c.Duration < 30*time.Minute || actors < 1000) {
		return fmt.Errorf(
			"%w: release stage requires at least 1000 active sessions for 1800 real seconds",
			errLoadStage,
		)
	}
	return nil
}

type StageResult struct {
	Started, Finished time.Time
	Elapsed           time.Duration
	Activity          ActivityReport
	Executed          uint64
	Interrupted       bool
}

// RunStage uses the real monotonic clock. It never accepts an injected clock,
// compressed duration or pre-seeded activity. Each actor owns its sequential
// request stream; slow responses reduce achieved throughput visibly rather
// than spawning unbounded catch-up requests. A request already started at the
// boundary drains with its real client deadline, preserving uncertain writes
// for reconciliation rather than cancelling them to improve latency.
func RunStage(
	ctx context.Context,
	cfg StageConfig,
	actors []Actor,
	action Action,
	metrics *Metrics,
	ledger *WriteLedger,
) (StageResult, error) {
	var result StageResult
	if err := cfg.Validate(len(actors)); err != nil {
		return result, err
	}
	if action == nil || metrics == nil || ledger == nil {
		return result, fmt.Errorf(
			"%w: native action, metrics and accepted-write ledger required",
			errLoadStage,
		)
	}
	ids := make([]string, len(actors))
	people := make(map[string]struct{}, len(actors))
	for index, actor := range actors {
		if actor.Session == nil ||
			actor.Session.Person().String() == "00000000-0000-0000-0000-000000000000" {
			return result, fmt.Errorf(
				"%w: every active actor requires its own native authenticated session",
				errLoadStage,
			)
		}
		person := actor.Session.Person().String()
		if _, exists := people[person]; exists {
			return result, fmt.Errorf(
				"%w: the active cohort must represent distinct native people, not duplicated cookie jars",
				errLoadStage,
			)
		}
		people[person] = struct{}{}
		ids[index] = actor.ID
	}
	result.Started = time.Now()
	activity, err := NewActivity(result.Started, cfg.Duration, ids)
	if err != nil {
		return result, err
	}
	stop := time.NewTimer(cfg.Duration)
	defer stop.Stop()
	done := make(chan struct{})
	go func() {
		select {
		case <-stop.C:
		case <-ctx.Done():
		}
		close(done)
	}()
	var workers sync.WaitGroup
	var count atomic.Uint64
	var firstError error
	var errorOnce sync.Once
	recordError := func(err error) { errorOnce.Do(func() { firstError = err }) }
	for index, actor := range actors {
		workers.Add(1)
		go func(index int, actor Actor) {
			defer workers.Done()
			// Deterministic offsets spread the sustained cohort across its requested
			// interval. Burst admission is a separate explicitly concurrent stage.
			offset := time.Duration(index) * cfg.Interval / time.Duration(len(actors))
			delay := time.NewTimer(offset)
			defer delay.Stop()
			select {
			case <-delay.C:
			case <-done:
				return
			}
			for sequence := uint64(0); ; sequence++ {
				select {
				case <-done:
					return
				default:
				}
				measurement, write, actionErr := action(ctx, actor, sequence)
				if actionErr != nil {
					measurement.Outcome = Unexpected
				}
				if err := metrics.Observe(measurement); err != nil {
					recordError(err)
				}
				count.Add(1)
				if actionErr == nil && measurement.Outcome == Accepted {
					at := time.Now()
					if at.Sub(result.Started) < cfg.Duration {
						if err := activity.Authenticated(actor.ID, at); err != nil {
							recordError(err)
						}
					}
					if write != nil {
						if err := ledger.Accept(*write); err != nil {
							recordError(err)
						}
					}
				}
				next := time.NewTimer(cfg.Interval)
				select {
				case <-next.C:
				case <-done:
					if !next.Stop() {
						select {
						case <-next.C:
						default:
						}
					}
					return
				}
			}
		}(index, actor)
	}
	workers.Wait()
	result.Finished = time.Now()
	result.Elapsed = result.Finished.Sub(result.Started)
	result.Activity = activity.Report()
	result.Executed = count.Load()
	result.Interrupted = ctx.Err() != nil
	if result.Interrupted {
		return result, fmt.Errorf("load stage interrupted: %w", ctx.Err())
	}
	return result, firstError
}
