package loadprofile

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestConcurrentMetricsAndExactAcknowledgementReplayPreserveEveryObservation(t *testing.T) {
	var metrics Metrics
	var ledger WriteLedger
	var workers sync.WaitGroup
	commits := make([]Commit, 1000)
	for actor := range 1000 {
		identity := fmt.Sprintf("synthetic-effect-%d", actor)
		commits[actor] = Commit{
			Identity:    identity,
			Fingerprint: "synthetic-original-response",
			Effects:     1,
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 10 {
				if err := metrics.Observe(
					Measurement{
						Phase: SustainedPhase, Surface: PublicSurface,
						Operation: MentorWrite,
						Duration:  time.Millisecond,
						Status:    200,
						Outcome:   Accepted,
					},
				); err != nil {
					t.Error(err)
				}
				if err := ledger.Accept(
					AcceptedWrite{Identity: identity, Fingerprint: "synthetic-original-response"},
				); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	workers.Wait()
	reports := metrics.Reports()
	if len(reports) != 1 || reports[0].Count != 10_000 || reports[0].Outcomes[Accepted] != 10_000 {
		t.Fatal("concurrent measurements lost", reports)
	}
	if report := ledger.Reconcile(
		commits,
	); report.Accepted != 1000 || report.Committed != 1000 || report.Missing != 0 ||
		report.DuplicateEffects != 0 {
		t.Fatal("original receipt replay created or lost effects", report)
	}
}
