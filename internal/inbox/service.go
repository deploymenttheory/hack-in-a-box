package inbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

var errScanLimit = errors.New("inbox authorization scan unavailable")

func (s Service) List(
	ctx context.Context,
	actor uuid.UUID,
	filter, cursor *uuid.UUID,
	limit int,
) (Page, error) {
	result := Page{Items: []Item{}}
	if actor == uuid.Nil || limit < 1 || limit > 100 {
		return result, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		person, err := q.Person(ctx, actor)
		if err != nil || !person.Active {
			return access.ErrForbidden
		}
		rows, err := q.InboxCandidates(
			ctx,
			dbgen.InboxCandidatesParams{RecipientID: actor, EventFilter: filter},
		)
		if err != nil {
			return fmt.Errorf("load notification candidates: %w", err)
		}
		if len(rows) > 10000 {
			return errScanLimit
		}
		// Acquire all event guards in a stable order before per-source checks.
		events := map[uuid.UUID]bool{}
		for _, row := range rows {
			events[row.EventID] = true
		}
		eventIDs := make([]uuid.UUID, 0, len(events))
		for id := range events {
			eventIDs = append(eventIDs, id)
		}
		sort.Slice(
			eventIDs,
			func(i, j int) bool { return bytes.Compare(eventIDs[i][:], eventIDs[j][:]) < 0 },
		)
		for _, id := range eventIDs {
			if _, err = q.LockEvent(ctx, id); err != nil {
				return fmt.Errorf("lock inbox events: %w", err)
			}
		}
		for _, row := range rows {
			destination, authErr := s.authorize(
				ctx,
				tx,
				actor,
				row.EventID,
				Source{Kind: row.SourceKind, ID: row.SourceID, Version: row.SourceVersion},
				row.Provenance,
			)
			err = authErr
			if hidden(err) {
				continue
			}
			if err != nil {
				return err
			}
			if !row.ReadAt.Valid {
				result.Unread++
			}
			if cursor != nil && bytes.Compare(row.ID[:], cursor[:]) >= 0 {
				continue
			}
			item := itemFrom(
				dbgen.InboxNotificationRow{
					ID:         row.ID,
					EventID:    row.EventID,
					EventName:  row.EventName,
					SourceKind: row.SourceKind,
					CreatedAt:  row.CreatedAt,
					ReadAt:     row.ReadAt,
				},
			)
			if destination != nil && destination.Kind == "outcome_review" {
				item.EventName = ""
				item.Label = label("outcome_review")
			}
			if len(result.Items) < limit {
				result.Items = append(result.Items, item)
			} else if result.NextCursor == nil {
				last := result.Items[len(result.Items)-1].ID
				result.NextCursor = &last
			}
		}
		return nil
	})
	return result, wrap("list", err)
}

func itemFrom(row dbgen.InboxNotificationRow) Item {
	item := Item{
		ID:        row.ID,
		EventID:   row.EventID,
		EventName: row.EventName,
		Label:     label(row.SourceKind),
		CreatedAt: row.CreatedAt,
	}
	if row.ReadAt.Valid {
		item.ReadAt = &row.ReadAt.Time
	}
	return item
}

func (s Service) Open(ctx context.Context, actor, id uuid.UUID, read *bool) (Detail, error) {
	var result Detail
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := q.InboxNotification(
			ctx,
			dbgen.InboxNotificationParams{ID: id, RecipientID: actor},
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("load recipient notification: %w", err)
		}
		destination, err := s.authorize(
			ctx,
			tx,
			actor,
			row.EventID,
			Source{Kind: row.SourceKind, ID: row.SourceID, Version: row.SourceVersion},
			row.Provenance,
		)
		if hidden(err) {
			return access.ErrNotFound
		}
		if err != nil {
			return err
		}
		if read != nil {
			row.ReadAt, err = q.MarkInboxRead(
				ctx,
				dbgen.MarkInboxReadParams{ID: id, RecipientID: actor, IsRead: *read},
			)
			if err != nil {
				return fmt.Errorf("update recipient read state: %w", err)
			}
		}
		if destination != nil && destination.Kind == "" {
			destination = nil
		}
		result = Detail{
			Item:        itemFrom(row),
			Subject:     row.Subject,
			Body:        row.Body,
			Format:      row.Format,
			Destination: destination,
		}
		if destination != nil && destination.Kind == "outcome_review" {
			result.Item.EventName = ""
			result.Item.Label = label("outcome_review")
		}
		return nil
	})
	return result, wrap("open", err)
}

func wrap(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("inbox %s: %w", action, err)
}
