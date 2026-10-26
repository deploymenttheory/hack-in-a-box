package loadprofile

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// PointSampler reads only the baseline's explicitly scoped Docker containers
// and native database. Its failures remain evidence, not empty successful rows.
type PointSampler func(context.Context) ([]ResourceSample, QueueSample, error)

// PointActionFactory binds every native subrequest to the point-local metrics,
// including mentor/cue preflights and helper queue reads.
type PointActionFactory func(int, *Metrics) (Action, error)

type SaturationConfig struct {
	Profile   string
	Duration  time.Duration
	Intervals []time.Duration
}

func ReleaseSaturationConfig() SaturationConfig {
	return SaturationConfig{
		Profile: "release", Duration: time.Minute,
		Intervals: []time.Duration{
			20 * time.Second,
			10 * time.Second,
			5 * time.Second,
			2 * time.Second,
			time.Second,
			500 * time.Millisecond,
			100 * time.Millisecond,
			10 * time.Millisecond,
		},
	}
}

func (c SaturationConfig) validate(actors int) error {
	if c.Profile != "release" && c.Profile != "smoke" {
		return fmt.Errorf("%w: explicit saturation profile required", errLoadStage)
	}
	if len(c.Intervals) < 2 || len(c.Intervals) > 12 || c.Duration <= 0 || c.Duration > time.Hour ||
		(c.Profile == "release" && (actors != 1000 || c.Duration < time.Minute)) {
		return fmt.Errorf(
			"%w: measured pressure curve and full release cohort required",
			errLoadStage,
		)
	}
	for index, interval := range c.Intervals {
		if interval <= 0 || interval >= time.Minute ||
			(index > 0 && interval >= c.Intervals[index-1]) {
			return fmt.Errorf("%w: saturation intervals must strictly decrease", errLoadStage)
		}
	}
	return nil
}

type SaturationPoint struct {
	Interval       time.Duration     `json:"think_interval_ns"`
	Stage          StageResult       `json:"actual_stage"`
	Operations     []OperationReport `json:"operations"`
	HTTPRequests   uint64            `json:"http_requests"`
	RequestsSecond float64           `json:"achieved_http_requests_per_second_including_drain"`
	UnexpectedRate float64           `json:"unexpected_http_error_rate"`
	Resources      []ResourceSample  `json:"resource_samples"`
	Queues         []QueueSample     `json:"queue_samples"`
	SampleFailures int               `json:"sampling_failures"`
	LimitReasons   []string          `json:"observed_limit_reasons"`
}

type SaturationResult struct {
	Points       []SaturationPoint `json:"points"`
	Completed    bool              `json:"completed"`
	LimitReached bool              `json:"limit_reached"`
	FullCohort   bool              `json:"full_1000_person_cohort"`
	Release      bool              `json:"release_profile"`
}

// RunSaturation runs real monotonic windows after baseline oracles have been
// frozen. Target breaches delimit the observed pressure boundary; they never
// revise or relabel baseline targets. Actual request mix, status distributions,
// resource/queue samples and accepted writes remain retained at every point.
func RunSaturation(
	ctx context.Context,
	cfg SaturationConfig,
	actors []Actor,
	factory PointActionFactory,
	ledger *WriteLedger,
	sampler PointSampler,
) (SaturationResult, error) {
	result := SaturationResult{Release: cfg.Profile == "release"}
	if err := cfg.validate(len(actors)); err != nil {
		return result, err
	}
	if _, err := cohortIdentities(actors); err != nil {
		return result, err
	}
	result.FullCohort = len(actors) == 1000
	if sampler == nil || factory == nil || ledger == nil {
		return result, fmt.Errorf(
			"%w: actual resource and queue observation required",
			errLoadStage,
		)
	}
	for index, interval := range cfg.Intervals {
		point, err := runSaturationPoint(
			ctx,
			cfg,
			interval,
			index,
			actors,
			factory,
			ledger,
			sampler,
		)
		result.Points = append(result.Points, point)
		if err != nil {
			return result, err
		}
		if len(point.LimitReasons) != 0 {
			result.LimitReached = true
			break
		}
	}
	result.Completed = len(result.Points) >= 2
	if !result.Completed {
		return result, fmt.Errorf(
			"%w: initial pressure already exceeds its target; no complete curve",
			errLoadStage,
		)
	}
	return result, nil
}

func runSaturationPoint(
	ctx context.Context,
	cfg SaturationConfig,
	interval time.Duration,
	index int,
	actors []Actor,
	factory PointActionFactory,
	ledger *WriteLedger,
	sampler PointSampler,
) (SaturationPoint, error) {
	point := SaturationPoint{Interval: interval}
	var metrics Metrics
	action, err := factory(index, &metrics)
	if err != nil || action == nil {
		return point, fmt.Errorf("%w: point-local native workload unavailable", errLoadStage)
	}
	var mu sync.Mutex
	sample := func() {
		bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		resources, queue, err := sampler(bounded)
		mu.Lock()
		defer mu.Unlock()
		if err != nil || len(resources) == 0 || queue.At.IsZero() {
			point.SampleFailures++
			return
		}
		point.Resources = append(point.Resources, resources...)
		point.Queues = append(point.Queues, queue)
	}
	sample()
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				sample()
			case <-stop:
				return
			}
		}
	}()
	stage, err := RunStage(ctx, StageConfig{
		Profile: cfg.Profile, Duration: cfg.Duration,
		Interval: interval, Phase: SaturationPhase,
	}, actors, action, &metrics, ledger)
	close(stop)
	<-stopped
	sample()
	point.Stage = stage
	point.Operations = metrics.Reports()
	var unexpected uint64
	for _, operation := range point.Operations {
		if operation.Operation == DisplayUpdate || operation.Operation == DueJobStart {
			continue
		}
		point.HTTPRequests += operation.Count
		unexpected += operation.Outcomes[Unexpected]
		if operation.P95 > operation.Operation.Limit() {
			point.LimitReasons = append(point.LimitReasons,
				string(operation.Surface)+"/"+string(operation.Operation)+": p95 target exceeded")
		}
	}
	if completeSpan := stage.Finished.Sub(
		stage.Started,
	); point.HTTPRequests > 0 &&
		completeSpan > 0 {
		point.RequestsSecond = float64(point.HTTPRequests) / completeSpan.Seconds()
		point.UnexpectedRate = float64(unexpected) / float64(point.HTTPRequests)
	}
	if point.UnexpectedRate >= .01 {
		point.LimitReasons = append(
			point.LimitReasons,
			"unexpected HTTP error rate is not below one percent",
		)
	}
	if err != nil {
		return point, err
	}
	if !stage.Completed || point.HTTPRequests == 0 || point.SampleFailures != 0 {
		return point, fmt.Errorf(
			"%w: pressure window or resource/queue evidence incomplete",
			errLoadStage,
		)
	}
	if cfg.Profile == "release" && stage.Activity.ActiveEveryMinute != 1000 {
		return point, fmt.Errorf(
			"%w: pressure window did not keep 1000 distinct people active",
			errLoadStage,
		)
	}
	return point, nil
}
