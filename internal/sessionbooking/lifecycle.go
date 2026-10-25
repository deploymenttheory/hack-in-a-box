package sessionbooking

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

// ReconcileEventInTransaction is called by native event/registration owners after
// their mutation under the same event lock. It does not authorize a public write
// or modify competition facts. The actor may have just withdrawn; private reads
// still use their independent current-authority path.
func (s Service) ReconcileEventInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor uuid.UUID,
	row dbgen.Event,
	now time.Time,
) error {
	var configuration event.Configuration
	if err := json.Unmarshal(row.Configuration, &configuration); err != nil {
		return contextError("reconcile event configuration", err)
	}
	rows, err := tx.Query(
		ctx,
		`SELECT id FROM bookable_sessions WHERE event_id=$1 ORDER BY id FOR UPDATE`,
		row.ID,
	)
	if err != nil {
		return contextError("lock sessions", err)
	}
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return contextError("scan session", err)
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return contextError("read sessions", err)
	}
	clock := s
	clock.Now = func() time.Time { return now }
	for _, id := range ids {
		st, err := loadRecord(ctx, tx, actor, row.ID, id, false)
		if err != nil {
			return err
		}
		changes := map[uuid.UUID]bool{}
		if row.Stage == "archived" {
			if now.Before(st.end) {
				if err = clock.queueUpdates(ctx, tx, actor, st, nil); err != nil {
					return err
				}
			}
			continue
		}
		var item event.AgendaItem
		for _, candidate := range configuration.Schedule {
			if candidate.ID == id {
				item = candidate
				break
			}
		}
		if item.ID == uuid.Nil || row.Stage == "cancelled" {
			if err = clock.cancelIn(ctx, tx, actor, st); err != nil {
				return err
			}
			continue
		}
		old := st.source
		next, sourceErr := snapshotOf(configuration, item, st.definition)
		needsReview := sourceErr != nil || validate(configuration, item, st.definition) != nil
		if sourceErr != nil {
			next = old
			next.Item = item
		}
		before, _ := json.Marshal(old)
		after, err := json.Marshal(next)
		if err != nil {
			return contextError("encode changed source", err)
		}
		if string(before) != string(after) || needsReview != st.needsReview {
			_, err = tx.Exec(
				ctx,
				`UPDATE bookable_sessions SET source_snapshot=$3,needs_review=$4,revision=revision+1,calendar_sequence=calendar_sequence+1 WHERE event_id=$1 AND id=$2`,
				row.ID,
				id,
				after,
				needsReview,
			)
			if err != nil {
				return contextError("update changed source", err)
			}
			st, err = loadRecord(ctx, tx, actor, row.ID, id, false)
			if err != nil {
				return err
			}
			if err = history(
				ctx,
				tx,
				actor,
				row.ID,
				id,
				nil,
				"agenda_changed",
				st.revision,
			); err != nil {
				return err
			}
		}
		if now.Before(st.end) {
			if err = clock.reconcile(ctx, tx, actor, st, changes); err != nil {
				return err
			}
		}
		if err = clock.queueUpdates(ctx, tx, actor, st, changes); err != nil {
			return err
		}
	}
	return nil
}

func (s Service) cancelIn(ctx context.Context, tx pgx.Tx, actor uuid.UUID, st scope) error {
	if st.cancelled {
		return nil
	}
	changes := map[uuid.UUID]bool{}
	people, err := notificationRecipients(ctx, tx, st.row.ID, st.item.ID)
	if err != nil {
		return err
	}
	for _, person := range people {
		if person.State == "booked" || person.State == "waitlisted" {
			changes[person.PersonID] = true
		}
	}
	if _, err := tx.Exec(
		ctx,
		`UPDATE bookable_sessions SET cancelled=true,revision=revision+1,calendar_sequence=calendar_sequence+1 WHERE event_id=$1 AND id=$2`,
		st.row.ID,
		st.item.ID,
	); err != nil {
		return contextError("cancel session", err)
	}
	if _, err := tx.Exec(
		ctx,
		`UPDATE session_bookings SET state='cancelled',revision=revision+1,calendar_sequence=$3+revision+1,updated_at=now() WHERE event_id=$1 AND session_id=$2 AND state IN ('booked','waitlisted')`,
		st.row.ID,
		st.item.ID,
		st.sequence+1,
	); err != nil {
		return contextError("cancel session places", err)
	}
	if err := history(
		ctx,
		tx,
		actor,
		st.row.ID,
		st.item.ID,
		nil,
		"cancel_session",
		st.revision+1,
	); err != nil {
		return err
	}
	current, err := loadRecord(ctx, tx, actor, st.row.ID, st.item.ID, false)
	if err != nil {
		return err
	}
	return s.queueUpdates(ctx, tx, actor, current, changes)
}

func (s Service) CancelSession(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	expected int64,
) (Page, error) {
	var result Page
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		st, err := load(ctx, tx, actor, eventID, id)
		if err != nil {
			return err
		}
		if !st.organiser || !st.row.WritesAllowed {
			return access.ErrForbidden
		}
		if st.revision != expected {
			return access.ErrConflict
		}
		if err = s.cancelIn(ctx, tx, actor, st); err != nil {
			return err
		}
		st, err = load(ctx, tx, actor, eventID, id)
		if err != nil {
			return err
		}
		result, err = s.page(ctx, tx, actor, st)
		return err
	})
	return result, contextError("cancel session", err)
}
