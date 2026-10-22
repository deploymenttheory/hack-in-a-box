package rubric

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// NotificationSource identifies the actual atomic common-challenge publication,
// including corrections, without copying criteria or private judge guidance.
type NotificationSource struct {
	ChallengeID      uuid.UUID
	ChallengeVersion int64
	PublishedAt      time.Time
	Correction       bool
}

// There is one current common challenge per event. Superseded publications are
// retained for history but are never new unattended dispatch sources.
func (s Service) DiscoverNotificationSourcesInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	requested *uuid.UUID,
) ([]NotificationSource, error) {
	q := dbgen.New(tx)
	if _, err := q.LockEvent(ctx, eventID); err != nil {
		return nil, fmt.Errorf("lock rubric notification sources: %w", err)
	}
	rows, err := tx.Query(
		ctx,
		`SELECT p.challenge_id,p.challenge_version,p.published_at,(p.reviewed_notice<>'') FROM rubric_publications p JOIN content_items c ON c.event_id=p.event_id AND c.id=p.challenge_id JOIN events e ON e.id=p.event_id WHERE p.event_id=$1 AND ($2::uuid IS NULL OR p.challenge_id=$2) AND c.kind='challenge' AND c.state<>'withdrawn' AND c.released_version=p.challenge_version AND e.published_at IS NOT NULL AND (p.reviewed_notice<>'' OR NOT EXISTS(SELECT 1 FROM rubric_publications older WHERE older.event_id=p.event_id AND (older.published_at,older.challenge_id,older.challenge_version)<(p.published_at,p.challenge_id,p.challenge_version))) ORDER BY p.challenge_id LIMIT 1`,
		eventID,
		requested,
	)
	if err != nil {
		return nil, fmt.Errorf("read current rubric publication metadata: %w", err)
	}
	defer rows.Close()
	result := []NotificationSource{}
	for rows.Next() {
		var source NotificationSource
		if err = rows.Scan(
			&source.ChallengeID,
			&source.ChallengeVersion,
			&source.PublishedAt,
			&source.Correction,
		); err != nil {
			return nil, fmt.Errorf("read rubric source metadata: %w", err)
		}
		result = append(result, source)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rubric publication metadata: %w", err)
	}
	return result, nil
}

func (s Service) ResolveNotificationAudienceInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, challengeID uuid.UUID,
	version int64,
) ([]uuid.UUID, error) {
	sources, err := s.DiscoverNotificationSourcesInTransaction(ctx, tx, eventID, &challengeID)
	if err != nil {
		return nil, err
	}
	if len(sources) != 1 || sources[0].ChallengeVersion != version {
		return nil, access.ErrNotFound
	}
	rows, err := tx.Query(
		ctx,
		`SELECT c.person_id FROM competition_claims c JOIN people p ON p.id=c.person_id AND p.active WHERE c.event_id=$1 AND c.state='confirmed' AND (NOT $2::boolean OR EXISTS(SELECT 1 FROM rubric_notice_recipients n WHERE n.event_id=c.event_id AND n.challenge_id=$3 AND n.challenge_version=$4 AND n.person_id=c.person_id)) ORDER BY c.person_id`,
		eventID,
		sources[0].Correction,
		challengeID,
		version,
	)
	if err != nil {
		return nil, fmt.Errorf("read current rubric recipients: %w", err)
	}
	people := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("read rubric recipient identity: %w", err)
		}
		people = append(people, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("iterate rubric recipients: %w", err)
	}
	result := []uuid.UUID{}
	for _, person := range people {
		// Reuse the public reader's full exact reference/digest/actual-release
		// policy; metadata existence alone never authorizes a recipient.
		released, readErr := ReleasedSourceForReaderInTransaction(ctx, tx, person, eventID, s.now())
 err=readErr
 if err != nil {
			if errors.Is(err, access.ErrNotFound) || errors.Is(err, access.ErrForbidden) {
				continue
			}
			return nil, err
		}
		if released.ChallengeID != challengeID || released.ChallengeVersion != version { return nil, access.ErrConflict }
		result = append(result, person)
	}
	return result, nil
}
