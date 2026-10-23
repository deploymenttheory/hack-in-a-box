// Package objectives owns versioned planning definitions. It does not record
// achieved outcomes, employee activity, assessments or financial returns.
package objectives

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
)

// Reference is the stable immutable definition identity retained by reporting
// observations. Later edits and retirement must never retarget an old reference.
type Reference struct {
	ID      uuid.UUID `json:"id"`
	Version int64     `json:"version"`
}

// MeasureReference binds a report to the precise measure definition it used.
// A measure ID is stable within revisions, while Version identifies the complete
// objective snapshot, including unit, target and measurement instructions.
type MeasureReference struct {
	Objective Reference `json:"objective"`
	MeasureID uuid.UUID `json:"measure_id"`
}

// Baseline is an organiser's planning input, not a verified achieved result.
// A nil baseline means unknown. Zero is an explicit decimal value. EvidenceSource
// describes where a later review should obtain evidence; it is not itself proof.
type Baseline struct {
	Value string `json:"value"`
	Basis string `json:"basis"`
}

type Measure struct {
	ID             uuid.UUID `json:"id"`
	Description    string    `json:"description"`
	Baseline       *Baseline `json:"baseline"`
	Target         *string   `json:"target"`
	Unit           string    `json:"unit"`
	Method         string    `json:"method"`
	EvidenceSource string    `json:"evidence_source"`
	ReviewHorizon  string    `json:"review_horizon"`
}

type Definition struct {
	Description    string     `json:"description"`
	OwnerID        *uuid.UUID `json:"owner_id"`
	ThemeSensitive bool       `json:"theme_sensitive"`
	Measures       []Measure  `json:"measures"`
}

// Configuration is copied by value into a template. Native versions, disclosures,
// observations, evidence and follow-up history deliberately have no fields here.
type Configuration struct {
	ID         uuid.UUID  `json:"id"`
	Definition Definition `json:"definition"`
}

var decimal = regexp.MustCompile(`^-?(?:0|[1-9][0-9]{0,17})(?:\.[0-9]{1,12})?$`)

func validText(value string, max int) bool {
	return len(value) <= max && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

func (d Definition) Validate() error {
	if strings.TrimSpace(d.Description) == "" || !validText(d.Description, 10000) ||
		len(d.Measures) > 50 ||
		(d.OwnerID != nil && *d.OwnerID == uuid.Nil) {
		return access.ErrInvalid
	}
	seen := map[uuid.UUID]bool{}
	for _, m := range d.Measures {
		if m.ID == uuid.Nil || seen[m.ID] || strings.TrimSpace(m.Description) == "" ||
			!validText(
				m.Description,
				4000,
			) || !validText(m.Unit, 160) || !validText(m.Method, 4000) ||
			!validText(m.EvidenceSource, 4000) || !validText(m.ReviewHorizon, 1000) {
			return access.ErrInvalid
		}
		seen[m.ID] = true
		if m.Target != nil && !decimal.MatchString(*m.Target) {
			return access.ErrInvalid
		}
		if m.Baseline != nil {
			if !decimal.MatchString(m.Baseline.Value) ||
				(m.Baseline.Basis != "estimate" && m.Baseline.Basis != "self_report") {
				return access.ErrInvalid
			}
		}
	}
	return nil
}

// PlanningGaps are advisory setup hints, never publication or attendance gates.
func (d Definition) PlanningGaps() []string {
	gaps := []string{}
	if d.OwnerID == nil {
		gaps = append(gaps, "owner_missing")
	}
	for _, m := range d.Measures {
		if m.Baseline == nil {
			gaps = append(gaps, "baseline_unknown")
		}
		if m.Target == nil {
			gaps = append(gaps, "target_missing")
		}
		if strings.TrimSpace(m.Unit) == "" {
			gaps = append(gaps, "unit_missing")
		}
		if strings.TrimSpace(m.Method) == "" {
			gaps = append(gaps, "method_missing")
		}
		if strings.TrimSpace(m.EvidenceSource) == "" {
			gaps = append(gaps, "evidence_source_missing")
		}
		if strings.TrimSpace(m.ReviewHorizon) == "" {
			gaps = append(gaps, "review_horizon_missing")
		}
	}
	return gaps
}
