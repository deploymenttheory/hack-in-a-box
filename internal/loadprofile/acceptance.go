package loadprofile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

// SourceOracleReceipt binds a native oracle's retained evidence to the exact
// source and workload being assessed. Producers must retain the original bytes;
// a digest or Passed flag alone is insufficient. This is evidence assembly,
// not a signature or a substitute for independently reviewing the producer.
type SourceOracleReceipt struct {
	Oracle
	Source        string `json:"source"`
	Tree          string `json:"tree"`
	RuntimeSHA256 string `json:"runtime_sha256"`
	Evidence      []byte `json:"evidence"`
}

type ReleaseDecision struct {
	RuntimeSHA256 string     `json:"runtime_sha256"`
	Report        Report     `json:"report"`
	Evaluation    Evaluation `json:"evaluation"`
}

// RuntimeDigest excludes the derived verdict so recording the decision cannot
// invalidate its own evidence. No runtime measurements are removed or rewritten.
func RuntimeDigest(runtime RuntimeEvidence) (string, error) {
	runtime.ReleasePassed = false
	raw, err := json.Marshal(runtime)
	if err != nil {
		return "", fmt.Errorf("encode native workload evidence: %w", err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

// EvaluateRelease overwrites caller-supplied measured report fields with the
// actual runtime receipt. Baseline contains independently inspected host/image/
// schema/placement facts, never a caller-provided completion verdict. runErr is
// the original workload/observer error and must not be discarded by the caller.
// This function changes only runtime.ReleasePassed, clearing an earlier verdict
// before every evaluation, including a failed re-evaluation.
func EvaluateRelease(
	runtime *RuntimeEvidence,
	baseline Report,
	receipts []SourceOracleReceipt,
	runErr error,
) ReleaseDecision {
	var decision ReleaseDecision
	if runtime == nil {
		decision.Evaluation.Failures = []string{"native workload evidence missing"}
		return decision
	}
	runtime.ReleasePassed = false
	r := baseline
	r.Started, r.Finished, r.Elapsed = runtime.Sustained.Started, runtime.Sustained.Finished, runtime.Sustained.Elapsed
	r.Activity, r.Operations = runtime.Sustained.Activity, runtime.Operations
	r.Integrity, r.Attempts = runtime.Integrity, runtime.Attempts
	r.BurstCompleted, r.SustainedCompleted = runtime.Burst.Completed, runtime.Sustained.Completed
	r.ResourceSamples, r.QueueSamples = baselineSampleCounts(*runtime)
	r.Dataset.AuthenticatedSessions = runtime.Authentication.Authenticated
	r.Dataset.PeoplePerEvent, r.Dataset.StaffPerEvent = nil, nil
	r.Dataset.ConfirmedPerEvent, r.Dataset.RegistrationCapacityPerEvent = nil, nil
	for _, fact := range runtime.Admission {
		r.Dataset.PeoplePerEvent = append(r.Dataset.PeoplePerEvent, fact.People)
		r.Dataset.StaffPerEvent = append(r.Dataset.StaffPerEvent, fact.Staff)
		r.Dataset.ConfirmedPerEvent = append(r.Dataset.ConfirmedPerEvent, fact.Confirmed)
		r.Dataset.RegistrationCapacityPerEvent = append(
			r.Dataset.RegistrationCapacityPerEvent,
			fact.ConfiguredCapacity,
		)
	}
	r.Dataset.Events = len(runtime.Admission)
	r.Oracles = nil
	for _, receipt := range receipts {
		r.Oracles = append(r.Oracles, receipt.Oracle)
	}
	decision.Report = r
	decision.Evaluation = r.Evaluate()
	fail := func(reason string) { decision.Evaluation.Failures = append(decision.Evaluation.Failures, reason) }
	if runErr != nil {
		// The error may contain a private response or provider detail. The
		// original stays with its private log; the public decision is safe.
		fail("native workload or observer returned an error; inspect private run log")
	}
	digest, err := RuntimeDigest(*runtime)
	decision.RuntimeSHA256 = digest
	if err != nil {
		fail("native workload cannot be encoded for source binding")
	}
	if !lowerHex(r.Source, 40) || !lowerHex(r.Tree, 40) {
		fail("exact source commit and tree IDs missing")
	}
	for _, name := range []string{"public", "admin", "worker", "postgres"} {
		if !strings.HasPrefix(r.Images[name], "sha256:") ||
			!lowerHex(strings.TrimPrefix(r.Images[name], "sha256:"), 64) ||
			r.Host.ServiceLimits[name] == "" {
			fail(name + ": immutable baseline image or resource limits missing")
		}
	}
	required := make(map[string]bool, len(requiredOracles))
	for _, name := range requiredOracles {
		required[name] = true
	}
	seen := make(map[string]bool, len(receipts))
	for _, receipt := range receipts {
		payload := sha256.Sum256(receipt.Evidence)
		if !required[receipt.Name] || seen[receipt.Name] || receipt.Source != r.Source ||
			receipt.Tree != r.Tree ||
			receipt.RuntimeSHA256 != digest ||
			len(receipt.Evidence) == 0 ||
			!json.Valid(receipt.Evidence) ||
			receipt.EvidenceSHA256 != hex.EncodeToString(payload[:]) {
			fail(
				"native oracle evidence is duplicate, unrecognised, missing or bound to another source/workload",
			)
		}
		seen[receipt.Name] = true
	}
	validateRuntimeFacts(*runtime, fail)
	validateSaturation(runtime.Saturation, runtime.Sustained.Finished, fail)
	decision.Evaluation.Passed = len(decision.Evaluation.Failures) == 0
	runtime.ReleasePassed = decision.Evaluation.Passed
	return decision
}

func lowerHex(value string, length int) bool {
	return len(value) == length && strings.Trim(value, "0123456789abcdef") == ""
}

func validStage(stage StageResult, minimum time.Duration) bool {
	return stage.Completed && !stage.Interrupted && stage.Executed > 0 && !stage.Started.IsZero() &&
		stage.Elapsed >= minimum &&
		!stage.PhaseEnded.Before(stage.Started.Add(stage.Elapsed)) &&
		!stage.Finished.Before(stage.PhaseEnded) &&
		stage.Activity.Sessions == 1000 &&
		stage.Activity.ActiveEveryMinute == 1000 &&
		stage.Activity.MissingActorMinutes == 0 &&
		stage.Activity.Windows >= int((minimum+time.Minute-1)/time.Minute)
}

func validateRuntimeFacts(runtime RuntimeEvidence, fail func(string)) {
	auth := runtime.Authentication
	if auth.Started.IsZero() || auth.Elapsed <= 0 || auth.Attempted != 1000 ||
		auth.Authenticated != 1000 ||
		auth.DistinctPeople != 1000 ||
		auth.Failed != 0 ||
		auth.Unattempted != 0 {
		fail("1000 distinct actual native sign-ins not proven")
	}
	if !validStage(runtime.Sustained, 30*time.Minute) {
		fail("actual uninterrupted sustained stage and every-minute cohort not proven")
	}
	validateBurst(runtime.Burst, runtime.Sustained.Started, fail)
	validateObservations(runtime, fail)
}

func validateBurst(burst BurstResult, sustainedStart time.Time, fail func(string)) {
	if !burst.Completed || burst.Interrupted || burst.Started.IsZero() || burst.Elapsed <= 0 ||
		burst.Finished.Before(burst.Started.Add(burst.Elapsed)) ||
		burst.Attempted != 200 ||
		burst.Accepted != 200 ||
		burst.FailedActions != 0 ||
		burst.MaximumInflight < 2 ||
		burst.ActionStartSpread < 0 ||
		burst.ActionStartSpread > burst.Elapsed ||
		sustainedStart.Before(burst.Finished) {
		fail("actual 200-person concurrent registration burst not proven")
	}
}

func validateObservations(runtime RuntimeEvidence, fail func(string)) {
	if runtime.SamplingErrors != 0 || runtime.StreamErrors != 0 {
		fail("native resource/queue sampling or display stream failed")
	}
	if !runtime.Due.Complete() {
		fail("healthy-provider due intents have missing, duplicate or premature starts")
	}
	display := runtime.Displays
	if display.Publications == 0 || display.ExpectedReceipts == 0 ||
		display.MatchedReceipts != display.ExpectedReceipts ||
		display.MissingReceipts != 0 ||
		display.MismatchedReceipts != 0 ||
		display.InvalidClockOrder != 0 {
		fail("matching native display source/body receipts incomplete")
	}
	events := make(map[string]bool, len(runtime.Admission))
	for _, fact := range runtime.Admission {
		id := fact.EventID.String()
		if fact.EventID == [16]byte{} || events[id] || fact.DuplicateActive != 0 ||
			fact.ExcludedCompetitors != 0 ||
			fact.ClaimedConfirmed != fact.Confirmed ||
			fact.Offered != 0 {
			fail("native admission identity, authority or capacity facts inconsistent")
		}
		events[id] = true
	}
}

func validateSaturation(result SaturationResult, after time.Time, fail func(string)) {
	if !result.Release || !result.FullCohort || !result.Completed || len(result.Points) < 2 {
		fail("measured release saturation curve with full cohort missing")
		return
	}
	previousInterval := time.Minute
	limited := false
	for _, point := range result.Points {
		resourceSamples, queueSamples := baselineSampleCounts(
			RuntimeEvidence{
				Sustained: point.Stage,
				Resources: point.Resources,
				Queues:    point.Queues,
			},
		)
		if limited || point.Interval <= 0 || point.Interval >= previousInterval ||
			point.Stage.Started.Before(after) ||
			!validStage(point.Stage, time.Minute) ||
			point.HTTPRequests == 0 ||
			point.RequestsSecond <= 0 ||
			point.SampleFailures != 0 ||
			resourceSamples == 0 ||
			queueSamples == 0 ||
			len(point.Operations) == 0 {
			fail(
				"saturation points lack actual ordered windows, traffic or resource/queue evidence",
			)
		}
		validatePressureMeasurements(point, fail)
		limited = len(point.LimitReasons) > 0
		previousInterval, after = point.Interval, point.Stage.Finished
	}
	if result.LimitReached != limited {
		fail("saturation limit declaration contradicts measured curve")
	}
	if !limited && len(result.Points) != len(ReleaseSaturationConfig().Intervals) {
		fail("saturation curve ended before an observed limit or the complete pressure range")
	}
}

func validatePressureMeasurements(point SaturationPoint, fail func(string)) {
	var requests, unexpected uint64
	breached := false
	for _, operation := range point.Operations {
		var outcomes, statuses uint64
		for _, count := range operation.Outcomes {
			outcomes += count
		}
		for _, count := range operation.Statuses {
			statuses += count
		}
		if operation.Phase != SaturationPhase || !operation.Operation.valid() ||
			!operation.Surface.valid() ||
			outcomes != operation.Count ||
			statuses != operation.Count ||
			operation.Count == 0 {
			fail("saturation operation labels or complete request accounting invalid")
		}
		if operation.Operation == DisplayUpdate || operation.Operation == DueJobStart {
			continue
		}
		requests += operation.Count
		unexpected += operation.Outcomes[Unexpected]
		breached = breached || operation.P95 > operation.Operation.Limit()
	}
	if requests == 0 || requests != point.HTTPRequests {
		fail("saturation HTTP request total differs from measured operations")
		return
	}
	rate := float64(unexpected) / float64(requests)
	span := point.Stage.Finished.Sub(point.Stage.Started).Seconds()
	if span <= 0 || math.IsNaN(point.RequestsSecond) || math.IsInf(point.RequestsSecond, 0) ||
		math.Abs(point.RequestsSecond-float64(requests)/span) > 0.000001 ||
		math.IsNaN(point.UnexpectedRate) ||
		math.Abs(point.UnexpectedRate-rate) > 0.000001 {
		fail("saturation achieved throughput or error fraction differs from measured operations")
	}
	if (breached || rate >= .01) != (len(point.LimitReasons) > 0) {
		fail("saturation limit declaration contradicts measured target breaches")
	}
}

// Saturation sampling continues after baseline collection. Those later rows
// cannot repair missing sustained resource or queue observations.
func baselineSampleCounts(runtime RuntimeEvidence) (int, int) {
	resources, queues := len(runtime.Resources), 0
	within := func(at time.Time) bool {
		return !at.Before(runtime.Sustained.Started) && !at.After(runtime.Sustained.Finished)
	}
	services := map[string]int{"public": 0, "admin": 0, "worker": 0, "postgres": 0}
	for _, sample := range runtime.Resources {
		if _, known := services[sample.Service]; known && within(sample.At) &&
			sample.CPUPercent != "" &&
			sample.MemoryUsage != "" {
			services[sample.Service]++
		}
	}
	// The least-observed required service determines the usable series length.
	for _, count := range services {
		if count < resources {
			resources = count
		}
	}
	for _, sample := range runtime.Queues {
		if within(sample.At) && !sample.At.IsZero() && sample.PendingByKind != nil {
			queues++
		}
	}
	return resources, queues
}
