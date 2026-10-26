package loadprofile

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// Synthetic native-query projections test accounting only; they do not prove
// a worker ran, a provider accepted anything or a release target was achieved.
func TestDueAccountingCannotDropUnstartedWorkFromFastSuccessfulStarts(t *testing.T) {
	now := time.Now()
	var expected []DueExpectation
	var facts []JobStartFact
	for index := range 31 {
		intent := DueExpectation{ID: uuid.New(), Event: uuid.New(), Kind: "communication.dispatch", Key: uuid.NewString(), Due: now.Add(-time.Minute)}
		fact := JobStartFact{ID: intent.ID, EventID: intent.Event, Kind: intent.Kind, Key: intent.Key, Due: intent.Due, Approved: true}
		if index < 30 {
			started := intent.Due.Add(time.Second)
			fact.FirstStarted, fact.Attempts = &started, 1
		}
		expected, facts = append(expected, intent), append(facts, fact)
	}
	report, samples, err := DueStarts(expected, facts, now, SustainedPhase)
	if err != nil || len(samples) != 31 || report.Started != 30 || report.Unstarted != 1 || report.Complete() {
		t.Fatal("unstarted native intent vanished", report, samples, err)
	}
	if samples[30].Outcome != Unexpected || samples[30].Duration != time.Minute {
		t.Fatal("missing start became fast success", samples[30])
	}
	started := expected[30].Due.Add(time.Second)
	facts[30].FirstStarted, facts[30].Attempts = &started, 1
	report, _, err = DueStarts(expected, facts, now, SustainedPhase)
	if err != nil || !report.Complete() {
		t.Fatal("exact independent start facts not reconciled", report, err)
	}
	for name, change := range map[string]func(*JobStartFact){
		"wrong-source":  func(f *JobStartFact) { f.Key = "other-owner-intent" },
		"unapproved":    func(f *JobStartFact) { f.Approved = false },
		"cancelled":     func(f *JobStartFact) { f.Cancelled = true },
		"different-due": func(f *JobStartFact) { f.Due = f.Due.Add(time.Second) },
	} {
		t.Run(name, func(t *testing.T) {
			copyOfFacts := append([]JobStartFact(nil), facts...)
			change(&copyOfFacts[0])
			if _, _, err := DueStarts(expected, copyOfFacts, now, SustainedPhase); err == nil {
				t.Fatal("healthy due cohort silently retargeted")
			}
		})
	}
	facts[0].Attempts = 2
	report, samples, err = DueStarts(expected, facts, now, SustainedPhase)
	if err != nil || report.Complete() || report.DuplicateAttempts != 1 || samples[0].Outcome != Unexpected {
		t.Fatal("healthy-provider duplicate attempt hidden", report, err)
	}
}

func TestFutureWorkCannotReplaceActualDueRuntimeSamples(t *testing.T) {
	now := time.Now()
	intent := DueExpectation{ID: uuid.New(), Event: uuid.New(), Kind: "communication.dispatch", Key: "synthetic-followup", Due: now.Add(time.Hour)}
	fact := JobStartFact{ID: intent.ID, EventID: intent.Event, Kind: intent.Kind, Key: intent.Key, Due: intent.Due, Approved: true}
	report, samples, err := DueStarts([]DueExpectation{intent}, []JobStartFact{fact}, now, SustainedPhase)
	if err != nil || report.NotYetDue != 1 || len(samples) != 0 || report.Complete() {
		t.Fatal("future scheduling was called actual due execution", report, samples, err)
	}
	started := now
	fact.FirstStarted, fact.Attempts = &started, 1
	if _, _, err := DueStarts([]DueExpectation{intent}, []JobStartFact{fact}, now, SustainedPhase); err == nil {
		t.Fatal("future scheduling hid an actual premature start")
	}
}
