package clarification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

func published(
	row dbgen.PublishedClarificationHistoryRow,
	current Source,
) (PublishedAnswer, error) {
	var document AnswerDocument
	var bound Source
	if err := json.Unmarshal(row.Document, &document); err != nil {
		return PublishedAnswer{}, fmt.Errorf("decode published answer document: %w", err)
	}
	if err := json.Unmarshal(row.Source, &bound); err != nil {
		return PublishedAnswer{}, fmt.Errorf("decode published answer source: %w", err)
	}
	return PublishedAnswer{
		ID:            row.ID,
		Version:       row.Version,
		Document:      document,
		Source:        bound,
		AuthorName:    row.AuthorName,
		AuthoredAt:    row.AuthoredAt,
		ApprovedAt:    row.ApprovedAt,
		PublishedAt:   row.PublishedAt,
		Current:       row.Current,
		SourceCurrent: bound == current,
	}, nil
}

func (s Service) Public(
	ctx context.Context,
	actor, eventID, answerID uuid.UUID,
) (PublicPage, error) {
	var out PublicPage
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		now := s.now()
		ref, err := rubric.ReleasedSourceForReaderInTransaction(ctx, tx, actor, eventID, now)
		if err != nil {
			return fmt.Errorf("Public: %w", err)
		}
		rows, err := q.PublishedClarificationHistory(
			ctx,
			dbgen.PublishedClarificationHistoryParams{
				EventID:       eventID,
				ID:            answerID,
				BeforeVersion: math.MaxInt64,
				PageSize:      21,
			},
		)
		if err != nil {
			return fmt.Errorf("read official answer history: %w", err)
		}
		if len(rows) == 0 {
			return access.ErrNotFound
		}
		out.HistoryHasMore = len(rows) > 20
		if out.HistoryHasMore {
			rows = rows[:20]
		}
		out.History = []PublishedAnswer{}
		for _, row := range rows {
			if now.Before(row.PublishedAt) {
				return access.ErrNotFound
			}
			item, itemErr := published(row, source(ref))
			if itemErr != nil {
				return itemErr
			}
			out.History = append(out.History, item)
			if row.Current {
				out.Answer = item
			}
		}
		if out.Answer.ID == uuid.Nil {
			return access.ErrNotFound
		}
		e, err := q.Event(ctx, eventID)
		if err != nil {
			return fmt.Errorf("read official answer archive state: %w", err)
		}
		out.Archived = !e.WritesAllowed
		return nil
	})
	if err != nil {
		return PublicPage{}, fmt.Errorf("Public: %w", err)
	}
	return out, nil
}

func (s Service) Search(
	ctx context.Context,
	actor, eventID uuid.UUID,
	search string,
) (PublicList, error) {
	return s.SearchPage(ctx, actor, eventID, search, uuid.Nil, 100)
}

func (s Service) SearchPage(
	ctx context.Context,
	actor, eventID uuid.UUID,
	search string,
	cursor uuid.UUID,
	limit int32,
) (PublicList, error) {
	search = strings.TrimSpace(search)
	if len(search) > 200 || strings.ContainsRune(search, 0) || limit < 1 || limit > 100 {
		return PublicList{}, access.ErrInvalid
	}
	var out PublicList
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		now := s.now()
		ref, err := rubric.ReleasedSourceForReaderInTransaction(ctx, tx, actor, eventID, now)
		if err != nil {
			return fmt.Errorf("Search: %w", err)
		}
		rows, err := q.PublishedClarifications(
			ctx,
			dbgen.PublishedClarificationsParams{
				EventID:  eventID,
				Search:   search,
				Cursor:   cursor,
				PageSize: limit + 1,
			},
		)
		if err != nil {
			return fmt.Errorf("search published answers: %w", err)
		}
		if len(rows) > int(limit) {
			next := rows[limit-1].ID
			out.NextCursor = &next
			rows = rows[:limit]
		}
		out.Items = []PublishedAnswer{}
		for _, row := range rows {
			if now.Before(row.PublishedAt) {
				return access.ErrNotFound
			}
			item, itemErr := published(dbgen.PublishedClarificationHistoryRow{
				ID:          row.ID,
				Version:     row.Version,
				Document:    row.Document,
				Source:      row.Source,
				AuthorName:  row.AuthorName,
				AuthoredAt:  row.AuthoredAt,
				ApprovedAt:  row.ApprovedAt,
				PublishedAt: row.PublishedAt,
				Current:     true,
			}, source(ref))
			if itemErr != nil {
				return itemErr
			}
			out.Items = append(out.Items, item)
		}
		return nil
	})
	if err != nil {
		return PublicList{}, fmt.Errorf("Search: %w", err)
	}
	return out, nil
}

// AuthorizeNotificationInTransaction retains historical published revisions as
// safe canonical links while requiring the original common audience's current
// confirmed status. Approval revocation never rewrites a completed publication.
func (s Service) AuthorizeNotificationInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, answerID uuid.UUID,
	version int64,
) error {
	if version < 1 {
		return access.ErrNotFound
	}
	q := dbgen.New(tx)
	_, confirmed, err := access.ContentReader(ctx, q, actor, eventID)
	if err != nil {
		return fmt.Errorf("AuthorizeNotificationInTransaction: %w", err)
	}
	if !confirmed {
		return access.ErrNotFound
	}
	now := s.now()
	if _, err = rubric.ReleasedSourceForReaderInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		now,
	); err != nil {
		return fmt.Errorf("AuthorizeNotificationInTransaction: %w", err)
	}
	p, err := q.ClarificationPublication(
		ctx,
		dbgen.ClarificationPublicationParams{
			EventID:  eventID,
			AnswerID: answerID,
			Version:  version,
		},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return access.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read published notification revision: %w", err)
	}
	if now.Before(p.PublishedAt) {
		return access.ErrNotFound
	}
	return nil
}
