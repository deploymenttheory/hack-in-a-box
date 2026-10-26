package loadprofile

import (
	"strings"
	"testing"
	"time"
)

// Synthetic unit inputs exercise the report's fail-closed rules. They are not
// domain/provider/load proof and are never written as release evidence.
func reportInput(t *testing.T) Report {
	t.Helper()
	start := time.Now()
	var metrics Metrics
	for _, operation := range []Operation{SessionRead, RegistrationRead, RegistrationWrite, MentorWrite, HelpRead, HelpClaim, ClarificationSearch, ShowcaseSearch, DisplayUpdate, DueJobStart} {
		for range 100 {
			if err := metrics.Observe(
				Measurement{
					Operation: operation,
					Duration:  time.Millisecond,
					Status:    200,
					Outcome:   Accepted,
				},
			); err != nil {
				t.Fatal(err)
			}
		}
	}
	oracles := make([]Oracle, 0, len(RequiredOracles))
	for _, name := range RequiredOracles {
		oracles = append(
			oracles,
			Oracle{
				Name:           name,
				Passed:         true,
				Observations:   100,
				EvidenceSHA256: strings.Repeat("a", 64),
			},
		)
	}
	return Report{
		Source: "synthetic-unit-only",
		Tree:   "synthetic-unit-only",
		Images: map[string]string{
			"public":   "synthetic",
			"admin":    "synthetic",
			"worker":   "synthetic",
			"postgres": "synthetic",
		},
		Schema:   54,
		Profile:  "release",
		Started:  start,
		Finished: start.Add(30 * time.Minute),
		Elapsed:  30 * time.Minute,
		Host: Host{
			OS:                "synthetic",
			CPU:               "synthetic",
			LogicalCPUs:       4,
			MemoryBytes:       8 << 30,
			DockerVersion:     "synthetic",
			DockerCPUs:        4,
			DockerMemoryBytes: 8 << 30,
			DriverPlacement:   "synthetic",
			ServiceLimits: map[string]string{
				"public":   "synthetic",
				"admin":    "synthetic",
				"worker":   "synthetic",
				"postgres": "synthetic",
			},
		},
		Dataset: Dataset{
			Events:                5,
			CapacityPerEvent:      200,
			ConfirmedPerEvent:     []int{200, 200, 200, 200, 200},
			AuthenticatedSessions: 1000,
			Mentors:               5,
			DisplayStreams:        5,
		},
		Activity:        ActivityReport{Sessions: 1000, ActiveEveryMinute: 1000, Windows: 30},
		Operations:      metrics.Reports(),
		Integrity:       IntegrityReport{Accepted: 100, Committed: 100},
		Oracles:         oracles,
		ResourceSamples: 30,
		QueueSamples:    30,
		BurstCompleted:  true,
		ExternalTLS:     true,
		DurableVolumes:  true,
		WorkerWallClock: true,
	}
}

func TestSmokeAndSeededAccountsCannotProduceFullLoad(t *testing.T) {
	for _, mutate := range []func(*Report){func(r *Report) { r.Profile = "smoke" }, func(r *Report) { r.Elapsed = 29 * time.Minute }, func(r *Report) { r.Activity.ActiveEveryMinute = 999 }, func(r *Report) { r.Activity.Windows = 1 }, func(r *Report) { r.BurstCompleted = false }} {
		r := reportInput(t)
		mutate(&r)
		if result := r.Evaluate(); result.FullLoad || result.Passed {
			t.Fatal("partial run accepted", result)
		}
	}
}

func TestThresholdsAndIntegrityCannotBeWaivedByFullDuration(t *testing.T) {
	control := reportInput(t)
	if result := control.Evaluate(); !result.Passed || !result.FullLoad {
		t.Fatal(result)
	}
	for name, mutate := range map[string]func(*Report){"lost-write": func(r *Report) { r.Integrity.Missing = 1 }, "p95": func(r *Report) { r.Operations[0].P95 = 501 * time.Millisecond }, "missing-oracle": func(r *Report) { r.Oracles = r.Oracles[1:] }, "one-percent-errors": func(r *Report) { r.Operations[0].Outcomes[Accepted] = 92; r.Operations[0].Outcomes[Unexpected] = 8 }, "compressed-wall": func(r *Report) { r.Finished = r.Started.Add(time.Minute) }, "no-worker-wall": func(r *Report) { r.WorkerWallClock = false }, "missing-activity": func(r *Report) { r.Activity.MissingActorMinutes = 1 }} {
		t.Run(name, func(t *testing.T) {
			r := reportInput(t)
			mutate(&r)
			if result := r.Evaluate(); result.Passed {
				t.Fatal("failed release target passed", result)
			}
		})
	}
}
