package loadprofile

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Synthetic samples exercise curve accounting only. No native API/provider,
// resource probe or load result is established or exported by these controls.
func TestSaturationKeepsPointLocalSubrequestsAndStopsAtMeasuredBoundary(t *testing.T) {
	factory := func(index int, metrics *Metrics) (Action, error) {
		return func(_ context.Context, actor Actor, _ uint64) (Measurement, *AcceptedWrite, error) {
			preflight := Measurement{Phase: actor.Phase, Surface: AdminSurface, Operation: CueRead,
				Duration: time.Microsecond, Status: 200, Outcome: Accepted}
			if err := metrics.Observe(preflight); err != nil {
				return preflight, nil, err
			}
			read := preflight
			read.Operation = SessionRead
			if index == 1 {
				read.Duration = 501 * time.Millisecond
			}
			return read, nil, nil
		}, nil
	}
	sampler := func(context.Context) ([]ResourceSample, QueueSample, error) {
		return []ResourceSample{{At: time.Now(), Service: "synthetic-component-input"}}, QueueSample{At: time.Now()}, nil
	}
	result, err := RunSaturation(t.Context(), SaturationConfig{Profile: "smoke", Duration: 20 * time.Millisecond,
		Intervals: []time.Duration{3 * time.Millisecond, 2 * time.Millisecond, time.Millisecond}},
		[]Actor{{ID: "synthetic", Session: &Session{person: uuid.New()}}}, factory, new(WriteLedger), sampler)
	if err != nil || !result.Completed || !result.LimitReached || result.Release || result.FullCohort || len(result.Points) != 2 {
		t.Fatal("diagnostic curve or observed stop misrepresented", result, err)
	}
	for index, point := range result.Points {
		if len(point.Operations) != 2 || point.HTTPRequests != 2*point.Stage.Executed ||
			point.RequestsSecond <= 0 || len(point.Resources) < 2 || len(point.Queues) < 2 {
			t.Fatal("point-local native subrequests or observations hidden", index, point)
		}
		if (index == 0 && len(point.LimitReasons) != 0) || (index == 1 && len(point.LimitReasons) != 1) {
			t.Fatal("pressure limit did not come from actual point distributions", index, point)
		}
	}
}

func TestReleaseSaturationCannotCompressWindowsOrReversePressure(t *testing.T) {
	for _, change := range []func(*SaturationConfig){
		func(c *SaturationConfig) { c.Duration = 59 * time.Second },
		func(c *SaturationConfig) { c.Intervals = []time.Duration{time.Second} },
		func(c *SaturationConfig) { c.Intervals = []time.Duration{time.Second, 2 * time.Second} },
	} {
		cfg := ReleaseSaturationConfig()
		change(&cfg)
		if err := cfg.validate(1000); err == nil {
			t.Fatal("incomplete release pressure curve accepted", cfg)
		}
	}
	if err := ReleaseSaturationConfig().validate(999); err == nil {
		t.Fatal("smaller cohort became full release pressure")
	}
}
