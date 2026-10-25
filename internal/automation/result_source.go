package automation

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/results"
)

// ResultSource consumes actual current result publications.
// Private assessments, preparation and ballots never enter automation.
type ResultSource struct{ Results *results.Service }

func (s ResultSource) DiscoverInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	d Definition,
) ([]Source, error) {
	return s.discover(ctx, tx, eventID, d, nil)
}

func (s ResultSource) DiscoverUnknownInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	binding Binding,
) ([]Source, error) {
	return s.discover(ctx, tx, eventID, binding.Definition, &binding)
}

func (s ResultSource) discover(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	d Definition,
	binding *Binding,
) ([]Source, error) {
	if s.Results == nil {
		return nil, communications.ErrUnresolved
	}
	result := []Source{}
	after := uuid.Nil
	for {
		page, next, err := s.Results.DiscoverNotificationSourcesPageInTransaction(
			ctx,
			tx,
			eventID,
			d.SourceID,
			after,
			200,
		)
		if err != nil {
			return nil, fmt.Errorf("discover native result transitions: %w", err)
		}
		for _, native := range page {
			if d.SourceVersion > 0 && native.Version != d.SourceVersion {
				continue
			}
			if binding != nil {
				var known bool
				err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM communication_occurrences WHERE event_id=$1 AND binding_id=$2 AND binding_version=$3 AND source_kind='results.published' AND source_id=$4 AND source_revision=$5)`, eventID, binding.Definition.ID, binding.Version, native.ID, native.Version).
					Scan(&known)
				if err != nil {
					return nil, fmt.Errorf("check recorded result occurrence: %w", err)
				}
				if known {
					continue
				}
			}
			result = append(
				result,
				Source{
					Kind:       "results.published",
					ID:         native.ID,
					Revision:   native.Version,
					OccurredAt: native.PublishedAt,
				},
			)
			if binding != nil && len(result) == 200 {
				return result, nil
			}
		}
		if next == nil {
			return result, nil
		}
		after = *next
	}
}

func (s ResultSource) ResolveInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	source Source,
) (SourceAudience, error) {
	if s.Results == nil || source.Kind != "results.published" {
		return SourceAudience{}, communications.ErrUnresolved
	}
	current, err := s.Results.ResolveNotificationAudienceInTransaction(
		ctx,
		tx,
		eventID,
		source.ID,
		source.Revision,
	)
	if err != nil {
		return SourceAudience{}, fmt.Errorf("resolve native result audience: %w", err)
	}
	return SourceAudience{RecipientIDs: current, Broadcast: true, RequireComplete: true}, nil
}
