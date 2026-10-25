package testdb

import (
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
)

func TestFixtureWallTimePreservesOrdinaryAndBothRepeatedHourInstants(t *testing.T) {
	for _, tc := range []struct {
		name, instant, wall, choice string
	}{
		{"ordinary", "2026-02-02T09:00:00Z", "2026-02-02T09:00:00", ""},
		{"earlier_fold", "2026-10-25T00:30:00Z", "2026-10-25T01:30:00", "earlier"},
		{"later_fold", "2026-10-25T01:30:00Z", "2026-10-25T01:30:00", "later"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			instant, err := time.Parse(time.RFC3339, tc.instant)
			if err != nil {
				t.Fatal(err)
			}
			wall, choice := WallTimeForInstant(t, instant, "Europe/London")
			if wall != tc.wall || choice != tc.choice {
				t.Fatalf("got %q/%q; want %q/%q", wall, choice, tc.wall, tc.choice)
			}
			if tc.choice != "" {
				if _, err = schedule.ResolveLocal(wall, "Europe/London", ""); err == nil {
					t.Fatal(
						"native validation accepted an ambiguous wall time without explicit choice",
					)
				}
			}
			resolved, err := schedule.ResolveLocal(wall, "Europe/London", choice)
			if err != nil || !resolved.Equal(instant) {
				t.Fatalf("fixture changed intended instant: got %s, error %v", resolved, err)
			}
		})
	}
}
