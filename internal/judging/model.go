// Package judging owns independent final judge cards. Qualification, award
// calculation, announcement and mentor raw records remain with their owners.
package judging

import (
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissionfreeze"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Service struct {
	Pool      *pgxpool.Pool
	Now       func() time.Time
	Finalists rubric.FinalistPolicy
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

type Score struct {
	CategoryID uuid.UUID  `json:"category_id"`
	Value      *int       `json:"value"`
	Evidence   []Evidence `json:"evidence"`
}

// Judge evidence refers only to currently visible frozen deliverable IDs or an
// explicit judge-authored note. It never references private mentor observations.
type Evidence struct {
	DeliverableID *uuid.UUID `json:"deliverable_id"`
	Note          string     `json:"note"`
}
type Card struct {
	RunID              uuid.UUID        `json:"run_id"`
	TeamID             uuid.UUID        `json:"team_id"`
	AuthorID           uuid.UUID        `json:"author_id"`
	Version            int64            `json:"version"`
	State              string           `json:"state"`
	SubmissionRevision int64            `json:"submission_revision"`
	Rubric             rubric.Reference `json:"rubric"`
	Scores             []Score          `json:"scores"`
	ChangedAt          time.Time        `json:"changed_at"`
}
type Assignment struct {
	TeamID   uuid.UUID   `json:"team_id"`
	JudgeIDs []uuid.UUID `json:"judge_ids"`
	Missing  int         `json:"missing"`
}
type Round struct {
	RunID          uuid.UUID    `json:"run_id"`
	Revision       int64        `json:"revision"`
	Deliberation   bool         `json:"deliberation"`
	RequiredJudges int          `json:"required_judges"`
	Assignments    []Assignment `json:"assignments"`
}
type RoundCommand struct {
	RequestID    uuid.UUID   `json:"request_id"`
	Action       string      `json:"action"`
	TeamID       uuid.UUID   `json:"team_id"`
	JudgeIDs     []uuid.UUID `json:"judge_ids"`
	JudgeID      uuid.UUID   `json:"judge_id"`
	CompetitorID uuid.UUID   `json:"competitor_id"`
	ConflictKind string      `json:"conflict_kind"`
	Enabled      bool        `json:"enabled"`
	Reason       string      `json:"reason"`
}
type CardCommand struct {
	RequestID          uuid.UUID `json:"request_id"`
	Action             string    `json:"action"`
	AuthorID           uuid.UUID `json:"author_id"`
	SubmissionRevision int64     `json:"submission_revision"`
	Scores             []Score   `json:"scores"`
	Reason             string    `json:"reason"`
}
type Brief struct {
	TeamID               uuid.UUID                 `json:"team_id"`
	TeamName             string                    `json:"team_name"`
	RunID                uuid.UUID                 `json:"run_id"`
	Criteria             rubric.PublicDefinition   `json:"criteria"`
	Reference            rubric.Reference          `json:"reference"`
	PrivateJudgeGuidance string                    `json:"private_judge_guidance"`
	Submission           submissions.Document      `json:"submission"`
	SubmissionRevision   int64                     `json:"submission_revision"`
	Output               submissions.OutputSummary `json:"output"`
	Repository           FreezeEvidence            `json:"repository"`
	OwnCard              *Card                     `json:"own_card"`
	SubmittedCards       []Card                    `json:"submitted_cards"`
	Deliberation         bool                      `json:"deliberation"`
}

// FreezeEvidence carries only the exact-final native consumer's authorized fields.
type FreezeEvidence struct {
	SourceID   uuid.UUID  `json:"source_id"`
	Status     string     `json:"status"`
	SHA        string     `json:"sha"`
	DecisionID *uuid.UUID `json:"decision_id"`
}

func freezeEvidence(v submissionfreeze.FinalEvidence) FreezeEvidence {
	return FreezeEvidence{v.SourceID, v.Status, v.SHA, v.DecisionID}
}

type Practice struct {
	FictionalEntryID uuid.UUID               `json:"fictional_entry_id"`
	Name             string                  `json:"name"`
	Rubric           rubric.Reference        `json:"rubric"`
	Criteria         rubric.PublicDefinition `json:"criteria"`
	Card             *PracticeCard           `json:"card"`
}
type PracticeCard struct {
	Version int64            `json:"version"`
	Rubric  rubric.Reference `json:"rubric"`
	State   string           `json:"state"`
	Scores  []Score          `json:"scores"`
}
type PracticeCommand struct {
	RequestID uuid.UUID `json:"request_id"`
	Action    string    `json:"action"`
	Scores    []Score   `json:"scores"`
}

type Receipt struct {
	RequestID   uuid.UUID `json:"request_id"`
	ActorID     uuid.UUID `json:"actor_id"`
	RunID       uuid.UUID `json:"run_id"`
	TeamID      uuid.UUID `json:"team_id"`
	Action      string    `json:"action"`
	Revision    int64     `json:"revision"`
	State       string    `json:"state"`
	Fingerprint string    `json:"fingerprint"`
}
