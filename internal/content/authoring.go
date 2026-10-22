package content

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
)

type Attachment struct {
	ID          uuid.UUID `json:"id"`
	Filename    string    `json:"filename"`
	MediaType   string    `json:"media_type"`
	Size        int64     `json:"size"`
	CanDownload bool      `json:"can_download"`
}
type Readiness struct {
	PublishedAt *time.Time    `json:"published_at"`
	Blockers    []event.Issue `json:"blockers"`
}

func attachmentDetails(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID uuid.UUID,
	ids []uuid.UUID,
) ([]Attachment, error) {
	result := make([]Attachment, 0, len(ids))
	for _, id := range ids {
		file, err := q.File(ctx, dbgen.FileParams{EventID: eventID, ID: id})
		if err != nil {
			return nil, fmt.Errorf("read attachment description: %w", err)
		}
		permitted := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			id,
			access.FileRead,
		) == nil
		result = append(
			result,
			Attachment{
				ID:          id,
				Filename:    file.Filename,
				MediaType:   file.MediaType,
				Size:        file.SizeBytes,
				CanDownload: permitted,
			},
		)
	}
	return result, nil
}

func (s Service) verifyAttachments(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID uuid.UUID,
	d Document,
) error {
	if len(d.Attachments) == 0 {
		return nil
	}
	if s.Storage == nil {
		return blocked(
			"attachment_unavailable",
			"Attachment storage is unavailable. Try again before approving or releasing this version.",
		)
	}
	reader := files.Service{
		Pool:    s.Pool,
		Storage: s.Storage,
		Policy:  access.EventPolicy{Queries: q},
	}
	for _, id := range d.Attachments {
		if _, _, err := reader.Download(ctx, actor, eventID, id); err != nil {
			return blocked(
				"attachment_unavailable",
				"An attachment is missing, corrupt or no longer accessible. Restore or replace it before approving or releasing this version.",
			)
		}
	}
	return nil
}

// PublicationStatus exposes the same server-owned gate used by the due job;
// it grants no preview or release authority and never discloses challenge text.
func (s Service) PublicationStatus(
	ctx context.Context,
	actor, eventID uuid.UUID,
) (Readiness, error) {
	result := Readiness{Blockers: []event.Issue{}}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock content readiness: %w", err)
		}
		if err = (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.StaffRead,
		); err != nil {
			return fmt.Errorf("authorize content readiness: %w", err)
		}
		result.PublishedAt = timePointer(row.PublishedAt)
		if row.PublishedAt.Valid {
			return nil
		}
		result.Blockers, err = event.PublicationReadiness(ctx, tx, eventID)
		if err != nil {
			return fmt.Errorf("read publication gates: %w", err)
		}
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("read publication readiness: %w", err)
	}
	return result, nil
}
