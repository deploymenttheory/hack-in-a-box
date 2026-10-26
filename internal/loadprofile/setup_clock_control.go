package loadprofile

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AdvanceSetupClock is a private, single-controller fixture operation. It only
// replaces the already-private domain-time file; it never contacts the app or
// changes production clocks. The expected value prevents an old phase command
// from silently advancing a newer preparation run.
func AdvanceSetupClock(directory string, expected, next time.Time) error {
	if !filepath.IsAbs(directory) || expected.IsZero() || next.IsZero() || !next.After(expected) {
		return fmt.Errorf(
			"%w: absolute private directory and strictly increasing expected clock required",
			errCohort,
		)
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w: private existing setup directory required", errCohort)
	}
	lockPath := filepath.Join(directory, ".domain-time-controller")
	lock, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("acquire private setup clock controller: %w", err)
	}
	defer func() { _ = lock.Close(); _ = os.Remove(lockPath) }()
	path := filepath.Join(directory, "domain-time")
	raw, err := ReadPrivateInput(path, 128)
	if err != nil {
		return fmt.Errorf("read current private setup clock: %w", err)
	}
	current, err := time.Parse(time.RFC3339Nano, string(bytes.TrimSpace(raw)))
	if err != nil || !current.Equal(expected) {
		return fmt.Errorf("%w: setup clock no longer matches the expected phase", errCohort)
	}
	replacement, err := os.CreateTemp(directory, ".domain-time-next-")
	if err != nil {
		return fmt.Errorf("reserve atomic setup clock replacement: %w", err)
	}
	name := replacement.Name()
	defer func() { _ = replacement.Close(); _ = os.Remove(name) }()
	if _, err := replacement.WriteString(next.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("write next setup phase: %w", err)
	}
	if err := replacement.Sync(); err != nil {
		return fmt.Errorf("sync next setup phase: %w", err)
	}
	if err := replacement.Close(); err != nil {
		return fmt.Errorf("close next setup phase: %w", err)
	}
	// Rename within the private volume prevents readers observing a partial time.
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("publish next setup phase: %w", err)
	}
	return nil
}
