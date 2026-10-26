package loadprofile

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// These are deliberately synthetic gate inputs, never native load evidence.
func acceptanceInput(t *testing.T) (RuntimeEvidence, Report, []SourceOracleReceipt) {
	t.Helper()
	r := reportInput(t)
	r.Source, r.Tree = strings.Repeat("a", 40), strings.Repeat("b", 40)
	for name := range r.Images {
		r.Images[name] = "sha256:" + strings.Repeat("c", 64)
	}
	start := r.Started
	stage := func(at time.Time, duration time.Duration) StageResult {
		return StageResult{
			Started:    at,
			PhaseEnded: at.Add(duration),
			Finished:   at.Add(duration),
			Elapsed:    duration,
			Completed:  true,
			Executed:   1000,
			Activity: ActivityReport{
				Sessions:          1000,
				ActiveEveryMinute: 1000,
				Windows:           int(duration / time.Minute),
			},
		}
	}
	runtime := RuntimeEvidence{
		Authentication: AuthenticationReceipt{
			Started:        start.Add(-time.Minute),
			Elapsed:        time.Second,
			Attempted:      1000,
			Authenticated:  1000,
			DistinctPeople: 1000,
		},
		Burst: BurstResult{
			Started:         start.Add(-time.Second),
			Finished:        start,
			Elapsed:         time.Second,
			Attempted:       200,
			Accepted:        200,
			MaximumInflight: 200,
			Completed:       true,
		},
		Sustained: stage(
			start,
			30*time.Minute,
		),
		Operations: r.Operations,
		Integrity:  r.Integrity,
		Attempts:   r.Attempts,
		Displays:   DisplayReport{Publications: 30, ExpectedReceipts: 30, MatchedReceipts: 30},
		Due:        DueReport{Expected: 30, Due: 30, Started: 30},
		Saturation: SaturationResult{Completed: true, FullCohort: true, Release: true},
	}
	for index := range 30 {
		at := start.Add(time.Duration(index) * time.Minute)
		for _, service := range []string{"public", "admin", "worker", "postgres"} {
			runtime.Resources = append(
				runtime.Resources,
				ResourceSample{
					At:          at,
					Service:     service,
					CPUPercent:  "1%",
					MemoryUsage: "1MiB / 1GiB",
				},
			)
		}
		runtime.Queues = append(
			runtime.Queues,
			QueueSample{At: at, PendingByKind: map[string]int64{}},
		)
	}
	for range 5 {
		runtime.Admission = append(
			runtime.Admission,
			AdmissionFact{
				EventID:            uuid.New(),
				ConfiguredCapacity: 180,
				Confirmed:          180,
				ClaimedConfirmed:   180,
				Staff:              20,
				People:             200,
			},
		)
	}
	for index, interval := range []time.Duration{20 * time.Second, 10 * time.Second} {
		at := start.Add(time.Duration(30+index) * time.Minute)
		var resources []ResourceSample
		for _, service := range []string{"public", "admin", "worker", "postgres"} {
			resources = append(
				resources,
				ResourceSample{
					At:          at,
					Service:     service,
					CPUPercent:  "1%",
					MemoryUsage: "1MiB / 1GiB",
				},
			)
		}
		operation := r.Operations[0]
		operation.Phase = SaturationPhase
		if index == 1 {
			operation.P95 = operation.Operation.Limit() + time.Millisecond
		}
		runtime.Saturation.Points = append(
			runtime.Saturation.Points,
			SaturationPoint{
				Interval:       interval,
				Stage:          stage(start.Add(time.Duration(30+index)*time.Minute), time.Minute),
				HTTPRequests:   operation.Count,
				RequestsSecond: float64(operation.Count) / 60,
				Operations:     []OperationReport{operation},
				Resources:      resources,
				Queues:         []QueueSample{{At: at, PendingByKind: map[string]int64{}}},
			},
		)
	}
	runtime.Saturation.Points[1].LimitReasons = []string{"synthetic-unit-only observed limit"}
	runtime.Saturation.LimitReached = true
	return runtime, r, bindAcceptanceReceipts(t, runtime, r)
}

func bindAcceptanceReceipts(
	t *testing.T,
	runtime RuntimeEvidence,
	report Report,
) []SourceOracleReceipt {
	t.Helper()
	digest, err := RuntimeDigest(runtime)
	if err != nil {
		t.Fatal(err)
	}
	var receipts []SourceOracleReceipt
	for _, name := range RequiredOracleNames() {
		payload := []byte(`{"synthetic_unit_only":"` + name + `"}`)
		hash := sha256.Sum256(payload)
		receipts = append(
			receipts,
			SourceOracleReceipt{
				Oracle: Oracle{
					Name:           name,
					Passed:         true,
					Observations:   100,
					EvidenceSHA256: hex.EncodeToString(hash[:]),
				},
				Source:        report.Source,
				Tree:          report.Tree,
				RuntimeSHA256: digest,
				Evidence:      payload,
			},
		)
	}
	return receipts
}

func TestReleaseAcceptanceUsesActualRuntimeAndRetainedSourceBoundOracles(t *testing.T) {
	runtime, baseline, receipts := acceptanceInput(t)
	// Caller supplied measured fields are deliberately false; actual runtime wins.
	baseline.Elapsed, baseline.BurstCompleted, baseline.SustainedCompleted = 0, false, false
	baseline.Activity, baseline.Operations = ActivityReport{}, nil
	decision := EvaluateRelease(&runtime, baseline, receipts, nil)
	if !decision.Evaluation.Passed || !runtime.ReleasePassed || !decision.Evaluation.FullLoad {
		t.Fatal(decision.Evaluation)
	}
	digest, err := RuntimeDigest(runtime)
	if err != nil || digest != decision.RuntimeSHA256 {
		t.Fatal("derived verdict changed workload binding", digest, err)
	}
	decision = EvaluateRelease(
		&runtime,
		baseline,
		receipts,
		errors.New("private response must not be published"),
	)
	if runtime.ReleasePassed || decision.Evaluation.Passed ||
		strings.Contains(strings.Join(decision.Evaluation.Failures, " "), "private response") {
		t.Fatal("original error was ignored or leaked", decision.Evaluation)
	}
}

func TestReleaseAcceptanceRejectsIncompleteRuntimeDespitePassingCallerReport(t *testing.T) {
	for name, mutate := range map[string]func(*RuntimeEvidence){
		"short-stage":              func(r *RuntimeEvidence) { r.Sustained.Elapsed = 29 * time.Minute },
		"drain-padded-stage":       func(r *RuntimeEvidence) { r.Sustained.PhaseEnded = r.Sustained.Started.Add(time.Minute) },
		"seeded-session-count":     func(r *RuntimeEvidence) { r.Authentication.DistinctPeople = 999 },
		"missing-minute":           func(r *RuntimeEvidence) { r.Sustained.Activity.MissingActorMinutes++ },
		"serial-burst":             func(r *RuntimeEvidence) { r.Burst.MaximumInflight = 1 },
		"lost-write":               func(r *RuntimeEvidence) { r.Attempts.Unreconciled++ },
		"missing-sse":              func(r *RuntimeEvidence) { r.Displays.MissingReceipts++ },
		"missing-due":              func(r *RuntimeEvidence) { r.Due.Unstarted++ },
		"stream-error":             func(r *RuntimeEvidence) { r.StreamErrors++ },
		"sampling-error":           func(r *RuntimeEvidence) { r.SamplingErrors++ },
		"duplicate-event":          func(r *RuntimeEvidence) { r.Admission[1].EventID = r.Admission[0].EventID },
		"over-capacity":            func(r *RuntimeEvidence) { r.Admission[0].Confirmed++ },
		"missing-saturation":       func(r *RuntimeEvidence) { r.Saturation.Points = nil },
		"smoke-saturation":         func(r *RuntimeEvidence) { r.Saturation.Release = false },
		"short-pressure":           func(r *RuntimeEvidence) { r.Saturation.Points[0].Stage.Elapsed = time.Second },
		"overlap-pressure":         func(r *RuntimeEvidence) { r.Saturation.Points[1].Stage.Started = r.Sustained.Started },
		"inactive-pressure":        func(r *RuntimeEvidence) { r.Saturation.Points[1].Stage.Activity.ActiveEveryMinute-- },
		"missing-pressure-samples": func(r *RuntimeEvidence) { r.Saturation.Points[1].Resources = nil },
		"skipped-pressure-level":   func(r *RuntimeEvidence) { r.Saturation.Points[1].Interval = 5 * time.Second },
		"wrong-pressure-range": func(r *RuntimeEvidence) {
			r.Saturation.Points[0].Interval = 59 * time.Second
			r.Saturation.Points[1].Interval = 58 * time.Second
		},
		"invented-pressure-limit": func(r *RuntimeEvidence) { r.Saturation.LimitReached = false },
		"saturation-cannot-pad-baseline-samples": func(r *RuntimeEvidence) {
			for index := range r.Resources {
				r.Resources[index].At = r.Sustained.Finished.Add(time.Minute)
			}
		},
		"missing-worker-samples": func(r *RuntimeEvidence) {
			for index := range r.Resources {
				if r.Resources[index].Service == "worker" {
					r.Resources[index].Service = "other"
				}
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			runtime, baseline, _ := acceptanceInput(t)
			mutate(&runtime)
			// Bind even the defective runtime: a correct digest cannot make bad
			// native measurements pass through a previously passing report.
			receipts := bindAcceptanceReceipts(t, runtime, baseline)
			runtime.ReleasePassed = true
			decision := EvaluateRelease(&runtime, baseline, receipts, nil)
			if decision.Evaluation.Passed || runtime.ReleasePassed {
				t.Fatal("incomplete runtime accepted", decision)
			}
		})
	}
}

func TestReleaseAcceptanceRejectsUnboundOracleEvidence(t *testing.T) {
	for name, mutate := range map[string]func(*Report, *[]SourceOracleReceipt){
		"missing":           func(_ *Report, r *[]SourceOracleReceipt) { *r = (*r)[1:] },
		"duplicate":         func(_ *Report, r *[]SourceOracleReceipt) { *r = append(*r, (*r)[0]) },
		"wrong-source":      func(_ *Report, r *[]SourceOracleReceipt) { (*r)[0].Source = strings.Repeat("d", 40) },
		"wrong-tree":        func(_ *Report, r *[]SourceOracleReceipt) { (*r)[0].Tree = strings.Repeat("d", 40) },
		"wrong-run":         func(_ *Report, r *[]SourceOracleReceipt) { (*r)[0].RuntimeSHA256 = strings.Repeat("d", 64) },
		"changed-payload":   func(_ *Report, r *[]SourceOracleReceipt) { (*r)[0].Evidence = []byte(`{"different":true}`) },
		"digest-only":       func(_ *Report, r *[]SourceOracleReceipt) { (*r)[0].Evidence = nil },
		"mutable-image-tag": func(b *Report, _ *[]SourceOracleReceipt) { b.Images["worker"] = "hackbox:latest" },
		"missing-named-image": func(b *Report, _ *[]SourceOracleReceipt) {
			b.Images["other"] = b.Images["worker"]
			delete(b.Images, "worker")
		},
	} {
		t.Run(name, func(t *testing.T) {
			runtime, baseline, receipts := acceptanceInput(t)
			mutate(&baseline, &receipts)
			if result := EvaluateRelease(
				&runtime,
				baseline,
				receipts,
				nil,
			); result.Evaluation.Passed ||
				runtime.ReleasePassed {
				t.Fatal("unbound oracle/baseline accepted", result)
			}
		})
	}
}
