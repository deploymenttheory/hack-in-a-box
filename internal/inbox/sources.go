package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
)

func label(kind string) string {
	switch kind {
	case "test_self":
		return "Test message for you"
	case "campaign":
		return "Event announcement"
	case "content":
		return "Released event resource"
	case "help":
		return "Help-request update"
	case "clarification":
		return "Reviewed common clarification"
	case "outcome_review":
		return "Outcome-review reminder"
	default:
		return "Event update"
	}
}

func (s Service) authorize(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	source Source,
	provenance []byte,
) (*Destination, error) {
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("lock notification event: %w", err)
	}
	person, err := q.Person(ctx, actor)
	if err != nil || !person.Active {
		return nil, access.ErrNotFound
	}
	switch source.Kind {
	case "test_self", "campaign":
		if len(provenance) == 0 {
			return nil, access.ErrNotFound
		}
		var retained communications.InboxProvenance
		if json.Unmarshal(provenance, &retained) != nil ||
			retained.SelfOnly != (source.Kind == "test_self") {
			return nil, access.ErrNotFound
		}
		if err = s.Communications.AuthorizeInboxRecipientInTransaction(
			ctx,
			tx,
			eventID,
			actor,
			retained,
		); err != nil {
			return nil, wrap("authorize source", err)
		}
		return &Destination{}, nil
	case "content":
		item, itemErr := q.ContentItem(
			ctx,
			dbgen.ContentItemParams{EventID: eventID, ID: source.ID},
		)
		if itemErr != nil {
			return nil, access.ErrNotFound
		}
		version, versionErr := access.ContentVersionForReader(ctx, q, actor, item, false)
		if versionErr != nil {
			return nil, wrap("authorize content version", versionErr)
		}
		if version == 0 || version != source.Version || !item.ReleasedAt.Valid ||
			s.now().Before(item.ReleasedAt.Time) {
			return nil, access.ErrNotFound
		}
		if item.Kind == "challenge" {
			var configuration event.Configuration
			if err = json.Unmarshal(row.Configuration, &configuration); err != nil {
				return nil, fmt.Errorf("decode notification schedule: %w", err)
			}
			planned, plannedErr := schedule.ChallengePublication(
				configuration.LiveStart,
				configuration.Timezone,
				configuration.DSTChoice,
			)
			if plannedErr != nil || s.now().Before(planned) || !row.PublishedAt.Valid ||
				s.now().Before(row.PublishedAt.Time) {
				return nil, access.ErrNotFound
			}
		}
		return &Destination{Kind: "content", ResourceID: source.ID}, nil
	default:
		owner := s.Owners[source.Kind]
		if owner == nil {
			return nil, access.ErrNotFound
		}
		if err = (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.EventRead,
		); err != nil {
			return nil, wrap("authorize source", err)
		}
		destination, ownerErr := owner.AuthorizeNotification(ctx, tx, actor, eventID, source)
		if ownerErr != nil {
			return nil, wrap("authorize owning resource", ownerErr)
		}
		// An owner cannot introduce arbitrary browser destinations or activate a
		// journey before that route has been implemented and reviewed here.
		if source.Kind == "help" && destination.Kind == "help" &&
			destination.ResourceID == source.ID {
			return &destination, nil
		}
		return nil, access.ErrNotFound
	}
}

func hidden(err error) bool {
	return errors.Is(err, access.ErrNotFound) || errors.Is(err, access.ErrForbidden) ||
		errors.Is(err, access.ErrConflict) ||
		errors.Is(err, communications.ErrUnresolved) ||
		errors.Is(err, event.ErrBlocked)
}
