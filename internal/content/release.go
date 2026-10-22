package content

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
)

func (s Service) release(
	ctx context.Context,
	tx pgx.Tx,
	row dbgen.Event,
	item *dbgen.ContentItem,
	actor *uuid.UUID,
) error {
	q := dbgen.New(tx)
	if item.State != "approved" || item.ApprovedBy == nil || !item.ApprovedRevision.Valid ||
		item.ApprovedRevision.Int64 != item.Revision {
		return access.ErrConflict
	}
	if !row.WritesAllowed || row.Stage == "cancelled" || row.Stage == "archived" {
		return access.ErrForbidden
	}
	roles, _, err := access.ContentReader(ctx, q, *item.ApprovedBy, row.ID)
	if err != nil {
		return fmt.Errorf("authorize content reader: %w", err)
	}
	if !slices.Contains(roles, "organiser") {
		return access.ErrForbidden
	}
	now := s.now()
	if item.ScheduledAt.Valid && now.Before(item.ScheduledAt.Time) {
		return access.ErrConflict
	}
	if item.Kind == "challenge" && !row.PublishedAt.Valid {
		if err = s.challengeGate(ctx, tx, row, *item, now); err != nil {
			return err
		}
	}
	if item.Kind == "brief" && !row.PublishedAt.Valid {
		return blocked(
			"challenge_embargo",
			"Briefs remain privileged until the common challenge publication.",
		)
	}
	doc, err := document(ctx, q, *item, item.Version)
	if err != nil {
		return err
	}
	if err = s.verifyAttachments(ctx, q, *item.ApprovedBy, row.ID, doc); err != nil {
		return err
	}
	item.State = "released"
	item.ReleasedVersion = pgtype.Int8{Int64: item.Version, Valid: true}
	item.ReleasedAt = timestamp(&now)
	item.Revision++
	if err = saveState(ctx, q, *item); err != nil {
		return err
	}
	if item.Kind == "challenge" && !row.PublishedAt.Valid {
		if err = q.PublishChallenge(
			ctx,
			dbgen.PublishChallengeParams{ID: row.ID, PublishedAt: timestamp(&now)},
		); err != nil {
			return fmt.Errorf("record common challenge publication: %w", err)
		}
	}
	return record(ctx, q, *item, actor, "released", "", now)
}

func (s Service) challengeGate(
	ctx context.Context,
	tx pgx.Tx,
	row dbgen.Event,
	item dbgen.ContentItem,
	now time.Time,
) error {
	if !item.ScheduledAt.Valid {
		return blocked("common_publication", "The initial challenge requires its common schedule.")
	}
	doc, err := document(ctx, dbgen.New(tx), item, item.Version)
	if err != nil {
		return err
	}
	if err = canonicalSchedule(row, &doc, item.Kind); err != nil {
		return err
	}
	if now.After(item.ScheduledAt.Time.Add(schedule.ChallengeDispatchTolerance)) {
		return blocked(
			"missed_publication",
			"The challenge publication window was missed. Postpone the hack to preserve the full preparation week or cancel it.",
		)
	}
	issues, err := event.PublicationReadiness(ctx, tx, row.ID)
	if err != nil {
		return fmt.Errorf("check publication gates: %w", err)
	}
	if len(issues) > 0 {
		return &event.Blocked{Issues: issues}
	}
	return nil
}

// Dispatch is a version-bound durable job. Old selections, duplicate workers,
// cancel/edit races and already released jobs become no-ops under the event lock.
func (s Service) Dispatch(
	ctx context.Context,
	eventID, id uuid.UUID,
	approvedRevision int64,
) error {
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock content publication: %w", err)
		}
		item, err := load(ctx, q, eventID, id)
		if err != nil {
			return err
		}
		if item.State != "approved" || item.Revision != approvedRevision ||
			!item.ScheduledAt.Valid ||
			s.now().Before(item.ScheduledAt.Time) {
			return nil
		}
		err = s.release(ctx, tx, row, &item, nil)
		if err == nil {
			return nil
		}
		var gate *event.Blocked
		if !errors.As(err, &gate) && !errors.Is(err, access.ErrForbidden) {
			return err
		}
		item.State = "blocked"
		if item.Kind == "challenge" && !row.PublishedAt.Valid &&
			s.now().After(item.ScheduledAt.Time.Add(schedule.ChallengeDispatchTolerance)) {
			item.State = "missed"
		}
		item.Revision++
		item.ApprovedBy = nil
		item.ApprovedRevision = pgtype.Int8{}
		if saveErr := saveState(ctx, q, item); saveErr != nil {
			return saveErr
		}
		// Only domain diagnostics, never page text or credentials, enter this history.
		return record(ctx, q, item, nil, item.State, err.Error(), s.now())
	})
	if err != nil {
		return fmt.Errorf("dispatch content publication: %w", err)
	}
	return nil
}

func (s Service) Maintain(ctx context.Context) error {
	rows, err := dbgen.New(s.Pool).DueContent(ctx, timestampValue(s.now()))
	if err != nil {
		return fmt.Errorf("find due content: %w", err)
	}
	for _, item := range rows {
		if err = s.Dispatch(ctx, item.EventID, item.ID, item.Revision); err != nil {
			return err
		}
	}
	return nil
}
func timestampValue(t time.Time) pgtype.Timestamptz { return timestamp(&t) }
