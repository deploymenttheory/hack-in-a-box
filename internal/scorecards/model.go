// Package scorecards owns independent private mentor cards, not moderated
// results, rankings, awards or a second rubric definition.
package scorecards

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

type Service struct {
	Pool       *pgxpool.Pool
	Now        func() time.Time
	Moderation ModerationPolicy
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

type CheckpointDefinition struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	RunID          uuid.UUID `json:"run_id"`
	OpensWallTime  string    `json:"opens_wall_time"`
	ClosesWallTime string    `json:"closes_wall_time"`
	DSTChoice      string    `json:"dst_choice"`
}
type Checkpoint struct {
	Definition CheckpointDefinition    `json:"definition"`
	Version    int64                   `json:"version"`
	Purpose    string                  `json:"purpose"`
	Rubric     rubric.Reference        `json:"rubric"`
	Criteria   rubric.PublicDefinition `json:"criteria"`
	Timezone   string                  `json:"timezone"`
	OpensAt    time.Time               `json:"opens_at"`
	ClosesAt   time.Time               `json:"closes_at"`
}
type EvidenceReference struct {
	Kind    string     `json:"kind"`
	ID      *uuid.UUID `json:"id"`
	Version *int64     `json:"version"`
	Note    string     `json:"note"`
}
type CategoryScore struct {
	CategoryID uuid.UUID           `json:"category_id"`
	Pending    bool                `json:"pending"`
	Value      *int                `json:"value"`
	Evidence   []EvidenceReference `json:"evidence"`
}
type CardReference struct {
	ID      uuid.UUID `json:"id"`
	Version int64     `json:"version"`
}
type Card struct {
	Revision           int64                   `json:"revision"`
	Reference          CardReference           `json:"reference"`
	TeamID             uuid.UUID               `json:"team_id"`
	CheckpointID       uuid.UUID               `json:"checkpoint_id"`
	CheckpointVersion  int64                   `json:"checkpoint_version"`
	AuthorID           uuid.UUID               `json:"author_id"`
	RunID              uuid.UUID               `json:"run_id"`
	Rubric             rubric.Reference        `json:"rubric"`
	Criteria           rubric.PublicDefinition `json:"criteria"`
	Purpose            string                  `json:"purpose"`
	SubmissionRevision *int64                  `json:"submission_revision"`
	State              string                  `json:"state"`
	Scores             []CategoryScore         `json:"scores"`
	CreatedAt          time.Time               `json:"created_at"`
	ChangedAt          time.Time               `json:"changed_at"`
}
type CheckpointCommand struct {
	Definition CheckpointDefinition `json:"definition"`
	Version    int64                `json:"version"`
	Reason     string               `json:"reason"`
}
type CardCommand struct {
	Action            string          `json:"action"`
	CheckpointID      uuid.UUID       `json:"checkpoint_id"`
	CheckpointVersion int64           `json:"checkpoint_version"`
	Version           int64           `json:"version"`
	Scores            []CategoryScore `json:"scores"`
}
type CheckpointPage struct {
	Revision   int64        `json:"revision"`
	Items      []Checkpoint `json:"items"`
	NextCursor *uuid.UUID   `json:"next_cursor"`
}
type Page struct {
	Revision   int64      `json:"revision"`
	Items      []Card     `json:"items"`
	NextCursor *uuid.UUID `json:"next_cursor"`
}
type History struct {
	Items      []Card `json:"items"`
	NextCursor *int64 `json:"next_cursor"`
}

// ModerationContext is a canonical owning-domain proof. Only a registered policy
// resolving a persisted current huddle/review may supply it. Caller-supplied
// metadata, independent drafts and nil policies never grant panel authority.
type ModerationContext struct {
	ID                 uuid.UUID       `json:"id"`
	Version            int64           `json:"version"`
	Binding            string          `json:"binding"`
	TeamID             uuid.UUID       `json:"team_id"`
	RunID              uuid.UUID       `json:"run_id"`
	SubmissionRevision int64           `json:"submission_revision"`
	RequiredAuthorIDs  []uuid.UUID     `json:"required_author_ids"`
	Cards              []CardReference `json:"cards"`
	UnresolvedGaps     int             `json:"unresolved_gaps"`
}
type ModerationPolicy interface {
	ResolveScorecardContextInTransaction(
		context.Context,
		pgx.Tx,
		uuid.UUID,
		uuid.UUID,
		uuid.UUID,
		uuid.UUID,
	) (ModerationContext, error)
}
type Signoff struct {
	ID        uuid.UUID         `json:"id"`
	TeamID    uuid.UUID         `json:"team_id"`
	RunID     uuid.UUID         `json:"run_id"`
	Context   ModerationContext `json:"context"`
	ActorID   uuid.UUID         `json:"actor_id"`
	CreatedAt time.Time         `json:"created_at"`
}

func digest(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func limit(value int) (int32, error) {
	if value == 0 {
		return 50, nil
	}
	if value < 1 || value > 100 {
		return 0, access.ErrInvalid
	}
	return int32(value), nil
}

func normalizeScores(
	input []CategoryScore,
	definition rubric.Definition,
	complete bool,
) ([]CategoryScore, error) {
	if len(input) > len(definition.Categories) {
		return nil, access.ErrInvalid
	}
	byID := map[uuid.UUID]CategoryScore{}
	for _, score := range input {
		if _, found := byID[score.CategoryID]; found {
			return nil, access.ErrInvalid
		}
		if !slices.ContainsFunc(
			definition.Categories,
			func(c rubric.Category) bool { return c.ID == score.CategoryID },
		) {
			return nil, access.ErrInvalid
		}
		byID[score.CategoryID] = score
	}
	out := make([]CategoryScore, 0, len(definition.Categories))
	for _, category := range definition.Categories {
		score, found := byID[category.ID]
		if !found {
			score = CategoryScore{
				CategoryID: category.ID,
				Pending:    true,
				Evidence:   []EvidenceReference{},
			}
		}
		if score.Evidence == nil {
			score.Evidence = []EvidenceReference{}
		}
		if (score.Pending && score.Value != nil) || (!score.Pending && score.Value == nil) ||
			len(score.Evidence) > 50 {
			return nil, access.ErrInvalid
		}
		if score.Value != nil &&
			(*score.Value < category.MinScore || *score.Value > category.MaxScore) {
			return nil, access.ErrInvalid
		}
		if complete && (score.Pending || len(score.Evidence) == 0) {
			return nil, access.ErrConflict
		}
		for _, e := range score.Evidence {
			if len(e.Note) > 10000 {
				return nil, access.ErrInvalid
			}
			switch e.Kind {
			case "note":
				if strings.TrimSpace(e.Note) == "" || e.ID != nil || e.Version != nil {
					return nil, access.ErrInvalid
				}
			case "observation", "submission":
				if e.ID == nil || *e.ID == uuid.Nil || e.Version == nil || *e.Version < 1 {
					return nil, access.ErrInvalid
				}
			default:
				return nil, access.ErrInvalid
			}
		}
		out = append(out, score)
	}
	return out, nil
}
