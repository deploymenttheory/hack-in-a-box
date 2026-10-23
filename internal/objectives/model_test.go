package objectives

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestPlanningValuesKeepUnknownSeparateFromExplicitZero(t *testing.T) {
	zero := "0"
	d := Definition{
		Description: "Improve onboarding",
		Measures:    []Measure{{ID: uuid.New(), Description: "Elapsed time", Target: &zero}},
	}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"baseline":null`) ||
		!strings.Contains(string(raw), `"target":"0"`) {
		t.Fatalf("unknown/zero conflated: %s", raw)
	}
	if len(d.PlanningGaps()) == 0 {
		t.Fatal("missing planning input not reported")
	}
	d.Measures[0].Baseline = &Baseline{Value: "0", Basis: "self_report"}
	if err = d.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, gap := range d.PlanningGaps() {
		if gap == "baseline_unknown" {
			t.Fatal("explicit self-report baseline treated as unknown")
		}
	}
}

func TestPlanningDefinitionsCannotClaimDemonstratedResults(t *testing.T) {
	d := Definition{
		Description: "Improve onboarding",
		Measures: []Measure{
			{
				ID:          uuid.New(),
				Description: "Elapsed time",
				Baseline:    &Baseline{Value: "12.5", Basis: "estimate"},
			},
		},
	}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	d.Measures[0].Baseline.Basis = "demonstrated"
	if d.Validate() == nil {
		t.Fatal(
			"configuration falsely accepted demonstrated result without native outcome evidence",
		)
	}
	for _, value := range []string{"NaN", "Infinity", "1e6", "01", " 0", "0 ", "", "9999999999999999999", "1.1234567890123"} {
		d.Measures[0].Baseline = nil
		d.Measures[0].Target = &value
		if d.Validate() == nil {
			t.Fatalf("accepted ambiguous/unbounded decimal %q", value)
		}
	}
}

func TestMeasureIdentityAndPrivatePlanningTextValidation(t *testing.T) {
	id := uuid.New()
	d := Definition{
		Description:    "Private challenge objective",
		ThemeSensitive: true,
		Measures:       []Measure{{ID: id, Description: "Measure"}},
	}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	d.Measures = append(d.Measures, d.Measures[0])
	if d.Validate() == nil {
		t.Fatal("duplicate stable measure identity")
	}
	d.Measures = d.Measures[:1]
	d.Description = "bad\x00text"
	if d.Validate() == nil {
		t.Fatal("NUL text")
	}
}
