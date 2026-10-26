// Package loadprofile implements the release-capacity measurement contract.
// It contains no application handler, authentication bypass or fixture clock.
package loadprofile

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

var errLoadMeasurement = errors.New("invalid load metrics")

const maxLatencyMillis = 60_000

// Distribution retains an exact millisecond upper bound rather than samples or
// an average. Memory is bounded independently of the duration/request count.
// An observation above the range remains visible as overflow and cannot pass a
// percentile threshold through truncation.
type Distribution struct {
	Count    uint64        `json:"count"`
	Overflow uint64        `json:"overflow"`
	Maximum  time.Duration `json:"maximum_ns"`
	Total    time.Duration `json:"total_ns"`
	bins     []uint64
}

func (d *Distribution) Observe(value time.Duration) error {
	if value < 0 {
		return fmt.Errorf("%w: negative latency is not a measurement", errLoadMeasurement)
	}
	if d.bins == nil {
		d.bins = make([]uint64, maxLatencyMillis+1)
	}
	d.Count++
	d.Total += value
	if value > d.Maximum {
		d.Maximum = value
	}
	index := int(value / time.Millisecond)
	if value%time.Millisecond != 0 {
		index++
	}
	if index > maxLatencyMillis {
		d.Overflow++
		return nil
	}
	d.bins[index]++
	return nil
}

// Percentile returns a conservative upper bound. Overflow uses the observed
// maximum; it never disappears from the denominator.
func (d *Distribution) Percentile(fraction float64) (time.Duration, error) {
	if d.Count == 0 || math.IsNaN(fraction) || fraction <= 0 || fraction > 1 {
		return 0, fmt.Errorf(
			"%w: percentile requires observations and a fraction in (0,1]",
			errLoadMeasurement,
		)
	}
	target := uint64(math.Ceil(float64(d.Count) * fraction))
	var seen uint64
	for millis, count := range d.bins {
		seen += count
		if seen >= target {
			return time.Duration(millis) * time.Millisecond, nil
		}
	}
	return d.Maximum, nil
}

type Operation string

const (
	SessionRead         Operation = "session-read"
	RegistrationRead    Operation = "registration-read"
	RegistrationWrite   Operation = "registration-write"
	MentorRead          Operation = "mentor-read"
	MentorWrite         Operation = "mentor-write"
	HelpRead            Operation = "help-read"
	HelpClaim           Operation = "help-claim"
	HelpWrite           Operation = "help-write"
	CueRead             Operation = "cue-read"
	CueWrite            Operation = "cue-write"
	ClarificationSearch Operation = "clarification-search"
	ShowcaseSearch      Operation = "showcase-search"
	DisplayUpdate       Operation = "display-update"
	DueJobStart         Operation = "due-job-start"
)

func (o Operation) valid() bool {
	switch o {
	case SessionRead,
		RegistrationRead,
		RegistrationWrite,
		MentorRead,
		MentorWrite,
		HelpRead,
		HelpClaim,
		HelpWrite,
		CueRead,
		CueWrite,
		ClarificationSearch,
		ShowcaseSearch,
		DisplayUpdate,
		DueJobStart:
		return true
	}
	return false
}

func (o Operation) Limit() time.Duration {
	switch o {
	case RegistrationWrite, MentorWrite, HelpClaim, HelpWrite, CueWrite:
		return time.Second
	case DisplayUpdate:
		return 2 * time.Second
	case DueJobStart:
		return 30 * time.Second
	default:
		return 500 * time.Millisecond
	}
}

type Outcome string

const (
	Accepted          Outcome = "accepted"
	ExpectedDenial    Outcome = "expected-current-authority-denial"
	ExpectedConflict  Outcome = "expected-contention-conflict"
	CapacityRejection Outcome = "expected-capacity-rejection"
	Unexpected        Outcome = "unexpected"
)

type Measurement struct {
	Phase     Phase
	Surface   Surface
	Operation Operation
	Duration  time.Duration
	Status    int
	Outcome   Outcome
}

type OperationReport struct {
	Phase     Phase              `json:"phase"`
	Surface   Surface            `json:"surface"`
	Operation Operation          `json:"operation"`
	Count     uint64             `json:"count"`
	P50       time.Duration      `json:"p50_upper_ns"`
	P95       time.Duration      `json:"p95_upper_ns"`
	P99       time.Duration      `json:"p99_upper_ns"`
	Maximum   time.Duration      `json:"maximum_ns"`
	Overflow  uint64             `json:"overflow"`
	Limit     time.Duration      `json:"p95_limit_ns"`
	Statuses  map[int]uint64     `json:"statuses"`
	Outcomes  map[Outcome]uint64 `json:"outcomes"`
}

type operationMetrics struct {
	distribution Distribution
	statuses     map[int]uint64
	outcomes     map[Outcome]uint64
}

// Metrics accepts bounded operation/outcome labels. It never retains request
// URLs, bodies, cookies, subjects, private text or identifiers in a report.
type Metrics struct {
	mu         sync.Mutex
	operations map[seriesKey]*operationMetrics
}

func (m *Metrics) Observe(sample Measurement) error {
	if !sample.Phase.valid() || !sample.Surface.valid() {
		return fmt.Errorf("%w: explicit bounded phase and origin required", errLoadMeasurement)
	}
	if !sample.Operation.valid() {
		return fmt.Errorf("%w: unknown load operation", errLoadMeasurement)
	}
	switch sample.Outcome {
	case Accepted, ExpectedDenial, ExpectedConflict, CapacityRejection, Unexpected:
	default:
		return fmt.Errorf("%w: unknown load outcome", errLoadMeasurement)
	}
	if sample.Status < 0 || sample.Status > 599 {
		return fmt.Errorf("%w: invalid HTTP measurement status", errLoadMeasurement)
	}
	// A response code alone does not establish an expected rejection. A domain
	// oracle must classify it, and server/transport failures cannot be expected.
	if ((sample.Status == 0 && sample.Operation != DueJobStart) || sample.Status >= 500) &&
		sample.Outcome != Unexpected {
		return fmt.Errorf(
			"%w: transport/server failures must remain unexpected",
			errLoadMeasurement,
		)
	}
	if sample.Duration < 0 {
		return fmt.Errorf("%w: negative latency is not a measurement", errLoadMeasurement)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.operations == nil {
		m.operations = make(map[seriesKey]*operationMetrics)
	}
	key := seriesKey{sample.Phase, sample.Surface, sample.Operation}
	metric := m.operations[key]
	if metric == nil {
		metric = &operationMetrics{
			statuses: make(map[int]uint64),
			outcomes: make(map[Outcome]uint64),
		}
		m.operations[key] = metric
	}
	if err := metric.distribution.Observe(sample.Duration); err != nil {
		return err
	}
	metric.statuses[sample.Status]++
	metric.outcomes[sample.Outcome]++
	return nil
}

func (m *Metrics) Reports() []OperationReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	reports := make([]OperationReport, 0, len(m.operations))
	for _, phase := range phases {
		for _, surface := range surfaces {
			for _, operation := range []Operation{SessionRead, RegistrationRead, RegistrationWrite, MentorRead, MentorWrite, HelpRead, HelpClaim, HelpWrite, CueRead, CueWrite, ClarificationSearch, ShowcaseSearch, DisplayUpdate, DueJobStart} {
				metric := m.operations[seriesKey{phase, surface, operation}]
				if metric == nil {
					continue
				}
				p50, _ := metric.distribution.Percentile(.50)
				p95, _ := metric.distribution.Percentile(.95)
				p99, _ := metric.distribution.Percentile(.99)
				statuses := make(map[int]uint64)
				for status, count := range metric.statuses {
					statuses[status] = count
				}
				outcomes := make(map[Outcome]uint64)
				for outcome, count := range metric.outcomes {
					outcomes[outcome] = count
				}
				reports = append(
					reports,
					OperationReport{
						Phase: phase, Surface: surface,
						Operation: operation,
						Count:     metric.distribution.Count,
						P50:       p50,
						P95:       p95,
						P99:       p99,
						Maximum:   metric.distribution.Maximum,
						Overflow:  metric.distribution.Overflow,
						Limit:     operation.Limit(),
						Statuses:  statuses,
						Outcomes:  outcomes,
					},
				)
			}
		}
	}
	return reports
}
