package loadprofile

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSetupClockAdvanceRequiresExactPrivateForwardPhase(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	first := time.Date(2026, time.September, 1, 12, 0, 0, 123, time.UTC)
	path := filepath.Join(directory, "domain-time")
	if err := os.WriteFile(path, []byte(first.Format(time.RFC3339Nano)), 0o600); err != nil {
		t.Fatal(err)
	}
	next := first.Add(time.Hour)
	if err := AdvanceSetupClock(directory, first, next); err != nil {
		t.Fatal(err)
	}
	requireClock := func(want time.Time) {
		t.Helper()
		raw, err := ReadPrivateInput(path, 128)
		if err != nil {
			t.Fatal(err)
		}
		got, err := time.Parse(time.RFC3339Nano, string(raw))
		if err != nil || !got.Equal(want) {
			t.Fatal("clock changed unexpectedly", err)
		}
	}
	requireClock(next)
	for _, phase := range [][2]time.Time{{first, next.Add(time.Hour)}, {next, next}, {next, first}} {
		if err := AdvanceSetupClock(directory, phase[0], phase[1]); err == nil {
			t.Fatal("stale or non-forward phase accepted")
		}
		requireClock(next)
	}
	lockPath := filepath.Join(directory, ".domain-time-controller")
	if err := os.WriteFile(lockPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AdvanceSetupClock(directory, next, next.Add(time.Hour)); err == nil {
		t.Fatal("second controller accepted")
	}
	requireClock(next)
	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AdvanceSetupClock(directory, next, next.Add(time.Hour)); err == nil {
		t.Fatal("nonprivate clock accepted")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := AdvanceSetupClock(directory, next, next.Add(time.Hour)); err == nil {
		t.Fatal("nonprivate directory accepted")
	}
}
