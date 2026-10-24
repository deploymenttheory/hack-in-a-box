// Package archives composes current-authority historical reporting. It grants no
// competition write exception, score, award or provider capability.
package archives

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/surveys"
)

type Service struct {
	Outcomes   outcomes.Service
	Scorecards scorecards.Service
	Surveys    surveys.Service
	Pool       *pgxpool.Pool
	Clock      func() time.Time
}

func (s Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

// Metric values describe exact native record cohorts, never inferred performance.
// A nil denominator is inapplicable, not zero or an unknown global population.
type Metric struct {
	Key               string `json:"key"`
	Value             int64  `json:"value"`
	Unit              string `json:"unit"`
	Denominator       *int64 `json:"denominator"`
	DenominatorSource string `json:"denominator_source"`
	Source            string `json:"source"`
	Definition        string `json:"definition"`
}
type SourceAvailability struct {
	Source string `json:"source"`
	State  string `json:"state"`
	Reason string `json:"reason"`
}
type Summary struct {
	EventID       uuid.UUID            `json:"event_id"`
	EventName     string               `json:"event_name"`
	EventRevision int64                `json:"event_revision"`
	ObservedAt    time.Time            `json:"observed_at"`
	Metrics       []Metric             `json:"metrics"`
	Sources       []SourceAvailability `json:"sources"`
}
type View struct {
	EventID       uuid.UUID  `json:"event_id"`
	EventName     string     `json:"event_name"`
	EventRevision int64      `json:"event_revision"`
	CompletedAt   *time.Time `json:"completed_at"`
	ReadOnly      bool       `json:"read_only"`
	SummaryState  string     `json:"summary_state"`
	Summary       *Summary   `json:"summary"`
}
type Page struct {
	Items      []View     `json:"items"`
	NextCursor *uuid.UUID `json:"next_cursor"`
}
type ReportPage struct {
	Items      []Summary  `json:"items"`
	NextCursor *uuid.UUID `json:"next_cursor"`
	Cohort     string     `json:"cohort"`
}
