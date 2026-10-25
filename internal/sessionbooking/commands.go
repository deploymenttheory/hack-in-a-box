package sessionbooking

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

type Command struct {
	SessionRevision  int64     `json:"session_revision"`
	RequestID        uuid.UUID `json:"request_id"`
	Action           string    `json:"action"`
	ExpectedRevision int64     `json:"expected_revision"`
}

func (s Service) Change(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	c Command,
) (Receipt, error) {
	var result Receipt
	fingerprint, err := commandFingerprint(c)
	if err != nil {
		return result, err
	}
	err = database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		st, err := load(ctx, tx, actor, eventID, id)
		if err != nil {
			return err
		}
		var existingActor, existingSession uuid.UUID
		var existingHash string
		var receipt []byte
		err = tx.QueryRow(ctx, `SELECT actor_id,session_id,fingerprint,result FROM session_commands WHERE event_id=$1 AND request_id=$2`, eventID, c.RequestID).
			Scan(&existingActor, &existingSession, &existingHash, &receipt)
		if err == nil {
			if existingActor != actor || existingSession != id || existingHash != fingerprint {
				return access.ErrConflict
			}
			return json.Unmarshal(receipt, &result)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return contextError("Change", err)
		}
		if !st.actorMayBook() {
			return access.ErrForbidden
		}
		if c.SessionRevision != st.revision {
			return access.ErrConflict
		}
		changes := map[uuid.UUID]bool{}
		if err = s.reconcile(ctx, tx, actor, st, changes); err != nil {
			return err
		}
		var state string
		var revision int64
		err = tx.QueryRow(ctx, `SELECT state,revision FROM session_bookings WHERE event_id=$1 AND session_id=$2 AND person_id=$3`, eventID, id, actor).
			Scan(&state, &revision)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return contextError("Change", err)
		}
		if revision != c.ExpectedRevision {
			return access.ErrConflict
		}
		switch c.Action {
		case "reserve":
			if s.now().Before(st.opens) || !s.now().Before(st.closes) {
				return access.ErrForbidden
			}
			if state == "booked" || state == "waitlisted" {
				return access.ErrConflict
			}
			var count int
			if err = tx.QueryRow(ctx, `SELECT count(*) FROM session_bookings WHERE event_id=$1 AND session_id=$2 AND state='booked'`, eventID, id).
				Scan(&count); err != nil {
				return contextError("Change", err)
			}
			state = "waitlisted"
			if count < st.definition.Capacity {
				state = "booked"
			}
			err = tx.QueryRow(ctx, `INSERT INTO session_bookings(event_id,session_id,person_id,state,calendar_sequence) VALUES($1,$2,$3,$4,$5+1) ON CONFLICT(event_id,session_id,person_id) DO UPDATE SET state=EXCLUDED.state,queue_sequence=nextval('session_bookings_queue_sequence_seq'),revision=session_bookings.revision+1,calendar_sequence=$5+session_bookings.revision+1,updated_at=now() RETURNING revision`, eventID, id, actor, state, st.sequence).
				Scan(&revision)
		case "cancel":
			if state != "booked" && state != "waitlisted" {
				return access.ErrConflict
			}
			err = tx.QueryRow(ctx, `UPDATE session_bookings SET state='cancelled',revision=revision+1,calendar_sequence=$4+revision+1,updated_at=now() WHERE event_id=$1 AND session_id=$2 AND person_id=$3 RETURNING revision`, eventID, id, actor, st.sequence).
				Scan(&revision)
			state = "cancelled"
		}
		if err != nil {
			return contextError("Change", err)
		}
		if err = history(ctx, tx, actor, eventID, id, &actor, c.Action, st.revision); err != nil {
			return err
		}
		if c.Action == "cancel" {
			changes[actor] = true
		}
		if err = s.reconcile(ctx, tx, actor, st, changes); err != nil {
			return err
		}
		if err = s.queueUpdates(ctx, tx, actor, st, changes); err != nil {
			return err
		}
		result = Receipt{
			RequestID: c.RequestID,
			SessionID: id,
			PersonID:  actor,
			Action:    c.Action,
			State:     state,
			Revision:  revision,
		}
		receipt, err = json.Marshal(result)
		if err != nil {
			return contextError("Change", err)
		}
		_, err = tx.Exec(
			ctx,
			`INSERT INTO session_commands(event_id,session_id,request_id,actor_id,fingerprint,result) VALUES($1,$2,$3,$4,$5,$6)`,
			eventID,
			id,
			c.RequestID,
			actor,
			fingerprint,
			receipt,
		)
		return contextError("Change", err)
	})
	return result, contextError("Change", err)
}

// reconcile is called only with the locked event and session. It never changes
// competition claims or team membership. Current ineligible waitlistees cannot
// consume a session place; existing eligible bookings survive capacity reduction.
func (s Service) reconcile(
	ctx context.Context,
	tx pgx.Tx,
	actor uuid.UUID,
	st scope,
	changes map[uuid.UUID]bool,
) error {
	rows, err := tx.Query(
		ctx,
		`SELECT b.person_id,b.state,p.active,COALESCE(c.state,'') FROM session_bookings b JOIN people p ON p.id=b.person_id LEFT JOIN competition_claims c ON c.event_id=b.event_id AND c.person_id=b.person_id WHERE b.event_id=$1 AND b.session_id=$2 AND b.state IN ('booked','waitlisted') ORDER BY b.queue_sequence`,
		st.row.ID,
		st.item.ID,
	)
	if err != nil {
		return contextError("reconcile", err)
	}
	type candidate struct {
		id     uuid.UUID
		state  string
		active bool
		claim  string
	}
	items := []candidate{}
	for rows.Next() {
		var v candidate
		if err = rows.Scan(&v.id, &v.state, &v.active, &v.claim); err != nil {
			rows.Close()
			return contextError("reconcile", err)
		}
		items = append(items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return contextError("reconcile", err)
	}
	booked := 0
	waiting := []uuid.UUID{}
	for _, v := range items {
		eligible := v.active && v.claim == "confirmed" && !st.cancelled
		if eligible {
			var audienceErr error
			eligible, audienceErr = eligibleAudience(ctx, tx, st, v.id)
			if audienceErr != nil {
				return audienceErr
			}
		}
		if !eligible {
			if _, err = tx.Exec(
				ctx,
				`UPDATE session_bookings SET state='ineligible',revision=revision+1,calendar_sequence=$4+revision+1,updated_at=now() WHERE event_id=$1 AND session_id=$2 AND person_id=$3`,
				st.row.ID,
				st.item.ID,
				v.id,
				st.sequence,
			); err != nil {
				return contextError("reconcile", err)
			}
			changes[v.id] = true
			if err = history(
				ctx,
				tx,
				actor,
				st.row.ID,
				st.item.ID,
				&v.id,
				"invalidate",
				st.revision,
			); err != nil {
				return err
			}
			continue
		}
		if v.state == "booked" {
			booked++
		} else {
			waiting = append(waiting, v.id)
		}
	}
	if !st.row.WritesAllowed || st.cancelled || st.needsReview || s.now().Before(st.opens) ||
		!s.now().Before(st.closes) {
		return nil
	}
	for _, person := range waiting {
		if booked >= st.definition.Capacity {
			break
		}
		if _, err = tx.Exec(
			ctx,
			`UPDATE session_bookings SET state='booked',revision=revision+1,calendar_sequence=$4+revision+1,updated_at=now() WHERE event_id=$1 AND session_id=$2 AND person_id=$3`,
			st.row.ID,
			st.item.ID,
			person,
			st.sequence,
		); err != nil {
			return contextError("reconcile", err)
		}
		if err = history(
			ctx,
			tx,
			actor,
			st.row.ID,
			st.item.ID,
			&person,
			"promote",
			st.revision,
		); err != nil {
			return err
		}
		booked++
	}
	return nil
}

func eligibleAudience(ctx context.Context, tx pgx.Tx, st scope, person uuid.UUID) (bool, error) {
	if slices.Contains(st.item.Audience, "all") || slices.Contains(st.item.Audience, "attendee") {
		return true, nil
	}
	roles, err := dbgen.New(tx).
		ActiveRoles(ctx, dbgen.ActiveRolesParams{EventID: st.row.ID, PersonID: person})
	if err != nil {
		return false, contextError("eligibleAudience", err)
	}
	for _, role := range roles {
		if slices.Contains(st.item.Audience, role) {
			return true, nil
		}
	}
	return false, nil
}

func commandFingerprint(c Command) (string, error) {
	if c.RequestID == uuid.Nil || c.SessionRevision < 1 ||
		(c.Action != "reserve" && c.Action != "cancel") ||
		c.ExpectedRevision < 0 {
		return "", access.ErrInvalid
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return "", contextError("command fingerprint", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func (st scope) actorMayBook() bool {
	return st.row.WritesAllowed && !st.cancelled && !st.needsReview && st.confirmed && st.audience
}
