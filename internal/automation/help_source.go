package automation

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/help"
)

// HelpSource uses only the native owner's actual transitions and current
// resource audience. Private request text and locations never enter automation.
type HelpSource struct{ Help help.Service }

func (s HelpSource) DiscoverInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	d Definition,
) ([]Source, error) {
	return s.discover(ctx, tx, eventID, d, nil)
}

func (s HelpSource) DiscoverUnknownInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	binding Binding,
) ([]Source, error) {
	return s.discover(ctx, tx, eventID, binding.Definition, &binding)
}

func (s HelpSource) discover(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	d Definition,
	binding *Binding,
) ([]Source, error) {
	result := []Source{}
	after := uuid.Nil
	for {
		page, next, err := s.Help.DiscoverNotificationSourcesPageInTransaction(
			ctx,
			tx,
			eventID,
			d.SourceID,
			after,
			200,
		)
		if err != nil {
			return nil, fmt.Errorf("discover native help transitions: %w", err)
		}
		for _, native := range page {
			if d.SourceVersion > 0 && native.Revision != d.SourceVersion {
				continue
			}
			if binding != nil {
				var known bool
				err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM communication_occurrences WHERE event_id=$1 AND binding_id=$2 AND binding_version=$3 AND source_kind='help.updated' AND source_id=$4 AND source_revision=$5)`, eventID, binding.Definition.ID, binding.Version, native.RequestID, native.Revision).
					Scan(&known)
				if err != nil {
					return nil, fmt.Errorf("check recorded help occurrence: %w", err)
				}
				if known {
					continue
				}
			}
			result = append(
				result,
				Source{
					Kind:       "help.updated",
					ID:         native.RequestID,
					Revision:   native.Revision,
					OccurredAt: native.OccurredAt,
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

func (s HelpSource) ResolveInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	source Source,
) (SourceAudience, error) {
	if source.Kind != "help.updated" {
		return SourceAudience{}, communications.ErrUnresolved
	}
	current, err := s.Help.ResolveNotificationAudienceInTransaction(
		ctx,
		tx,
		eventID,
		source.ID,
		source.Revision,
	)
	if err != nil {
		return SourceAudience{}, fmt.Errorf("resolve native help audience: %w", err)
	}
	return SourceAudience{RecipientIDs: current.RecipientIDs}, nil
}
