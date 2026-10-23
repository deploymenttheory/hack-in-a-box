package submissions

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// OutputSummary is a factual formal-output projection. It excludes assessment,
// qualification, reviewer commentary, working drafts and submitted prose/URLs.
type OutputSummary struct {
	TeamID         uuid.UUID       `json:"team_id"`
	EntryID        uuid.UUID       `json:"entry_id"`
	Outcome        string          `json:"outcome"`
	FrozenAt       *time.Time      `json:"frozen_at"`
	FrozenRevision *int64          `json:"frozen_revision"`
	SubmittedAt    *time.Time      `json:"submitted_at"`
	Artifacts      []ArtifactCount `json:"artifacts"`
}
type ArtifactCount struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

// SummariesInTransaction authorizes every requested team through the existing
// native submission policy BEFORE settling, then reconciles the event once.
// The caller must still impose its dashboard or explicit organiser-preview role.
func (s Service) SummariesInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	teamIDs []uuid.UUID,
) ([]OutputSummary, error) {
	out := []OutputSummary{}
	if len(teamIDs) > 100 {
		return out, access.ErrInvalid
	}
	if len(teamIDs) == 0 {
		return out, nil
	}
	scopes := make([]scope, 0, len(teamIDs))
	for _, teamID := range teamIDs {
		v, err := s.authorizedScope(ctx, tx, actor, eventID, teamID)
		if err != nil {
			return out, err
		}
		scopes = append(scopes, v)
	}
	if err := s.Settle(ctx, tx, scopes[0].row, s.now()); err != nil {
		return out, err
	}
	q := dbgen.New(tx)
	for i, v := range scopes {
		current, err := loadScopeState(ctx, q, v, eventID, teamIDs[i])
		if err != nil {
			return out, err
		}
		item := OutputSummary{
			TeamID:    teamIDs[i],
			EntryID:   current.entry.ID,
			Outcome:   current.state.Outcome,
			Artifacts: []ArtifactCount{},
		}
		if current.state.Outcome == "frozen" && current.state.FrozenAt.Valid &&
			current.state.ActiveRevision.Valid {
			item.FrozenAt = &current.state.FrozenAt.Time
			item.FrozenRevision = &current.state.ActiveRevision.Int64
			row, readErr := q.SubmissionRevision(
				ctx,
				dbgen.SubmissionRevisionParams{
					EventID:  eventID,
					TeamID:   teamIDs[i],
					Revision: *item.FrozenRevision,
				},
			)
			if readErr != nil {
				return out, fmt.Errorf("read exact frozen factual source: %w", readErr)
			}
			var document Document
			if readErr = json.Unmarshal(row.Document, &document); readErr != nil {
				return out, fmt.Errorf("read frozen artifact facts: %w", readErr)
			}
			item.SubmittedAt = &row.SubmittedAt
			counts := map[string]int{}
			permitted := 0
			for _, evidence := range document.Evidence {
				index := slices.IndexFunc(
					document.Definitions,
					func(def Definition) bool { return def.ID == evidence.DeliverableID },
				)
				if index < 0 || !slices.Contains(audience(document.Definitions[index]), "judge") {
					continue
				}
				counts[evidence.Kind]++
				permitted++
			}
			unknown := permitted - counts["file"] - counts["url"] - counts["repository"]
			if unknown > 0 {
				item.Artifacts = append(
					item.Artifacts,
					ArtifactCount{Kind: "other", Count: unknown},
				)
			}
			for _, kind := range []string{"file", "url", "repository"} {
				if counts[kind] > 0 {
					item.Artifacts = append(
						item.Artifacts,
						ArtifactCount{Kind: kind, Count: counts[kind]},
					)
				}
			}
		}
		out = append(out, item)
	}
	return out, nil
}
