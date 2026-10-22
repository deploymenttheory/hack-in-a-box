package help

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

type NotificationSource struct {
	RequestID  uuid.UUID
	Revision   int64
	OccurredAt time.Time
	Action     string
}
type NotificationAudience struct {
	RequestID    uuid.UUID
	Revision     int64
	Status       string
	RequesterID  uuid.UUID
	ResourceID   uuid.UUID
	RecipientIDs []uuid.UUID
}

func factStatus(r Request) string {
	switch r.Status {
	case "resolved":
		return "resolved"
	case "cancelled":
		return "withdrawn"
	default:
		return "open"
	}
}

// DiscoverNotificationSourcesInTransaction exposes each request's actual latest
// transition. Older revisions are no longer safe dispatch sources; the owning
// inbox route always rechecks the currently authorised resource.
// DiscoverNotificationSourcesPageInTransaction filters the event/request before
// its bounded UUID keyset. The adapter can scan past already-known occurrences
// without a permanent event-history cap or loading any private request context.
func (s Service) DiscoverNotificationSourcesPageInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	requestedID *uuid.UUID,
	after uuid.UUID,
	limit int32,
) ([]NotificationSource, *uuid.UUID, error) {
	if limit < 1 || limit > 1000 {
		return nil, nil, access.ErrInvalid
	}
	q := dbgen.New(tx)
	if _, err := lock(ctx, q, eventID); err != nil {
		return nil, nil, err
	}
	rows, err := q.HelpNotificationSourcesPage(
		ctx,
		dbgen.HelpNotificationSourcesPageParams{
			EventID:     eventID,
			RequestedID: requestedID,
			AfterID:     after,
			PageSize:    limit + 1,
		},
	)
	if err != nil {
		return nil, nil, wrap("page help source metadata", err)
	}
	var next *uuid.UUID
	if len(rows) > int(limit) {
		rows = rows[:limit]
		cursor := rows[len(rows)-1].RequestID
		next = &cursor
	}
	result := []NotificationSource{}
	for _, row := range rows {
		result = append(
			result,
			NotificationSource{
				RequestID:  row.RequestID,
				Revision:   row.Revision,
				OccurredAt: row.OccurredAt,
				Action:     row.Action,
			},
		)
	}
	return result, next, nil
}

func (s Service) DiscoverNotificationSourcesInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
) ([]NotificationSource, error) {
	result := []NotificationSource{}
	after := uuid.Nil
	for {
		page, next, err := s.DiscoverNotificationSourcesPageInTransaction(
			ctx,
			tx,
			eventID,
			nil,
			after,
			1000,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, page...)
		if next == nil {
			return result, nil
		}
		after = *next
	}
}

func (s Service) resolveNotificationAudience(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, id uuid.UUID,
	revision int64,
) (NotificationAudience, error) {
	var result NotificationAudience
	row, err := lock(ctx, q, eventID)
	if err != nil {
		return result, err
	}
	r, err := load(ctx, q, eventID, id)
	if err != nil {
		return result, err
	}
	if revision < 1 || revision != r.Revision {
		return result, access.ErrNotFound
	}
	people, err := s.recipients(ctx, q, row, r)
	if err != nil {
		return result, err
	}
	return NotificationAudience{
		RequestID:    r.ID,
		Revision:     r.Revision,
		Status:       factStatus(r),
		RequesterID:  r.RequesterID,
		ResourceID:   r.ID,
		RecipientIDs: people,
	}, nil
}

func (s Service) ResolveNotificationAudienceInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, id uuid.UUID,
	revision int64,
) (NotificationAudience, error) {
	return s.resolveNotificationAudience(ctx, dbgen.New(tx), eventID, id, revision)
}

// AuthorizeCommunicationFact is the native help owner. It deliberately has no
// fallback to fact.OwnerID when the request is absent, stale or inconsistent.
func (s Service) AuthorizeCommunicationFact(
	ctx context.Context,
	q *dbgen.Queries,
	actor uuid.UUID,
	fact dbgen.CommunicationFact,
) (bool, error) {
	if fact.Kind != "help" || fact.ResourceID == nil || *fact.ResourceID != fact.ID ||
		fact.OwnerID == nil {
		return false, nil
	}
	row, err := lock(ctx, q, fact.EventID)
	if errors.Is(err, access.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	current, err := load(ctx, q, fact.EventID, fact.ID)
	if errors.Is(err, access.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if current.Revision != fact.Revision || factStatus(current) != fact.Status ||
		current.RequesterID != *fact.OwnerID {
		return false, nil
	}
	// Evaluate this recipient directly; do not rebuild every recipient's
	// audience inside each personalised renderer invocation.
	_, err = s.authorizeRead(ctx, q, row, actor, current)
	if errors.Is(err, access.ErrNotFound) || errors.Is(err, event.ErrBlocked) {
		return false, nil
	}
	return err == nil, err
}
