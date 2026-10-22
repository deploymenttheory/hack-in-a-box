package content

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
)

type Service struct {
	Storage files.Storage
	Pool    *pgxpool.Pool
	Clock   func() time.Time
}

func (s Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
}

func timestamp(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func timePointer(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func numberPointer(n pgtype.Int8) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func load(ctx context.Context, q *dbgen.Queries, eventID, id uuid.UUID) (dbgen.ContentItem, error) {
	item, err := q.ContentItem(ctx, dbgen.ContentItemParams{EventID: eventID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return item, access.ErrNotFound
	}
	if err != nil {
		return item, fmt.Errorf("load content: %w", err)
	}
	return item, nil
}

func document(
	ctx context.Context,
	q *dbgen.Queries,
	item dbgen.ContentItem,
	version int64,
) (Document, error) {
	var d Document
	row, err := q.ContentVersion(
		ctx,
		dbgen.ContentVersionParams{EventID: item.EventID, ContentID: item.ID, Version: version},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, access.ErrNotFound
	}
	if err != nil {
		return d, fmt.Errorf("load content version: %w", err)
	}
	if err = json.Unmarshal(row.Document, &d); err != nil {
		return d, fmt.Errorf("decode content version: %w", err)
	}
	return d, nil
}

func projection(
	ctx context.Context,
	q *dbgen.Queries,
	actor uuid.UUID,
	item dbgen.ContentItem,
	preview bool,
	version int64,
) (Page, error) {
	var result Page
	allowed, err := access.ContentVersionForReader(ctx, q, actor, item, preview)
	if err != nil {
		return result, fmt.Errorf("authorize content reader: %w", err)
	}
	if allowed == 0 {
		return result, access.ErrNotFound
	}
	roles, _, err := access.ContentReader(ctx, q, actor, item.EventID)
	if err != nil {
		return result, fmt.Errorf("authorize content reader: %w", err)
	}
	manage := slices.Contains(roles, "organiser")
	if version != 0 {
		if !preview || !manage {
			return result, access.ErrForbidden
		}
		allowed = version
	}
	doc, err := document(ctx, q, item, allowed)
	if err != nil {
		return result, err
	}
	result = Page{
		ID:              item.ID,
		Kind:            item.Kind,
		Revision:        item.Revision,
		Version:         allowed,
		State:           item.State,
		Document:        doc,
		ReleasedVersion: numberPointer(item.ReleasedVersion),
		ReleasedAt:      timePointer(item.ReleasedAt),
		CanManage:       manage,
	}
	result.AttachmentDetails, err = attachmentDetails(ctx, q, actor, item.EventID, doc.Attachments)
	if err != nil {
		return result, err
	}
	ack, ackErr := q.ContentAcknowledgement(
		ctx,
		dbgen.ContentAcknowledgementParams{
			EventID:   item.EventID,
			ContentID: item.ID,
			Version:   allowed,
			PersonID:  actor,
		},
	)
	if ackErr != nil && !errors.Is(ackErr, pgx.ErrNoRows) {
		return result, fmt.Errorf("read acknowledgement: %w", ackErr)
	}
	if ackErr == nil {
		result.AcknowledgedAt = &ack
	}

	prior, priorErr := q.ContentPriorAcknowledgement(
		ctx,
		dbgen.ContentPriorAcknowledgementParams{
			EventID:   item.EventID,
			ContentID: item.ID,
			PersonID:  actor,
			Version:   allowed,
		},
	)
	if priorErr != nil && !errors.Is(priorErr, pgx.ErrNoRows) {
		return result, fmt.Errorf("read prior personal acknowledgement: %w", priorErr)
	}
	if priorErr == nil {
		result.PreviousAcknowledgement = &Acknowledgement{
			Version: prior.Version,
			At:      prior.AcknowledgedAt,
		}
	}

	if !preview {
		result.State = "released"
		result.Revision = allowed
	}
	if preview && item.State == "withdrawn" {
		result.Message = "Future application access is removed. Previously downloaded copies cannot be recalled."
	}
	if preview && (item.State == "missed" || item.State == "blocked") {
		result.Message = "Publication was blocked. Review the history, save a corrected version and approve it again."
		if item.Kind == "challenge" && !item.ReleasedVersion.Valid {
			result.Message = "Publication did not occur. Postpone the hack to preserve the preparation week or cancel it."
		}
	}
	return result, nil
}

func (s Service) Read(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	preview bool,
	version int64,
) (Page, error) {
	var result Page
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(ctx, eventID); err != nil {
			return fmt.Errorf("lock content read: %w", err)
		}
		item, err := load(ctx, q, eventID, id)
		if err != nil {
			return err
		}
		result, err = projection(ctx, q, actor, item, preview, version)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("read content page: %w", err)
	}
	return result, nil
}

func (s Service) List(
	ctx context.Context,
	actor, eventID, cursor uuid.UUID,
	limit int32,
	preview bool,
) (List, error) {
	return s.Search(ctx, actor, eventID, cursor, limit, preview, "")
}

// Search evaluates the current released audience before matching any text or
// advancing a visible cursor. Private titles and drafts cannot affect results.
func (s Service) Search(
	ctx context.Context,
	actor, eventID, cursor uuid.UUID,
	limit int32,
	preview bool,
	search string,
) (List, error) {
	result := List{Items: []Page{}}
	if limit < 1 || limit > 100 || len(search) > 200 {
		return result, access.ErrInvalid
	}
	search = strings.ToLower(strings.TrimSpace(search))
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(ctx, eventID); err != nil {
			return fmt.Errorf("lock content listing: %w", err)
		}
		roles, confirmed, err := access.ContentReader(ctx, q, actor, eventID)
		if err != nil {
			return fmt.Errorf("authorize content reader: %w", err)
		}
		if (!confirmed && len(roles) == 0) ||
			(preview && !slices.Contains(roles, "organiser") && !slices.Contains(roles, "early_brief_access")) {
			return access.ErrForbidden
		}
		// Cursor advances only through visible items, avoiding hidden-title/count leaks.
		for {
			rows, err := q.ContentItems(
				ctx,
				dbgen.ContentItemsParams{EventID: eventID, Cursor: cursor, PageSize: 100},
			)
			if err != nil {
				return fmt.Errorf("list content: %w", err)
			}
			for _, item := range rows {
				cursor = item.ID
				page, err := projection(ctx, q, actor, item, preview, 0)
				if errors.Is(err, access.ErrNotFound) {
					continue
				}
				if err != nil {
					return err
				}
				if search != "" &&
					!strings.Contains(
						strings.ToLower(page.Document.Title+"\n"+page.Document.Markdown),
						search,
					) {
					continue
				}
				if len(result.Items) == int(limit) {
					id := result.Items[len(result.Items)-1].ID
					result.NextCursor = &id
					return nil
				}
				result.Items = append(result.Items, page)
			}
			if len(rows) < 100 {
				return nil
			}
		}
	})
	if err != nil {
		return result, fmt.Errorf("list available content: %w", err)
	}
	return result, nil
}

func saveState(ctx context.Context, q *dbgen.Queries, item dbgen.ContentItem) error {
	_, err := q.SaveContentState(
		ctx,
		dbgen.SaveContentStateParams{
			EventID:          item.EventID,
			ID:               item.ID,
			Revision:         item.Revision,
			Version:          item.Version,
			State:            item.State,
			ReleasedVersion:  item.ReleasedVersion,
			ScheduledAt:      item.ScheduledAt,
			ApprovedBy:       item.ApprovedBy,
			ApprovedRevision: item.ApprovedRevision,
			ReleasedAt:       item.ReleasedAt,
		},
	)
	if err != nil {
		return fmt.Errorf("save content state: %w", err)
	}
	return nil
}

func record(
	ctx context.Context,
	q *dbgen.Queries,
	item dbgen.ContentItem,
	actor *uuid.UUID,
	action, reason string,
	now time.Time,
) error {
	if err := q.RecordContentChange(
		ctx,
		dbgen.RecordContentChangeParams{
			EventID:     item.EventID,
			ContentID:   item.ID,
			Revision:    item.Revision,
			Version:     item.Version,
			Action:      action,
			ActorID:     actor,
			Reason:      reason,
			ScheduledAt: item.ScheduledAt,
			OccurredAt:  now,
		},
	); err != nil {
		return fmt.Errorf("record content change: %w", err)
	}
	a := audit.ContentChanged
	if action == "released" {
		a = audit.ContentReleased
	}
	if err := audit.Append(
		ctx,
		q,
		audit.Record{
			ActorID:  actor,
			EventID:  &item.EventID,
			TargetID: item.ID,
			Action:   a,
			Reason:   audit.OrganiserDecision,
			Metadata: audit.Metadata{Revision: &item.Revision},
		},
	); err != nil {
		return fmt.Errorf("audit content change: %w", err)
	}
	return nil
}
