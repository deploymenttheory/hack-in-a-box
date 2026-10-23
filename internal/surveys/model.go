// Package surveys owns identifiable private responses and deliberately reviewed
// aggregate disclosures. It never equates self-report with evidenced adoption.
package surveys

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/objectives"
)

const ResponseVisibility = "Responses are identifiable to current event organisers, not anonymous. Other participants cannot read your response. Wider reports contain only suppressed aggregates and explicitly reviewed summary text."

type Service struct {
	Pool *pgxpool.Pool
	Now  func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

type QuestionMeasure struct {
	QuestionID uuid.UUID                   `json:"question_id"`
	Measure    objectives.MeasureReference `json:"measure"`
	Construct  string                      `json:"construct"`
}

type Definition struct {
	Form          forms.Definition  `json:"form"`
	OpensAt       time.Time         `json:"opens_at"`
	ClosesAt      time.Time         `json:"closes_at"`
	MinimumCohort int               `json:"minimum_cohort"`
	Measures      []QuestionMeasure `json:"measures"`
}

func (d Definition) Validate() error {
	if err := forms.Validate(d.Form); err != nil {
		return err
	}
	if len(d.Form.Fields) == 0 || d.OpensAt.IsZero() || !d.ClosesAt.After(d.OpensAt) ||
		d.ClosesAt.Sub(d.OpensAt) > 90*24*time.Hour ||
		d.MinimumCohort < 5 ||
		d.MinimumCohort > 100 ||
		len(d.Measures) > len(d.Form.Fields) {
		return access.ErrInvalid
	}
	fields := map[uuid.UUID]bool{}
	for _, f := range d.Form.Fields {
		fields[f.ID] = true
	}
	seen := map[uuid.UUID]bool{}
	for _, m := range d.Measures {
		if !fields[m.QuestionID] || seen[m.QuestionID] ||
			(m.Construct != "self_reported_learning" && m.Construct != "self_reported_adoption") ||
			m.Measure.Objective.ID == uuid.Nil ||
			m.Measure.Objective.Version < 1 ||
			m.Measure.MeasureID == uuid.Nil {
			return access.ErrInvalid
		}
		seen[m.QuestionID] = true
	}
	return nil
}

type Configuration struct {
	ID          uuid.UUID  `json:"id"`
	Version     int64      `json:"version"`
	Definition  Definition `json:"definition"`
	AuthorID    uuid.UUID  `json:"author_id"`
	CreatedAt   time.Time  `json:"created_at"`
	ApprovedAt  *time.Time `json:"approved_at"`
	PublishedAt *time.Time `json:"published_at"`
	Cancelled   bool       `json:"cancelled"`
}

type ParticipantSurvey struct {
	ID                 uuid.UUID         `json:"id"`
	Version            int64             `json:"version"`
	Form               forms.Definition  `json:"form"`
	OpensAt            time.Time         `json:"opens_at"`
	ClosesAt           time.Time         `json:"closes_at"`
	ResponseVisibility string            `json:"response_visibility"`
	Measures           []QuestionMeasure `json:"measures"`
	CanRespond         bool              `json:"can_respond"`
	Response           *Response         `json:"response"`
}

type Response struct {
	Revision    int64         `json:"revision"`
	FormVersion int64         `json:"form_version"`
	Answers     forms.Answers `json:"answers"`
	SubmittedAt time.Time     `json:"submitted_at"`
}

type ResponseCommand struct {
	CommandID   uuid.UUID     `json:"command_id"`
	FormVersion int64         `json:"form_version"`
	Answers     forms.Answers `json:"answers"`
}

type Command struct {
	Action     string      `json:"action"`
	Definition *Definition `json:"definition"`
}

type Cell struct {
	Value string `json:"value"`
	Count *int   `json:"count"`
}

type QuestionAggregate struct {
	QuestionID      uuid.UUID        `json:"question_id"`
	QuestionVersion int64            `json:"question_version"`
	Label           string           `json:"label"`
	Suppressed      bool             `json:"suppressed"`
	Answered        *int             `json:"answered"`
	Unknown         *int             `json:"unknown"`
	Cells           []Cell           `json:"cells"`
	Measure         *QuestionMeasure `json:"measure"`
}

type Report struct {
	SurveyID           uuid.UUID           `json:"survey_id"`
	SurveyVersion      int64               `json:"survey_version"`
	ResponseVisibility string              `json:"response_visibility"`
	Suppressed         bool                `json:"suppressed"`
	Respondents        *int                `json:"respondents"`
	Questions          []QuestionAggregate `json:"questions"`
	ReviewedSummary    string              `json:"reviewed_summary"`
}

func validText(s string, max int) bool {
	return utf8.ValidString(s) && len(s) <= max && !strings.ContainsRune(s, 0)
}
