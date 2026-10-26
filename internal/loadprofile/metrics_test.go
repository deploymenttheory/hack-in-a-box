package loadprofile

import (
	"testing"
	"time"
)

func TestPhaseAndOriginDistributionsCannotHideSlowPublicReads(t *testing.T) {
	var metrics Metrics
	for _, surface := range []Surface{PublicSurface, AdminSurface} {
		for range 100 {
			duration := time.Millisecond
			if surface == PublicSurface {
				duration = 600 * time.Millisecond
			}
			if err := metrics.Observe(
				Measurement{
					Phase:     SustainedPhase,
					Surface:   surface,
					Operation: HelpRead,
					Duration:  duration,
					Status:    200,
					Outcome:   Accepted,
				},
			); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := metrics.Observe(
		Measurement{
			Phase:     SaturationPhase,
			Surface:   PublicSurface,
			Operation: HelpRead,
			Duration:  10 * time.Second,
			Status:    503,
			Outcome:   Unexpected,
		},
	); err != nil {
		t.Fatal(err)
	}
	reports := metrics.Reports()
	if len(reports) != 3 {
		t.Fatal("phase/origin data merged", reports)
	}
	foundSlow, foundFailure := false, false
	for _, report := range reports {
		if report.Phase == SustainedPhase && report.Surface == PublicSurface {
			foundSlow = report.P95 == 600*time.Millisecond
		}
		if report.Phase == SaturationPhase {
			foundFailure = report.Statuses[503] == 1 && report.Outcomes[Unexpected] == 1
		}
	}
	if !foundSlow || !foundFailure {
		t.Fatal("fast origin or another phase concealed slow/error distribution", reports)
	}
	if err := metrics.Observe(
		Measurement{Operation: SessionRead, Status: 200, Outcome: Accepted},
	); err == nil {
		t.Fatal("unlabelled traffic accepted")
	}
}

func TestPercentileRetainsSlowTailAndOverflow(t *testing.T) {
	var d Distribution
	for range 94 {
		if err := d.Observe(10 * time.Millisecond); err != nil {
			t.Fatal(err)
		}
	}
	for range 6 {
		if err := d.Observe(120 * time.Second); err != nil {
			t.Fatal(err)
		}
	}
	p95, err := d.Percentile(.95)
	if err != nil || p95 != 120*time.Second || d.Count != 100 || d.Overflow != 6 {
		t.Fatal("slow tail disappeared", p95, d.Count, d.Overflow, err)
	}
	if err := d.Observe(-time.Nanosecond); err == nil || d.Count != 100 {
		t.Fatal("negative latency mutated observations")
	}
}

func TestThresholdRoundingIsConservative(t *testing.T) {
	var d Distribution
	if err := d.Observe(500*time.Millisecond + time.Nanosecond); err != nil {
		t.Fatal(err)
	}
	p95, err := d.Percentile(.95)
	if err != nil || p95 != 501*time.Millisecond {
		t.Fatal("threshold rounded down", p95, err)
	}
	if _, err := new(Distribution).Percentile(.95); err == nil {
		t.Fatal("missing samples produced percentile")
	}
}

func TestServerAndTransportFailuresCannotBeExpected(t *testing.T) {
	var metrics Metrics
	for _, status := range []int{0, 500, 503} {
		if err := metrics.Observe(
			Measurement{
				Phase: SustainedPhase, Surface: PublicSurface,
				Operation: RegistrationWrite,
				Duration:  time.Millisecond,
				Status:    status,
				Outcome:   CapacityRejection,
			},
		); err == nil {
			t.Fatal("server failure classified as capacity", status)
		}
	}
	if err := metrics.Observe(
		Measurement{
			Phase: SustainedPhase, Surface: PublicSurface,
			Operation: HelpClaim,
			Duration:  time.Millisecond,
			Status:    409,
			Outcome:   ExpectedConflict,
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := metrics.Observe(
		Measurement{
			Phase:     SustainedPhase,
			Surface:   PublicSurface,
			Operation: DueJobStart,
			Duration:  time.Second,
			Outcome:   Accepted,
		},
	); err != nil {
		t.Fatal(err)
	}
	reports := metrics.Reports()
	if len(reports) != 2 || reports[0].Outcomes[ExpectedConflict] != 1 ||
		reports[1].P95 != time.Second {
		t.Fatal(reports)
	}
	// Public report copies must not mutate measurement state.
	reports[0].Outcomes[ExpectedConflict] = 100
	if metrics.Reports()[0].Outcomes[ExpectedConflict] != 1 {
		t.Fatal("report exposed mutable metric state")
	}
}
