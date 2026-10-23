// Package reviewdashboard composes current operational support and assessment
// completeness without granting private moderation or judge authority.
package reviewdashboard

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/feedback"
	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/deploymenttheory/hack-in-a-box/internal/moderation"
	"github.com/deploymenttheory/hack-in-a-box/internal/observations"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
)

type Service struct {
	Pool       *pgxpool.Pool
	Clock      func() time.Time
	Help       help.Service
	Feedback   *feedback.Service
	Cards      *scorecards.Service
	Moderation *moderation.Service
}

func (s Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

type Page struct {
	EventID    uuid.UUID  `json:"event_id"`
	EventName  string     `json:"event_name"`
	Timezone   string     `json:"timezone"`
	Revision   int64      `json:"revision"`
	Now        time.Time  `json:"now"`
	Organiser  bool       `json:"organiser"`
	Teams      []Team     `json:"teams"`
	NextCursor *uuid.UUID `json:"next_cursor"`
}
type Team struct {
	ID             uuid.UUID                  `json:"id"`
	Name           string                     `json:"name"`
	State          string                     `json:"state"`
	Assignments    []Assignment               `json:"assignments"`
	CoverageIssues []string                   `json:"coverage_issues"`
	Output         *submissions.OutputSummary `json:"output"`
	Support        Support                    `json:"support"`
	Assessment     Assessment                 `json:"assessment"`
	Agenda         []moderation.AgendaItem    `json:"agenda"`
	MoreAgenda     bool                       `json:"more_agenda"`
}
type Assignment struct {
	PersonID uuid.UUID `json:"person_id"`
	Name     string    `json:"name"`
	Slot     string    `json:"slot"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
}
type Support struct {
	Risk         string                     `json:"risk"`
	Availability string                     `json:"availability"`
	Operational  observations.SupportStatus `json:"operational"`
	Actions      *feedback.SupportSummary   `json:"actions"`
	Help         HelpMetrics                `json:"help"`
}
type HelpMetrics struct {
	Waiting              int            `json:"waiting"`
	Claimed              int            `json:"claimed"`
	Unresolved           int            `json:"unresolved"`
	Resolved             int            `json:"resolved"`
	Cancelled            int            `json:"cancelled"`
	OverdueWaiting       int            `json:"overdue_waiting"`
	OldestWaitingSeconds *int64         `json:"oldest_waiting_seconds"`
	OldestOpenSeconds    *int64         `json:"oldest_open_seconds"`
	FirstResponse        TimingSummary  `json:"first_response"`
	Completion           TimingSummary  `json:"completion"`
	Requests             []help.Summary `json:"requests"`
	MoreRequests         bool           `json:"more_requests"`
}
type TimingSummary struct {
	Samples     int      `json:"samples"`
	MeanSeconds *float64 `json:"mean_seconds"`
}
type Assessment struct {
	Availability   string     `json:"availability"`
	OutputState    string     `json:"output_state"`
	Live           Completion `json:"live"`
	Final          Completion `json:"final"`
	CanonicalFinal bool       `json:"canonical_final"`
}
type Completion struct {
	Checkpoints       int  `json:"checkpoints"`
	RequiredReviews   int  `json:"required_reviews"`
	MissingAssignment int  `json:"missing_assignment"`
	Missing           int  `json:"missing"`
	Drafts            int  `json:"drafts"`
	Submitted         int  `json:"submitted"`
	NotOpen           int  `json:"not_open"`
	Unavailable       int  `json:"unavailable"`
	PendingCategories int  `json:"pending_categories"`
	MissingEvidence   int  `json:"missing_evidence"`
	OverdueCheckins   int  `json:"overdue_checkins"`
	Complete          bool `json:"complete"`
}
type FactualPreview struct {
	EventID     uuid.UUID                 `json:"event_id"`
	TeamID      uuid.UUID                 `json:"team_id"`
	TeamName    string                    `json:"team_name"`
	TeamState   string                    `json:"team_state"`
	GeneratedAt time.Time                 `json:"generated_at"`
	Output      submissions.OutputSummary `json:"output"`
}
