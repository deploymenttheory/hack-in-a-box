// Package moderation owns recorded huddles and review decisions. Independent
// scorecards, operational actions and results retain their native owners.
package moderation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
)

type Service struct {
	Pool    *pgxpool.Pool
	Clock   func() time.Time
	Cards   *scorecards.Service
	Actions ActionPolicy
}

func (s Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

// ActionState is native metadata, not action prose or a moderation permission.
// A done action is necessary but not sufficient to resolve an evidence gap.
type ActionState struct {
	ID              uuid.UUID `json:"id"`
	Version         int64     `json:"version"`
	FeedbackID      uuid.UUID `json:"feedback_id"`
	FeedbackVersion int64     `json:"feedback_version"`
	Binding         string    `json:"binding"`
	Status          string    `json:"status"`
	OwnerEligible   bool      `json:"owner_eligible"`
}
type ActionPolicy interface {
	ResolveModerationActionInTransaction(
		context.Context,
		pgx.Tx,
		uuid.UUID,
		uuid.UUID,
		uuid.UUID,
		uuid.UUID,
		int64,
	) (ActionState, error)
}
type Definition struct {
	ID                uuid.UUID `json:"id"`
	CheckpointID      uuid.UUID `json:"checkpoint_id"`
	CheckpointVersion int64     `json:"checkpoint_version"`
	ModeratorID       uuid.UUID `json:"moderator_id"`
	SecondReviewerID  uuid.UUID `json:"second_reviewer_id"`
	ReviewDueAt       time.Time `json:"review_due_at"`
}
type SecondReview struct {
	ReviewerID  uuid.UUID                 `json:"reviewer_id"`
	DueAt       time.Time                 `json:"due_at"`
	Card        *scorecards.CardReference `json:"card"`
	CompletedAt *time.Time                `json:"completed_at"`
}
type Gap struct {
	ID            uuid.UUID                      `json:"id"`
	ActionID      uuid.UUID                      `json:"action_id"`
	ActionVersion int64                          `json:"action_version"`
	Reason        string                         `json:"reason"`
	ResolvedBy    *uuid.UUID                     `json:"resolved_by"`
	ResolvedAt    *time.Time                     `json:"resolved_at"`
	Resolution    string                         `json:"resolution"`
	Action        *ActionState                   `json:"action"`
	Evidence      []scorecards.EvidenceReference `json:"evidence"`
}
type Proposal struct {
	ID                 uuid.UUID                      `json:"id"`
	Card               scorecards.CardReference       `json:"card"`
	OriginalReviewerID uuid.UUID                      `json:"original_reviewer_id"`
	Original           scorecards.CategoryScore       `json:"original"`
	ProposedValue      int                            `json:"proposed_value"`
	Evidence           []scorecards.EvidenceReference `json:"evidence"`
	ProposedBy         uuid.UUID                      `json:"proposed_by"`
	ProposedAt         time.Time                      `json:"proposed_at"`
	Reason             string                         `json:"reason"`
	State              string                         `json:"state"`
	DecidedBy          *uuid.UUID                     `json:"decided_by"`
	DecidedAt          *time.Time                     `json:"decided_at"`
	DecisionReason     string                         `json:"decision_reason"`
}
type Huddle struct {
	Definition         Definition                    `json:"definition"`
	TeamID             uuid.UUID                     `json:"team_id"`
	Version            int64                         `json:"version"`
	State              string                        `json:"state"`
	Checkpoint         scorecards.Checkpoint         `json:"checkpoint"`
	PrimaryID          uuid.UUID                     `json:"primary_id"`
	PanelIDs           []uuid.UUID                   `json:"panel_ids"`
	Review             SecondReview                  `json:"second_review"`
	Cards              []scorecards.CardReference    `json:"cards"`
	Gaps               []Gap                         `json:"gaps"`
	Proposals          []Proposal                    `json:"proposals"`
	SubmissionRevision *int64                        `json:"submission_revision"`
	ActorID            uuid.UUID                     `json:"actor_id"`
	Action             string                        `json:"action"`
	Reason             string                        `json:"reason"`
	ChangedAt          time.Time                     `json:"changed_at"`
	SignoffID          *uuid.UUID                    `json:"signoff_id"`
	SignoffContext     *scorecards.ModerationContext `json:"signoff_context"`
}
type Command struct {
	Version       int64                          `json:"version"`
	Action        string                         `json:"action"`
	Definition    *Definition                    `json:"definition"`
	Reason        string                         `json:"reason"`
	GapID         uuid.UUID                      `json:"gap_id"`
	ActionID      uuid.UUID                      `json:"action_id"`
	ActionVersion int64                          `json:"action_version"`
	Evidence      []scorecards.EvidenceReference `json:"evidence"`
	ProposalID    uuid.UUID                      `json:"proposal_id"`
	Card          scorecards.CardReference       `json:"card"`
	CategoryID    uuid.UUID                      `json:"category_id"`
	ProposedValue *int                           `json:"proposed_value"`
}
type Page struct {
	Revision   int64      `json:"revision"`
	Items      []Huddle   `json:"items"`
	NextCursor *uuid.UUID `json:"next_cursor"`
}
type History struct {
	Items      []Huddle `json:"items"`
	NextCursor *int64   `json:"next_cursor"`
}

func binding(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
