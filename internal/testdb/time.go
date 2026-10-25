package testdb

import (
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
)

// WallTimeForInstant preserves a known fixture instant when the native command
// accepts local second-precision wall time. A repeated clock hour needs the
// explicit choice matching that instant; no production DST validation changes.
func WallTimeForInstant(t testing.TB, instant time.Time, timezone string) (string, string) {
	t.Helper()
	location, err := time.LoadLocation(timezone)
	if err != nil {
		t.Fatal(err)
	}
	instant = instant.Truncate(time.Second)
	wall := instant.In(location).Format("2006-01-02T15:04:05")
	for _, choice := range []string{"", "earlier", "later"} {
		resolved, resolveErr := schedule.ResolveLocal(wall, timezone, choice)
		if resolveErr == nil && resolved.Equal(instant) {
			return wall, choice
		}
	}
	t.Fatalf(
		"fixture wall time does not resolve to intended instant %s in %s",
		instant.Format(time.RFC3339),
		timezone,
	)
	return "", ""
}
