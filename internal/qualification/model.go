// Package qualification owns moderated final mentor cards and private panel
// selection. Independent cards, frozen submissions and judge scoring retain
// their native owners; this module neither confirms nor publishes awards.
package qualification

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/moderation"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissionfreeze"
)

type Service struct {
	Pool       *pgxpool.Pool
	Now        func() time.Time
	Moderation *moderation.Service
	Cards      *scorecards.Service
	Freeze     *submissionfreeze.Service
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

type Panel struct {
	MentorIDs   []uuid.UUID `json:"mentor_ids"`
	OrganiserID uuid.UUID   `json:"organiser_id"`
}
type FinalSource struct {
	FreezeBinding        string           `json:"freeze_binding"`
	FreezeStatus         string           `json:"freeze_status"`
	RepositoryDecisionID *uuid.UUID       `json:"repository_decision_id"`
	HuddleID             uuid.UUID        `json:"huddle_id"`
	HuddleVersion        int64            `json:"huddle_version"`
	SignoffID            uuid.UUID        `json:"signoff_id"`
	SubmissionRevision   int64            `json:"submission_revision"`
	Rubric               rubric.Reference `json:"rubric"`
	Binding              string           `json:"binding"`
}
type FinalCard struct {
	TeamID    uuid.UUID                  `json:"team_id"`
	Version   int64                      `json:"version"`
	State     string                     `json:"state"`
	Source    FinalSource                `json:"source"`
	Scores    []scorecards.CategoryScore `json:"scores"`
	ActorID   uuid.UUID                  `json:"actor_id"`
	Reason    string                     `json:"reason"`
	ChangedAt time.Time                  `json:"changed_at"`
}
type Eligibility struct {
	TeamID   uuid.UUID                      `json:"team_id"`
	Binding  string                         `json:"binding"`
	Decision string                         `json:"decision"`
	Reason   string                         `json:"reason"`
	Evidence []scorecards.EvidenceReference `json:"evidence"`
	ActorID  uuid.UUID                      `json:"actor_id"`
	At       time.Time                      `json:"at"`
}
type TeamStatus struct {
	TeamID        uuid.UUID `json:"team_id"`
	EntryID       uuid.UUID `json:"entry_id"`
	Outcome       string    `json:"outcome"`
	FreezeStatus  string    `json:"freeze_status"`
	FreezeBinding string    `json:"freeze_binding"`
	Eligible      bool      `json:"eligible"`
	Blockers      []string  `json:"blockers"`
}
type Ranking struct {
	TeamID uuid.UUID `json:"team_id"`
	Total  int64     `json:"total"`
	Rank   int       `json:"rank"`
}
type CategoryProposal struct {
	CategoryID uuid.UUID   `json:"category_id"`
	TeamIDs    []uuid.UUID `json:"team_ids"`
}
type Tie struct {
	ID         string      `json:"id"`
	Kind       string      `json:"kind"`
	CategoryID *uuid.UUID  `json:"category_id"`
	TeamIDs    []uuid.UUID `json:"team_ids"`
	Seats      int         `json:"seats"`
}
type Vote struct {
	TieID    string                         `json:"tie_id"`
	Seat     int                            `json:"seat"`
	TeamID   uuid.UUID                      `json:"team_id"`
	ActorID  uuid.UUID                      `json:"actor_id"`
	Reason   string                         `json:"reason"`
	Evidence []scorecards.EvidenceReference `json:"evidence"`
	At       time.Time                      `json:"at"`
}
type TieDecision struct {
	TieID    string                         `json:"tie_id"`
	Seat     int                            `json:"seat"`
	TeamID   uuid.UUID                      `json:"team_id"`
	ActorID  uuid.UUID                      `json:"actor_id"`
	Casting  bool                           `json:"casting"`
	Reason   string                         `json:"reason"`
	Evidence []scorecards.EvidenceReference `json:"evidence"`
	At       time.Time                      `json:"at"`
}
type Proposal struct {
	Binding     string             `json:"binding"`
	Rankings    []Ranking          `json:"rankings"`
	FinalistIDs []uuid.UUID        `json:"finalist_ids"`
	ReserveIDs  []uuid.UUID        `json:"reserve_ids"`
	Categories  []CategoryProposal `json:"categories"`
	Ties        []Tie              `json:"ties"`
	Blockers    []string           `json:"blockers"`
}
type Signoff struct {
	ActorID uuid.UUID `json:"actor_id"`
	At      time.Time `json:"at"`
	Binding string    `json:"binding"`
	Reason  string    `json:"reason"`
}
type Round struct {
	RunID       uuid.UUID     `json:"run_id"`
	Version     int64         `json:"version"`
	Panel       Panel         `json:"panel"`
	Cards       []FinalCard   `json:"cards"`
	Eligibility []Eligibility `json:"eligibility"`
	Proposal    *Proposal     `json:"proposal"`
	Votes       []Vote        `json:"votes"`
	Decisions   []TieDecision `json:"decisions"`
	Signoff     *Signoff      `json:"signoff"`
	ActorID     uuid.UUID     `json:"actor_id"`
	Action      string        `json:"action"`
	Reason      string        `json:"reason"`
	ChangedAt   time.Time     `json:"changed_at"`
}
type Page struct {
	Receipt  *Receipt                `json:"receipt"`
	Preview  *Proposal               `json:"preview"`
	Revision int64                   `json:"revision"`
	Round    Round                   `json:"round"`
	Criteria rubric.PublicDefinition `json:"criteria"`
	Teams    []TeamStatus            `json:"teams"`
	Current  bool                    `json:"current"`
}
type History struct {
	Items      []Round `json:"items"`
	NextCursor *int64  `json:"next_cursor"`
}
type CardInput struct {
	TeamID  uuid.UUID                  `json:"team_id"`
	Version int64                      `json:"version"`
	Scores  []scorecards.CategoryScore `json:"scores"`
}
type Command struct {
	RequestID            uuid.UUID                      `json:"request_id"`
	FreezeBinding        string                         `json:"freeze_binding"`
	RepositoryDecisionID *uuid.UUID                     `json:"repository_decision_id"`
	Version              int64                          `json:"version"`
	Action               string                         `json:"action"`
	Reason               string                         `json:"reason"`
	Panel                *Panel                         `json:"panel"`
	Card                 *CardInput                     `json:"card"`
	TeamID               uuid.UUID                      `json:"team_id"`
	CardVersion          int64                          `json:"card_version"`
	Decision             string                         `json:"decision"`
	Evidence             []scorecards.EvidenceReference `json:"evidence"`
	TieID                string                         `json:"tie_id"`
	Seat                 int                            `json:"seat"`
}

func digest(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func failure(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("qualification %s: %w", operation, err)
}

type Receipt struct {
	RequestID        uuid.UUID `json:"request_id"`
	RunID            uuid.UUID `json:"run_id"`
	Action           string    `json:"action"`
	ExpectedRevision int64     `json:"expected_revision"`
	ExpectedVersion  int64     `json:"expected_version"`
	Version          int64     `json:"version"`
	Fingerprint      string    `json:"fingerprint"`
}
type Recovery struct {
	Receipt *Receipt `json:"receipt"`
}
