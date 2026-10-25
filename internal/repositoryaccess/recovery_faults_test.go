package repositoryaccess

import (
	"errors"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/google/uuid"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestRecoveryUnsentInvitationRequiresNewReviewAndNeverBlindlyReplays(t *testing.T) {
	pool := testdb.New(t, true)
	f := setupAccess(t, pool)
	f.publish()
	plan := f.review()
	f.approve(plan)
	f.provider.fault = "unsent"
	f.execute(plan.Actions[0])
	state := f.view()
	if len(state.Grants) != 1 || state.Grants[0].Uncertain || state.Grants[0].State != "failed" || state.Grants[0].Diagnostic != "provider_unavailable_before_write" || len(f.provider.snapshot.Invitations) != 0 {
		t.Fatal("definitely unsent invitation became accepted or ambiguous", state)
	}
	attempts := f.provider.writes
	f.provider.fault = ""
	f.execute(plan.Actions[0])
	if f.provider.writes != attempts {
		t.Fatal("old failed action blindly replayed")
	}
	replacement := f.review()
	if len(replacement.Actions) != 1 {
		t.Fatal(replacement)
	}
	f.approve(replacement)
	f.execute(replacement.Actions[0])
	f.execute(replacement.Actions[0])
	if f.provider.writes != attempts+1 || len(f.provider.snapshot.Invitations) != 1 {
		t.Fatal("reviewed recovery duplicated invitation")
	}
}

func TestRecoveryQueuedCollaboratorRechecksCurrentFreezeAndRetirement(t *testing.T) {
	pool := testdb.New(t, true)
	for _, boundary := range []string{"unfrozen_team", "disbanded_team", "withdrawn_member"} {
		t.Run(boundary, func(t *testing.T) {
			f := setupAccess(t, pool)
			f.publish()
			plan := f.review()
			f.approve(plan)
			// These are fault-injected current facts after a valid reviewed queue. The
			// worker must not treat an old approval as permanent eligibility.
			switch boundary {
			case "unfrozen_team":
				f.exec("UPDATE hack_teams SET frozen=false WHERE event_id=$1 AND id=$2", f.event, f.team)
			case "disbanded_team":
				f.exec("UPDATE hack_teams SET state='disbanded' WHERE event_id=$1 AND id=$2", f.event, f.team)
				f.exec("UPDATE competition_entries SET active=false WHERE event_id=$1 AND team_id=$2", f.event, f.team)
			case "withdrawn_member":
				f.exec("UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2", f.event, f.member)
			}
			var operation uuid.UUID
			if err := pool.QueryRow(t.Context(), "SELECT id FROM operations WHERE event_id=$1 AND target_id=$2", f.event, plan.Actions[0].ID).Scan(&operation); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if err := f.jobs.Execute(t.Context(), operation); err != nil && !(boundary == "disbanded_team" && errors.Is(err, jobs.ErrEligibility)) {
					t.Fatal(err)
				}
			}
			if f.provider.writes != 0 || len(f.provider.snapshot.Invitations) != 0 {
				t.Fatal("stale queued approval crossed current native boundary")
			}
			var attempted bool
			if err := pool.QueryRow(t.Context(), "SELECT attempted FROM repository_access_effects WHERE event_id=$1 AND id=$2", f.event, plan.Actions[0].ID).Scan(&attempted); err != nil || attempted {
				t.Fatal("ineligible work acquired an external-effect claim", attempted, err)
			}
		})
	}
}
