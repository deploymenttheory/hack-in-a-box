package app

import (
	"os"
	"testing"
)

// Only the scenario's explicit cases are supported. No production default,
// retention policy, review window or request timeout is changed by this flag.
func lifecycleRetentionDays(t *testing.T) int32 {
	t.Helper()
	switch os.Getenv("HACKBOX_LIFECYCLE_RETENTION_CASE") {
	case "", "configured120":
		return 120
	case "default90":
		return 90
	default:
		t.Fatal("choose configured120 or default90 retention scenario")
		return 0
	}
}
