package results

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// NotificationSource is one durable explicitly released event. No private
// decision, ballot, raw card or prose enters communication source discovery.
type NotificationSource struct {
	ID          uuid.UUID
	Version     int64
	PublishedAt time.Time
}

func (s Service) DiscoverNotificationSourcesPageInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	requested *uuid.UUID,
	after uuid.UUID,
	limit int32,
) ([]NotificationSource, *uuid.UUID, error) {
	if limit < 1 || limit > 1000 {
		return nil, nil, access.ErrInvalid
	}
	if _, err := dbgen.New(tx).LockEvent(ctx, eventID); err != nil {
		return nil, nil, failure("notification event", err)
	}
	rows, err := tx.Query(
		ctx,
		"SELECT p.id,p.result_version,p.published_at FROM result_publications p WHERE p.event_id=$1 AND ($2::uuid IS NULL OR p.id=$2) AND p.id>$3 AND p.published_at<=$4 AND NOT EXISTS(SELECT 1 FROM result_publications newer WHERE newer.event_id=p.event_id AND newer.kind=p.kind AND newer.result_version>p.result_version AND newer.published_at<=$4) ORDER BY p.id LIMIT $5",
		eventID,
		requested,
		after,
		s.now(),
		limit+1,
	)
	if err != nil {
		return nil, nil, failure("notification sources", err)
	}
	defer rows.Close()
	out := []NotificationSource{}
	for rows.Next() {
		var source NotificationSource
		if err = rows.Scan(&source.ID, &source.Version, &source.PublishedAt); err != nil {
			return nil, nil, failure("notification source rows", err)
		}
		out = append(out, source)
	}
	if err = rows.Err(); err != nil {
		return nil, nil, failure("notification source rows", err)
	}
	var next *uuid.UUID
	if len(out) > int(limit) {
		out = out[:limit]
		last := out[len(out)-1].ID
		next = &last
	}
	return out, next, nil
}

func (s Service) ResolveNotificationAudienceInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, publicationID uuid.UUID,
	version int64,
) ([]uuid.UUID, error) {
	q := dbgen.New(tx)
	if _, err := q.LockEvent(ctx, eventID); err != nil {
		return nil, failure("notification audience event", err)
	}
	var exists bool
	err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM result_publications p WHERE p.event_id=$1 AND p.id=$2 AND p.result_version=$3 AND p.published_at<=$4 AND NOT EXISTS(SELECT 1 FROM result_publications newer WHERE newer.event_id=p.event_id AND newer.kind=p.kind AND newer.result_version>p.result_version AND newer.published_at<=$4))", eventID, publicationID, version, s.now()).
		Scan(&exists)
	if err != nil {
		return nil, failure("notification audience rows", err)
	}
	if !exists {
		return nil, access.ErrNotFound
	}
	rows, err := tx.Query(
		ctx,
		"SELECT p.id FROM people p WHERE p.active AND (EXISTS(SELECT 1 FROM competition_claims c WHERE c.event_id=$1 AND c.person_id=p.id AND c.state='confirmed') OR EXISTS(SELECT 1 FROM event_memberships m WHERE m.event_id=$1 AND m.person_id=p.id AND m.active AND m.role IN ('organiser','mentor','judge','mc'))) ORDER BY p.id",
		eventID,
	)
	if err != nil {
		return nil, failure("notification audience rows", err)
	}
	out := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, failure("notification audience rows", err)
		}
		out = append(out, id)
	}
	err = rows.Err()
	rows.Close()
	return out, failure("notification audience iteration", err)
}
