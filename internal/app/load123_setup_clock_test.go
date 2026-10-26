package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad123SetupClockIsPrivateAndFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "domain-time")
	first := time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)
	if err := os.WriteFile(path, []byte(first.Format(time.RFC3339Nano)), 0o600); err != nil {
		t.Fatal(err)
	}
	clock, err := load123SetupClock(path)
	if err != nil || !clock().Equal(first) {
		t.Fatal("explicit setup clock missing", err)
	}
	second := first.AddDate(0, 0, 7)
	replacement := path + ".new"
	if err := os.WriteFile(
		replacement,
		[]byte(second.Format(time.RFC3339Nano)),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if !clock().Equal(second) {
		t.Fatal("actual owner control replacement not observed")
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := load123SetupClock(path); err == nil {
		t.Fatal("nonprivate setup control accepted")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("lost control silently fell back to wall time")
		}
	}()
	clock()
}
