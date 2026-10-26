package results

import (
	"time"

	"github.com/google/uuid"
)

// MentorAdvisory is deliberately independent of Vote and the tally/decision
// binding. It contains only newly authored advisory views, never mentor cards.
type MentorAdvisory struct {
	TieID              string    `json:"tie_id"`
	TeamID             uuid.UUID `json:"team_id"`
	AuthorID           uuid.UUID `json:"author_id"`
	AuthorName         string    `json:"author_name"`
	Version            int64     `json:"version"`
	PreviousVersion    int64     `json:"previous_version"`
	RunID              uuid.UUID `json:"run_id"`
	SourceBinding      string    `json:"source_binding"`
	SubmissionRevision int64     `json:"submission_revision"`
	Views              string    `json:"views"`
	Reason             string    `json:"reason"`
	At                 time.Time `json:"at"`
}
type AdvisoryTarget struct {
	TieID              string          `json:"tie_id"`
	CategoryID         uuid.UUID       `json:"category_id"`
	CategoryName       string          `json:"category_name"`
	TeamID             uuid.UUID       `json:"team_id"`
	TeamName           string          `json:"team_name"`
	SubmissionRevision int64           `json:"submission_revision"`
	Version            int64           `json:"version"`
	Own                *MentorAdvisory `json:"own"`
}
type AdvisoryPage struct {
	Revision      int64            `json:"revision"`
	RunID         uuid.UUID        `json:"run_id"`
	SourceBinding string           `json:"source_binding"`
	Targets       []AdvisoryTarget `json:"targets"`
	Receipt       *AdvisoryReceipt `json:"receipt"`
}
type AdvisoryCommand struct {
	RequestID          uuid.UUID `json:"request_id"`
	Action             string    `json:"action"`
	Version            int64     `json:"version"`
	SourceBinding      string    `json:"source_binding"`
	TieID              string    `json:"tie_id"`
	TeamID             uuid.UUID `json:"team_id"`
	SubmissionRevision int64     `json:"submission_revision"`
	Views              string    `json:"views"`
	Reason             string    `json:"reason"`
}
type AdvisoryReceipt struct {
	RequestID        uuid.UUID `json:"request_id"`
	ActorID          uuid.UUID `json:"actor_id"`
	Action           string    `json:"action"`
	RunID            uuid.UUID `json:"run_id"`
	TieID            string    `json:"tie_id"`
	TeamID           uuid.UUID `json:"team_id"`
	ExpectedRevision int64     `json:"expected_revision"`
	ExpectedVersion  int64     `json:"expected_version"`
	Version          int64     `json:"version"`
	Fingerprint      string    `json:"fingerprint"`
}
type AdvisoryRecovery struct {
	Receipt *AdvisoryReceipt `json:"receipt"`
}
