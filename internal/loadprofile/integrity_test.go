package loadprofile

import (
	"fmt"
	"testing"
	"time"
)

func TestLostChangedAndDuplicatedAcceptedEffectsFailIndependentReconciliation(t *testing.T) {
	var ledger WriteLedger
	for _, id := range []string{"committed", "lost", "changed", "duplicated"} {
		if err := ledger.Accept(AcceptedWrite{id, "original"}); err != nil {
			t.Fatal(err)
		}
	}
	// Exact ACK replay is one native effect, not a second accepted mutation.
	if err := ledger.Accept(AcceptedWrite{"committed", "original"}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Accept(AcceptedWrite{"committed", "later-current-state"}); err == nil {
		t.Fatal("current read overwrote original ACK")
	}
	report := ledger.Reconcile(
		[]Commit{
			{"committed", "original", 1},
			{"changed", "different", 1},
			{"duplicated", "original", 1},
			{"duplicated", "original", 1},
		},
	)
	if report != (IntegrityReport{Accepted: 4, Committed: 3, Missing: 1, Changed: 1, DuplicateEffects: 1}) {
		t.Fatal(report)
	}
}

func TestOneThousandSeededOrInitiallyActiveAccountsAreNotSustainedLoad(t *testing.T) {
	actors := make([]string, 1000)
	for index := range actors {
		actors[index] = fmt.Sprintf("private-actor-%d", index)
	}
	start := time.Now()
	activity, err := NewActivity(start, 30*time.Minute, actors)
	if err != nil {
		t.Fatal(err)
	}
	for _, actor := range actors {
		if err := activity.Authenticated(actor, start); err != nil {
			t.Fatal(err)
		}
	}
	report := activity.Report()
	if report.ActiveEveryMinute != 0 || report.MissingActorMinutes != 29000 {
		t.Fatal("sign-in census became sustained load", report)
	}
	for minute := 1; minute < 30; minute++ {
		for _, actor := range actors {
			if err := activity.Authenticated(
				actor,
				start.Add(time.Duration(minute)*time.Minute),
			); err != nil {
				t.Fatal(err)
			}
		}
	}
	report = activity.Report()
	if report.ActiveEveryMinute != 1000 || report.MissingActorMinutes != 0 {
		t.Fatal(report)
	}
	if err := activity.Authenticated("replacement", start); err == nil {
		t.Fatal("replacement actor filled activity")
	}
	if err := activity.Authenticated(actors[0], start.Add(30*time.Minute)); err == nil {
		t.Fatal("outside-stage request filled activity")
	}
}
