package objectives

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	oq "github.com/deploymenttheory/hack-in-a-box/internal/objectives/objectivequery"
)

// Disclosure selects an exact immutable objective snapshot and its explicitly
// reviewed measures. The native content document is the audience/approval owner.
// No bare public flag on planning configuration grants attendee access.
type Disclosure struct {
	Objective  Reference   `json:"objective"`
	MeasureIDs []uuid.UUID `json:"measure_ids"`
}

type PublicMeasure struct {
	ID             uuid.UUID `json:"id"`
	Description    string    `json:"description"`
	Target         *string   `json:"target"`
	Unit           string    `json:"unit"`
	Method         string    `json:"method"`
	EvidenceSource string    `json:"evidence_source"`
	ReviewHorizon  string    `json:"review_horizon"`
}

type PublicDefinition struct {
	Reference   Reference       `json:"reference"`
	Description string          `json:"description"`
	Measures    []PublicMeasure `json:"measures"`
}

func validateDisclosure(d Disclosure) error {
	if d.Objective.ID == uuid.Nil || d.Objective.Version < 1 || len(d.MeasureIDs) > 50 {
		return access.ErrInvalid
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range d.MeasureIDs {
		if id == uuid.Nil || seen[id] {
			return access.ErrInvalid
		}
		seen[id] = true
	}
	return nil
}

// ValidateDisclosuresInTransaction validates only references, not publication
// authority. The actual native content approval/release transaction owns that.
func ValidateDisclosuresInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	refs []Disclosure,
	release bool,
	initialChallenge bool,
	now time.Time,
) error {
	if len(refs) > 100 {
		return access.ErrInvalid
	}
	seen := map[uuid.UUID]bool{}
	for _, ref := range refs {
		if err := validateDisclosure(ref); err != nil {
			return err
		}
		if seen[ref.Objective.ID] {
			return access.ErrInvalid
		}
		seen[ref.Objective.ID] = true
		v, err := load(ctx, oq.New(tx), eventID, ref.Objective)
		if err != nil {
			return err
		}
		head, err := oq.New(tx).
			ObjectiveHead(ctx, oq.ObjectiveHeadParams{EventID: eventID, ID: ref.Objective.ID})
		if err != nil {
			return fmt.Errorf("read objective disclosure head: %w", err)
		}
		current, err := load(
			ctx,
			oq.New(tx),
			eventID,
			Reference{ID: head.ID, Version: head.CurrentVersion},
		)
		if err != nil {
			return err
		}
		if v.Retired || current.Retired {
			return access.ErrConflict
		}
		for _, id := range ref.MeasureIDs {
			if !slices.ContainsFunc(
				v.Definition.Measures,
				func(m Measure) bool { return m.ID == id },
			) {
				return access.ErrInvalid
			}
		}
		if release && v.Definition.ThemeSensitive && !initialChallenge {
			row, err := dbgen.New(tx).Event(ctx, eventID)
			if err != nil {
				return fmt.Errorf("read objective challenge embargo: %w", err)
			}
			if !row.PublishedAt.Valid || row.PublishedAt.Time.After(now) {
				return access.ErrForbidden
			}
		}
	}
	return nil
}

// Public resolves only a reference retained in the exact currently readable
// released native content version. It never uses privileged preview authority,
// and does not expose private owner identities or planning-baseline self-reports.
func (s Service) Public(
	ctx context.Context,
	actor, eventID, contentID uuid.UUID,
	ref Reference,
) (PublicDefinition, error) {
	out := PublicDefinition{Measures: []PublicMeasure{}}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock objective disclosure event: %w", err)
		}
		item, err := q.ContentItem(ctx, dbgen.ContentItemParams{EventID: eventID, ID: contentID})
		if err != nil {
			return access.ErrNotFound
		}
		allowed, err := access.ContentVersionForReader(ctx, q, actor, item, false)
		if err != nil {
			return fmt.Errorf("authorize disclosed objective source: %w", err)
		}
		if allowed == 0 {
			return access.ErrNotFound
		}
		source, err := q.ContentVersion(
			ctx,
			dbgen.ContentVersionParams{EventID: eventID, ContentID: contentID, Version: allowed},
		)
		if err != nil {
			return fmt.Errorf("read objective disclosure source: %w", err)
		}
		var native struct {
			Objectives []Disclosure `json:"objectives"`
		}
		if err = json.Unmarshal(source.Document, &native); err != nil {
			return fmt.Errorf("decode objective disclosure source: %w", err)
		}
		var selected *Disclosure
		for i := range native.Objectives {
			if native.Objectives[i].Objective == ref {
				selected = &native.Objectives[i]
				break
			}
		}
		if selected == nil {
			return access.ErrNotFound
		}
		v, err := load(ctx, oq.New(tx), eventID, ref)
		if err != nil {
			return err
		}
		if v.Definition.ThemeSensitive &&
			(!row.PublishedAt.Valid || row.PublishedAt.Time.After(s.now())) {
			return access.ErrNotFound
		}
		out.Reference = ref
		out.Description = v.Definition.Description
		for _, m := range v.Definition.Measures {
			if slices.Contains(selected.MeasureIDs, m.ID) {
				out.Measures = append(
					out.Measures,
					PublicMeasure{
						ID:             m.ID,
						Description:    m.Description,
						Target:         m.Target,
						Unit:           m.Unit,
						Method:         m.Method,
						EvidenceSource: m.EvidenceSource,
						ReviewHorizon:  m.ReviewHorizon,
					},
				)
			}
		}
		return nil
	})
	if err != nil {
		return out, fmt.Errorf("read disclosed objective: %w", err)
	}
	return out, nil
}
