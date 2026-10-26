package loadprofile

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// DueExpectation is captured from the actual approved native owner intent.
// It selects runnable healthy-provider work, never the aggregate pending gauge
// containing non-runnable automation source facts such as teams.published.
type DueExpectation struct {
	ID, Event uuid.UUID
	Kind, Key string
	Due       time.Time
}

type DueReport struct {
	Expected          int `json:"expected"`
	Due               int `json:"due"`
	NotYetDue         int `json:"not_yet_due"`
	Started           int `json:"started"`
	Unstarted         int `json:"unstarted"`
	DuplicateAttempts int `json:"duplicate_attempts"`
	PrematureStarts   int `json:"premature_starts"`
}

func (r DueReport) Complete() bool {
	return r.Expected > 0 && r.Due == r.Expected && r.Started == r.Expected &&
		r.Unstarted == 0 && r.NotYetDue == 0 && r.DuplicateAttempts == 0 && r.PrematureStarts == 0
}

// DueStarts converts independently queried native due/attempt timestamps into
// observations. databaseNow is sampled from the same database after workload
// drain; driver wall time and provider acceptance/completion are not substitutes.
// Missing/unstarted due work remains an unexpected observation and fails the
// oracle, even when the successful starts have a fast p95.
func DueStarts(
	expected []DueExpectation,
	facts []JobStartFact,
	databaseNow time.Time,
	phase Phase,
) (DueReport, []Measurement, error) {
	report := DueReport{Expected: len(expected)}
	if len(expected) == 0 || databaseNow.IsZero() || !phase.valid() {
		return report, nil, fmt.Errorf(
			"%w: actual expected intents and database boundary required",
			errNativeOracle,
		)
	}
	byID := make(map[uuid.UUID]JobStartFact, len(facts))
	for _, fact := range facts {
		if _, duplicate := byID[fact.ID]; duplicate || fact.ID == uuid.Nil {
			return report, nil, fmt.Errorf(
				"%w: duplicate native operation projection",
				errNativeOracle,
			)
		}
		byID[fact.ID] = fact
	}
	seen := make(map[uuid.UUID]bool, len(expected))
	measurements := make([]Measurement, 0, len(expected))
	for _, intent := range expected {
		if seen[intent.ID] || intent.ID == uuid.Nil || intent.Event == uuid.Nil ||
			intent.Kind == "" || intent.Key == "" || intent.Due.IsZero() {
			return report, nil, fmt.Errorf(
				"%w: exact distinct approved intent identity required",
				errNativeOracle,
			)
		}
		seen[intent.ID] = true
		fact, exists := byID[intent.ID]
		if !exists || fact.EventID != intent.Event || fact.Kind != intent.Kind ||
			fact.Key != intent.Key || !fact.Due.Equal(intent.Due) || !fact.Approved || fact.Cancelled {
			return report, nil, fmt.Errorf(
				"%w: healthy expected intent missing, replaced or cancelled",
				errNativeOracle,
			)
		}
		if intent.Due.After(databaseNow) {
			if fact.FirstStarted != nil || fact.Attempts != 0 {
				return report, nil, fmt.Errorf(
					"%w: future intent already attempted",
					errNativeOracle,
				)
			}
			report.NotYetDue++
			continue
		}
		report.Due++
		sample := Measurement{
			Phase: phase, Surface: WorkerSurface, Operation: DueJobStart,
			Duration: databaseNow.Sub(intent.Due), Outcome: Unexpected,
		}
		if fact.FirstStarted == nil || fact.Attempts == 0 {
			report.Unstarted++
		} else {
			report.Started++
			sample.Status = 200
			sample.Duration = fact.FirstStarted.Sub(intent.Due)
			sample.Outcome = Accepted
			if sample.Duration < 0 || fact.FirstStarted.After(databaseNow) {
				report.PrematureStarts++
				sample.Duration = 0
				sample.Outcome = Unexpected
			}
			if fact.Attempts != 1 {
				report.DuplicateAttempts++
				sample.Outcome = Unexpected
			}
		}
		measurements = append(measurements, sample)
	}
	if len(seen) != len(byID) {
		return report, nil, fmt.Errorf(
			"%w: unreviewed extra operation in healthy intent facts",
			errNativeOracle,
		)
	}
	return report, measurements, nil
}
