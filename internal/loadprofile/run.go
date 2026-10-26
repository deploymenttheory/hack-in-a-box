package loadprofile

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// RuntimeConfig contains measured runtime controls, never an acceptance
// override. Release duration and saturation bounds are validated independently
// of this configuration by the actual monotonic stages.
type RuntimeConfig struct {
	Duration   time.Duration
	Interval   time.Duration
	Containers map[string]string
	Saturation SaturationConfig
}

func ReleaseRuntimeConfig(containers map[string]string) RuntimeConfig {
	return RuntimeConfig{
		Duration: 30 * time.Minute, Interval: 5 * time.Second,
		Containers: containers, Saturation: ReleaseSaturationConfig(),
	}
}

// RuntimeEvidence is aggregated workload evidence. Provider receipts and the
// deliberate current-permission/contention probes are separate native oracles;
// this type deliberately has no boolean that can certify them from traffic.
type RuntimeEvidence struct {
	Authentication AuthenticationReceipt `json:"authentication"`
	Burst          BurstResult           `json:"burst"`
	Sustained      StageResult           `json:"sustained"`
	Operations     []OperationReport     `json:"operations"`
	Integrity      IntegrityReport       `json:"integrity"`
	Attempts       AttemptReport         `json:"write_attempts"`
	Displays       DisplayReport         `json:"displays"`
	Due            DueReport             `json:"due"`
	Admission      []AdmissionFact       `json:"native_admission"`
	Resources      []ResourceSample      `json:"resource_samples"`
	Queues         []QueueSample         `json:"queue_samples"`
	SamplingErrors int                   `json:"sampling_errors"`
	StreamErrors   int                   `json:"stream_errors"`
	Saturation     SaturationResult      `json:"saturation"`
	// A workload receipt alone cannot satisfy the release's independent
	// privacy/provider/follow-up oracles or baseline-image provenance gates.
	ReleasePassed bool `json:"release_passed"`
}

// RunNativeProfile executes actual authenticated native traffic, not a seeded
// account-count proxy. Setup failure returns partial evidence. Runtime errors
// are retained while the complete configured workload drains, so a fast failed
// request cannot silently disappear from either the denominator or ledgers.
func RunNativeProfile(
	ctx context.Context,
	fixture PreparedFixture,
	config RuntimeConfig,
	transport http.RoundTripper,
	database NativeDatabase,
) (RuntimeEvidence, error) {
	var evidence RuntimeEvidence
	if err := validateRuntimeConfig(fixture, config, transport, database); err != nil {
		return evidence, err
	}
	actors, authentication, err := AuthenticateCohort(
		ctx,
		fixture.Cohort,
		fixture.Origins,
		fixture.Issuer,
		transport,
		20,
	)
	evidence.Authentication = authentication
	defer func() {
		for _, actor := range actors {
			actor.Session.Close()
		}
	}()
	if err != nil {
		return evidence, err
	}
	var events []uuid.UUID
	for _, event := range fixture.Events {
		events = append(events, event.ID)
	}
	if err := database.scoped(events); err != nil {
		return evidence, err
	}
	streams := make(map[string]uuid.UUID, len(fixture.Displays))
	for _, display := range fixture.Displays {
		streams[display.ID] = display.Event
	}
	displays, err := NewDisplayLedger(streams)
	if err != nil {
		return evidence, err
	}
	var metrics Metrics
	var attempts AttemptLedger
	var accepted WriteLedger
	writes := NativeWrites{Attempts: &attempts, Metrics: &metrics}
	tracking := CueTracking{Database: database, Displays: displays}
	workload, err := NewNativeWorkload(fixture.Journeys, writes, fixture.Run, tracking)
	if err != nil {
		return evidence, err
	}
	actorList := make([]Actor, len(actors))
	bySubject := make(map[string]Actor, len(actors))
	for index, actor := range actors {
		actorList[index] = actor.Actor
		bySubject[actor.ID] = actor.Actor
	}
	var failures []error
	var observationMu sync.Mutex
	observeContext, stopObserving := context.WithCancel(ctx)
	var observers sync.WaitGroup
	for _, display := range fixture.Displays {
		observers.Go(func() {
			err := ReadDisplay(
				observeContext,
				&http.Client{Transport: transport},
				fixture.Origins[PublicSurface],
				display.Token,
				display.ID,
				display.Event,
				display.Forbidden,
				displays,
			)
			if err != nil && observeContext.Err() == nil {
				observationMu.Lock()
				evidence.StreamErrors++
				observationMu.Unlock()
			}
		})
	}
	sample := func(sampleContext context.Context) ([]ResourceSample, QueueSample, error) {
		resources, resourceErr := DockerResources(sampleContext, config.Containers)
		queue, queueErr := database.Queue(sampleContext, events)
		return resources, queue, errors.Join(resourceErr, queueErr)
	}
	observers.Go(func() {
		tick := time.NewTicker(10 * time.Second)
		defer tick.Stop()
		for {
			resources, queue, err := sample(observeContext)
			observationMu.Lock()
			evidence.Resources = append(evidence.Resources, resources...)
			if !queue.At.IsZero() {
				evidence.Queues = append(evidence.Queues, queue)
			}
			if err != nil && observeContext.Err() == nil {
				evidence.SamplingErrors++
			}
			observationMu.Unlock()
			select {
			case <-observeContext.Done():
				return
			case <-tick.C:
			}
		}
	})
	defer func() { stopObserving(); observers.Wait() }()
	// Keep all declared people on ordinary native reads during the concurrent
	// registration barrier. The background action contains no write retry.
	readBindings := make(map[string]ReadBinding, len(fixture.Journeys))
	for subject, journey := range fixture.Journeys {
		readBindings[subject] = journey.Read
	}
	reads, err := NewNativeReads(readBindings)
	if err != nil {
		return evidence, err
	}
	backgroundContext, stopBackground := context.WithCancel(ctx)
	background := make(chan error, 1)
	go func() {
		_, err := RunStage(backgroundContext, StageConfig{
			Profile: fixture.Cohort.Profile,
			Phase:   BurstPhase, Duration: time.Hour, Interval: config.Interval,
		}, actorList, reads.Action, &metrics, &accepted)
		background <- err
	}()
	tasks := make([]RegistrationTask, 0, len(fixture.Burst.Applications))
	for subject, application := range fixture.Burst.Applications {
		tasks = append(tasks, RegistrationTask{Actor: bySubject[subject], Input: application})
	}
	evidence.Burst, err = RunRegistrationBurst(ctx, fixture.Burst.Config, tasks, writes, &accepted)
	stopBackground()
	backgroundErr := <-background
	if err != nil {
		failures = append(failures, fmt.Errorf("native registration burst: %w", err))
	}
	// Intentional background-phase cancellation is not a request error; the
	// HTTP measurements retain any actual failed in-flight reads separately.
	if backgroundErr != nil && !errors.Is(backgroundErr, context.Canceled) {
		failures = append(failures, backgroundErr)
	}
	evidence.Sustained, err = RunStage(ctx, StageConfig{
		Profile: fixture.Cohort.Profile, Phase: SustainedPhase,
		Duration: config.Duration, Interval: config.Interval,
	}, actorList, workload.Action, &metrics, &accepted)
	if err != nil {
		failures = append(failures, err)
	}
	// Freeze baseline source/due/latency observations BEFORE the intentionally
	// overloaded appendix. Accepted writes from that appendix still reconcile.
	failures = append(
		failures,
		evidence.collectBaseline(ctx, fixture, database, displays, &metrics, events)...)
	evidence.Saturation, err = RunSaturation(ctx, config.Saturation, actorList,
		func(_ int, pointMetrics *Metrics) (Action, error) {
			point, err := NewNativeWorkload(
				fixture.Journeys,
				NativeWrites{Attempts: &attempts, Metrics: pointMetrics},
				fixture.Run,
				tracking,
			)
			if err != nil {
				return nil, err
			}
			return point.Action, nil
		}, &accepted, sample)
	if err != nil {
		failures = append(failures, err)
	}
	stopObserving()
	observers.Wait()
	facts, commits, err := database.Effects(ctx, attempts.Specs())
	if err != nil {
		failures = append(failures, err)
	} else {
		evidence.Integrity = accepted.Reconcile(commits)
		evidence.Attempts = attempts.Reconcile(facts)
	}
	return evidence, errors.Join(failures...)
}

func (e *RuntimeEvidence) collectBaseline(
	ctx context.Context,
	fixture PreparedFixture,
	database NativeDatabase,
	displays *DisplayLedger,
	metrics *Metrics,
	events []uuid.UUID,
) []error {
	finished := time.Now()
	var failures []error
	var err error
	var samples []Measurement
	e.Displays, samples = displays.Measurements(finished)
	for _, measurement := range samples {
		if err := metrics.Observe(measurement); err != nil {
			failures = append(failures, err)
		}
	}
	intentIDs := make([]uuid.UUID, len(fixture.Due))
	for index, intent := range fixture.Due {
		intentIDs[index] = intent.ID
	}
	jobFacts, jobsErr := database.JobStarts(ctx, intentIDs)
	now, nowErr := database.DatabaseTime(ctx)
	if jobsErr == nil && nowErr == nil {
		e.Due, samples, err = DueStarts(fixture.Due, jobFacts, now, SustainedPhase)
		for _, measurement := range samples {
			if err := metrics.Observe(measurement); err != nil {
				failures = append(failures, err)
			}
		}
	} else {
		err = errors.Join(jobsErr, nowErr)
	}
	if err != nil {
		failures = append(failures, err)
	}
	e.Admission, err = database.Admission(ctx, events)
	if err != nil {
		failures = append(failures, err)
	}
	e.Operations = metrics.Reports()
	return failures
}

func validateRuntimeConfig(
	fixture PreparedFixture,
	config RuntimeConfig,
	transport http.RoundTripper,
	database NativeDatabase,
) error {
	if err := fixture.Validate(); err != nil {
		return err
	}
	if transport == nil || database.pool == nil || len(config.Containers) < 4 {
		return fmt.Errorf(
			"%w: verified transport, scoped database and explicit service containers required",
			errLoadStage,
		)
	}
	if err := (StageConfig{
		Profile: fixture.Cohort.Profile, Phase: SustainedPhase,
		Duration: config.Duration, Interval: config.Interval,
	}).Validate(len(fixture.Cohort.Members)); err != nil {
		return err
	}
	if config.Saturation.Profile != fixture.Cohort.Profile {
		return fmt.Errorf("%w: saturation and native cohort profiles must match", errLoadStage)
	}
	if err := config.Saturation.validate(len(fixture.Cohort.Members)); err != nil {
		return err
	}
	return nil
}
