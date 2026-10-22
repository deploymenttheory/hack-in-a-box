package automation

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
)

// ClarificationSource consumes actual current common-answer publications.
// Private originating questions and attachments never enter automation.
type ClarificationSource struct{ Answers *clarification.Service }

func (s ClarificationSource) DiscoverInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	d Definition,
) ([]Source, error) {
	return s.discover(ctx, tx, eventID, d, nil)
}

func (s ClarificationSource) DiscoverUnknownInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	binding Binding,
) ([]Source, error) {
	return s.discover(ctx, tx, eventID, binding.Definition, &binding)
}

func (s ClarificationSource) discover(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	d Definition,
	binding *Binding,
) ([]Source, error) {
	if s.Answers == nil {
		return nil, communications.ErrUnresolved
	}
	result := []Source{}
	after := uuid.Nil
	for {
		page, next, err := s.Answers.DiscoverNotificationSourcesPageInTransaction(
			ctx,
			tx,
			eventID,
			d.SourceID,
			after,
			200,
		)
		if err != nil {
			return nil, fmt.Errorf("discover native common-answer transitions: %w", err)
		}
		for _, native := range page {
			if d.SourceVersion > 0 && native.Version != d.SourceVersion {
				continue
			}
			if binding != nil {
				var known bool
				err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM communication_occurrences WHERE event_id=$1 AND binding_id=$2 AND binding_version=$3 AND source_kind='clarification.published' AND source_id=$4 AND source_revision=$5)`, eventID, binding.Definition.ID, binding.Version, native.AnswerID, native.Version).
					Scan(&known)
				if err != nil {
					return nil, fmt.Errorf("check recorded common-answer occurrence: %w", err)
				}
				if known {
					continue
				}
			}
			result = append(
				result,
				Source{
					Kind:       "clarification.published",
					ID:         native.AnswerID,
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

func (s ClarificationSource) ResolveInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	source Source,
) (SourceAudience, error) {
	if s.Answers == nil || source.Kind != "clarification.published" {
		return SourceAudience{}, communications.ErrUnresolved
	}
	current, err := s.Answers.ResolveNotificationAudienceInTransaction(
		ctx,
		tx,
		eventID,
		source.ID,
		source.Revision,
	)
	if err != nil {
		return SourceAudience{}, fmt.Errorf("resolve native common-answer audience: %w", err)
	}
	return SourceAudience{RecipientIDs: current, Broadcast: true, RequireComplete: true}, nil
}
