package results

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/deploymenttheory/hack-in-a-box/internal/qualification"
	"github.com/deploymenttheory/hack-in-a-box/internal/resultpublication"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

type Service struct {
	Pool          *pgxpool.Pool
	Now           func() time.Time
	Judging       *judging.Service
	Qualification *qualification.Service
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

type Tie struct {
	ID         string      `json:"id"`
	Kind       string      `json:"kind"`
	CategoryID *uuid.UUID  `json:"category_id"`
	TeamIDs    []uuid.UUID `json:"team_ids"`
}
type VoteEvidence struct {
	TeamID        uuid.UUID  `json:"team_id"`
	DeliverableID *uuid.UUID `json:"deliverable_id"`
	Note          string     `json:"note"`
}
type Vote struct {
	TieID    string         `json:"tie_id"`
	TeamID   uuid.UUID      `json:"team_id"`
	JudgeID  uuid.UUID      `json:"judge_id"`
	Evidence []VoteEvidence `json:"evidence"`
	Reason   string         `json:"reason"`
	At       time.Time      `json:"at"`
}
type Award struct {
	PrizeID    uuid.UUID  `json:"prize_id"`
	Name       string     `json:"name"`
	CategoryID *uuid.UUID `json:"category_id"`
	TeamID     uuid.UUID  `json:"team_id"`
	Amount     string     `json:"amount"`
	Currency   string     `json:"currency"`
}
type Confirmation struct {
	ID              uuid.UUID `json:"id"`
	JudgeApprovalID uuid.UUID `json:"judge_approval_id"`
	SourceBinding   string    `json:"source_binding"`
	DecisionBinding string    `json:"decision_binding"`
	Awards          []Award   `json:"awards"`
	ActorID         uuid.UUID `json:"actor_id"`
	Reason          string    `json:"reason"`
	At              time.Time `json:"at"`
}

// Preview is private organiser preparation. It contains no raw judge cards or
// mentor rankings/notes; it is never a participant or room display projection.
type Preview struct {
	JudgeRunID       uuid.UUID                `json:"judge_run_id"`
	Reference        rubric.Reference         `json:"reference"`
	Criteria         rubric.PublicDefinition  `json:"criteria"`
	Binding          string                   `json:"binding"`
	FinalistsBinding string                   `json:"finalists_binding"`
	Selection        *qualification.Selection `json:"selection"`
	Tallies          []TeamTally              `json:"tallies"`
	Ties             []Tie                    `json:"ties"`
	Blockers         []string                 `json:"blockers"`
}
type Round struct {
	PreparedBy   uuid.UUID     `json:"prepared_by"`
	Version      int64         `json:"version"`
	Source       Preview       `json:"source"`
	Votes        []Vote        `json:"votes"`
	Confirmation *Confirmation `json:"confirmation"`
	ActorID      uuid.UUID     `json:"actor_id"`
	Action       string        `json:"action"`
	Reason       string        `json:"reason"`
	ChangedAt    time.Time     `json:"changed_at"`
}
type (
	NamedTeam      = resultpublication.NamedTeam
	Announcement   = resultpublication.Announcement
	AnnouncedAward = resultpublication.AnnouncedAward
	Page           struct {
		Advisories    []MentorAdvisory `json:"advisories"`
		TieStates     []TieState       `json:"tie_states"`
		Receipt       *Receipt         `json:"receipt"`
		Revision      int64            `json:"revision"`
		Round         Round            `json:"round"`
		Preview       Preview          `json:"preview"`
		Current       bool             `json:"current"`
		Announcements []Announcement   `json:"announcements"`
	}
)

type Command struct {
	RequestID     uuid.UUID      `json:"request_id"`
	Version       int64          `json:"version"`
	Action        string         `json:"action"`
	SourceBinding string         `json:"source_binding"`
	TieID         string         `json:"tie_id"`
	TeamID        uuid.UUID      `json:"team_id"`
	Evidence      []VoteEvidence `json:"evidence"`
	Awards        []Award        `json:"awards"`
	Reason        string         `json:"reason"`
}
type Receipt struct {
	RunID            uuid.UUID   `json:"run_id"`
	TieID            string      `json:"tie_id"`
	TeamIDs          []uuid.UUID `json:"team_ids"`
	RequestID        uuid.UUID   `json:"request_id"`
	ActorID          uuid.UUID   `json:"actor_id"`
	Action           string      `json:"action"`
	ExpectedRevision int64       `json:"expected_revision"`
	ExpectedVersion  int64       `json:"expected_version"`
	Version          int64       `json:"version"`
	Fingerprint      string      `json:"fingerprint"`
	PublicationID    *uuid.UUID  `json:"publication_id"`
}
type Recovery struct {
	Receipt *Receipt `json:"receipt"`
}
