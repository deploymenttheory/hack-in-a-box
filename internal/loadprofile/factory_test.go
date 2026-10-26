package loadprofile

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

func TestReleaseFactoryDeclaresExactCohortWithoutCreatingNativeFacts(t *testing.T) {
	run := uuid.New()
	anchor := time.Date(2026, time.October, 26, 12, 0, 0, 0, time.UTC)
	plan, err := NewReleaseFactoryPlan(run, anchor)
	if err != nil || len(plan.Events) != 5 || len(plan.Cohort.Members) != 1000 {
		t.Fatal("full requested fixture shape lost", err)
	}
	roles := make(map[string]int)
	bootstrap := 0
	for _, member := range plan.Cohort.Members {
		if member.Subject == plan.BootstrapSubject {
			bootstrap++
			if !member.Staff || member.Surface != AdminSurface {
				t.Fatal("bootstrap was not inside the ordinary staff cohort")
			}
		}
	}
	for _, item := range plan.Events {
		if issues := event.Validate(item.Input, true); len(issues) != 0 {
			t.Fatal("native event configuration would be rejected", issues)
		}
		if len(item.Staff) != 20 || item.Input.Configuration.Capacity != 180 ||
			!item.Input.Configuration.AdmissionCutoff.Before(item.Input.Configuration.TeamFreeze) {
			t.Fatal("staff/competitor/cutoff census inconsistent")
		}
		for _, person := range item.Staff {
			roles[person.Role]++
		}
	}
	if bootstrap != 1 || roles["organiser"] != 10 || roles["mentor"] != 40 ||
		roles["judge"] != 25 ||
		roles["mc"] != 10 ||
		roles["logistics"] != 15 {
		t.Fatal("cohort added a dormant operator or lost required roles", bootstrap, roles)
	}
	manifest, err := OIDCManifestForCohort(
		plan.Cohort,
		"https://issuer.load.test:8445",
		map[string]string{
			"load-public": "https://public.load.test:8443/auth/callback",
			"load-admin":  "https://admin.load.test:8444/auth/callback",
		},
	)
	if err != nil || len(manifest.People) != 1000 {
		t.Fatal("exact factory actors did not reach explicit external issuer", err)
	}
	if _, err := NewReleaseFactoryPlan(uuid.Nil, anchor); err == nil {
		t.Fatal("missing run identity accepted")
	}
}
