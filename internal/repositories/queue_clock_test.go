package repositories

import (
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestRepositoryImmediateIntentClockPreservesDefaultAndOriginalDue(t *testing.T) {
	pool := testdb.New(t, true)
	for _, explicit := range []bool{false, true} {
		name := "database-default"
		if explicit {
			name = "explicit-domain"
		}
		t.Run(name, func(t *testing.T) {
			f := setup(t, pool)
			team := f.team("published")
			second := f.team("published")
			clockReads := 0
			now := time.Date(2026, time.March, 20, 9, 0, 0, 123000000, time.UTC)
			if explicit {
				f.s.Now = func() time.Time { clockReads++; return now }
			}
			f.queue()
			if explicit && clockReads != 1 {
				t.Fatal("queue did not capture one trusted instant for its whole team batch")
			}
			original := f.op(team)
			repository := f.row(team)
			expected := repository.CreatedAt
			if explicit {
				expected = now
			}
			if !original.DueAt.Equal(expected) || original.State != "pending" || original.Attempts != 0 || !original.Approved {
				t.Fatal("new immediate intent lost the configured domain/default eligibility contract")
			}
			if explicit && !f.op(second).DueAt.Equal(original.DueAt) {
				t.Fatal("team batch used inconsistent trusted eligibility instants")
			}
			// Later maintenance/queue calls retain the original operation, provenance
			// and eligibility instant; advancing a clock cannot rewrite existing work.
			now = now.Add(24 * time.Hour)
			f.queue()
			if explicit && clockReads != 2 {
				t.Fatal("repeated batch did not capture exactly one trusted instant")
			}
			repeated := f.op(team)
			if repeated.ID != original.ID || !repeated.DueAt.Equal(original.DueAt) || repeated.Attempts != 0 || f.row(team).Provenance != repository.Provenance {
				t.Fatal("repeated native queue replaced original intent or its due instant")
			}
		})
	}
}
