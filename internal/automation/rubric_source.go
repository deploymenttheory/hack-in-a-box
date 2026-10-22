package automation

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

// RubricSource binds to the native atomic challenge publication tuple rather
// than a planning date, rubric draft, or manufactured generic fact.
type RubricSource struct{ Rubric rubric.Service }

func (s RubricSource) DiscoverInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	d Definition,
) ([]Source, error) {
	current, err := s.Rubric.DiscoverNotificationSourcesInTransaction(ctx, tx, eventID, d.SourceID)
	if err != nil {
		return nil, fmt.Errorf("discover native rubric sources: %w", err)
	}
	result := []Source{}
	for _, native := range current {
		result = append(
			result,
			Source{
				Kind:       "rubric.published",
				ID:         native.ChallengeID,
				Revision:   native.ChallengeVersion,
				OccurredAt: native.PublishedAt,
			},
		)
	}
	return result, nil
}

func (s RubricSource) ResolveInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	source Source,
) (SourceAudience, error) {
	if source.Kind != "rubric.published" {
		return SourceAudience{}, communications.ErrUnresolved
	}
	people, err := s.Rubric.ResolveNotificationAudienceInTransaction(
		ctx,
		tx,
		eventID,
		source.ID,
		source.Revision,
	)
	if err != nil {
		return SourceAudience{}, fmt.Errorf("resolve native rubric source audience: %w", err)
	}
	return SourceAudience{RecipientIDs: people, Broadcast: true, RequireComplete: true}, nil
}
