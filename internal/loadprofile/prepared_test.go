package loadprofile

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
)

func preparedSmokeFixture() PreparedFixture {
	registrationEvent, liveEvent, form := uuid.New(), uuid.New(), uuid.New()
	return PreparedFixture{
		Run: uuid.New(),
		Cohort: CohortPlan{
			Profile: "smoke",
			Members: []CohortMember{{Subject: "known", Event: liveEvent, Surface: PublicSurface}},
		},
		Origins: map[Surface]string{
			PublicSurface: "https://public.load.test:8443",
			AdminSurface:  "https://admin.load.test:8444",
		},
		Issuer: "https://issuer.load.test:8445",
		Events: []PreparedEvent{
			{ID: registrationEvent, Stage: "registration"},
			{ID: liveEvent, Stage: "live"},
		},
		Journeys: map[string]Journey{
			"known": {Read: ReadBinding{Event: liveEvent, Operations: []Operation{SessionRead}}},
		},
		Burst: PreparedBurst{
			Config: BurstConfig{
				Profile:     "smoke",
				Event:       registrationEvent,
				Form:        form,
				FormVersion: 1,
			},
			Applications: map[string]registration.Application{
				"known": {ID: uuid.New(), FormID: form, FormVersion: 1},
			},
		},
	}
}

func TestPreparedFixtureKeepsNativeScopeAndExactKnownCohort(t *testing.T) {
	valid := preparedSmokeFixture()
	raw, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParsePreparedFixture(raw)
	if err != nil {
		t.Fatal(err)
	}
	targets, err := parsed.FixtureTargets()
	if err != nil || len(targets) != 3 || targets[0] != "public.load.test:8443" {
		t.Fatal("exact front-door targets missing", err, targets)
	}
	for name, mutate := range map[string]func(*PreparedFixture){
		"unknown burst actor": func(f *PreparedFixture) {
			f.Burst.Applications["not-in-cohort"] = f.Burst.Applications["known"]
		},
		"retargeted home event": func(f *PreparedFixture) {
			journey := f.Journeys["known"]
			journey.Read.Event = f.Events[0].ID
			f.Journeys["known"] = journey
		},
		"private capability outside scope": func(f *PreparedFixture) {
			f.Displays = []PreparedDisplay{{ID: "display", Event: uuid.New(), Token: "private"}}
		},
		"invented due intent outside scope": func(f *PreparedFixture) {
			f.Due = []DueExpectation{{ID: uuid.New(), Event: uuid.New()}}
		},
		"same portal process":       func(f *PreparedFixture) { f.Origins[AdminSurface] = f.Origins[PublicSurface] },
		"smoke promoted to release": func(f *PreparedFixture) { f.Cohort.Profile = "release" },
	} {
		t.Run(name, func(t *testing.T) {
			fixture := preparedSmokeFixture()
			mutate(&fixture)
			if fixture.Validate() == nil {
				t.Fatal("invalid prepared source accepted")
			}
		})
	}
	for _, raw := range [][]byte{append(raw, []byte("{}")...), []byte(`{"run":"unexpected","grant":true}`)} {
		if _, err := ParsePreparedFixture(raw); err == nil {
			t.Fatal("ambiguous fixture JSON accepted")
		}
	}
}
