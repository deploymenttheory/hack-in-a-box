package clarification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

// NotificationSource exposes an actual current common-answer publication only.
// It deliberately has no originating question, private text or attachment field.
type NotificationSource struct {
	AnswerID    uuid.UUID
	Version     int64
	PublishedAt time.Time
}

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
	if _, err := dbgen.New(tx).LockEvent(ctx, eventID); err != nil {
		return nil, nil, fmt.Errorf("lock common-answer sources: %w", err)
	}
	rows, err := tx.Query(
		ctx,
		`SELECT a.id,p.version,p.published_at FROM clarification_answers a JOIN clarification_publications p ON p.event_id=a.event_id AND p.answer_id=a.id AND p.version=a.published_version WHERE a.event_id=$1 AND ($2::uuid IS NULL OR a.id=$2) AND a.id>$3 AND p.published_at<=$4 ORDER BY a.id LIMIT $5`,
		eventID,
		requestedID,
		after,
		s.now(),
		limit+1,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("page common-answer publication metadata: %w", err)
	}
	defer rows.Close()
	result := []NotificationSource{}
	for rows.Next() {
		var item NotificationSource
		if err = rows.Scan(&item.AnswerID, &item.Version, &item.PublishedAt); err != nil {
			return nil, nil, fmt.Errorf("read common-answer source metadata: %w", err)
		}
		result = append(result, item)
	}
	if err = rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate common-answer metadata: %w", err)
	}
	var next *uuid.UUID
	if len(result) > int(limit) {
		result = result[:limit]
		id := result[len(result)-1].AnswerID
		next = &id
	}
	return result, next, nil
}

// ResolveNotificationAudienceInTransaction authorizes new dispatch, unlike the
// independent retained historical-link authorizer. A correction or owning
// challenge/rubric change invalidates old queued source authority.
func (s Service) ResolveNotificationAudienceInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, answerID uuid.UUID,
	version int64,
) ([]uuid.UUID, error) {
	if version < 1 {
		return nil, access.ErrNotFound
	}
	if _, err := dbgen.New(tx).LockEvent(ctx, eventID); err != nil {
		return nil, fmt.Errorf("lock common-answer notification audience: %w", err)
	}
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT v.source FROM clarification_answers a JOIN clarification_versions v ON v.event_id=a.event_id AND v.answer_id=a.id AND v.version=a.published_version JOIN clarification_publications p ON p.event_id=v.event_id AND p.answer_id=v.answer_id AND p.version=v.version WHERE a.event_id=$1 AND a.id=$2 AND a.published_version=$3 AND p.published_at<=$4`, eventID, answerID, version, s.now()).
		Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, access.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read exact current common-answer source: %w", err)
	}
	var bound Source
	if err = json.Unmarshal(raw, &bound); err != nil {
		return nil, fmt.Errorf("decode common-answer source metadata: %w", err)
	}
	rows, err := tx.Query(
		ctx,
		`SELECT c.person_id FROM competition_claims c JOIN people p ON p.id=c.person_id AND p.active WHERE c.event_id=$1 AND c.state='confirmed' ORDER BY c.person_id`,
		eventID,
	)
	if err != nil {
		return nil, fmt.Errorf("read current common-answer recipient identities: %w", err)
	}
	people := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("read common-answer recipient: %w", err)
		}
		people = append(people, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("iterate common-answer recipients: %w", err)
	}
	result := []uuid.UUID{}
	for _, person := range people {
		current, readErr := rubric.ReleasedSourceForReaderInTransaction(
			ctx,
			tx,
			person,
			eventID,
			s.now(),
		)
		if errors.Is(readErr, access.ErrNotFound) || errors.Is(readErr, access.ErrForbidden) {
			continue
		}
		if readErr != nil {
			return nil, fmt.Errorf("authorize current common-answer source reader: %w", readErr)
		}
		if source(current) != bound {
			return nil, access.ErrConflict
		}
		result = append(result, person)
	}
	return result, nil
}
