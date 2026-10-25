package sessionbooking

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// Roster is scoped to this session. It deliberately excludes contact details,
// logistics preferences, other sessions and all competition assessment data.
func (s Service) Roster(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
) ([]AttendanceEntry, error) {
	result := []AttendanceEntry{}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		st, err := load(ctx, tx, actor, eventID, id)
		if err != nil {
			return err
		}
		if !st.attendance {
			return access.ErrForbidden
		}
		rows, err := tx.Query(
			ctx,
			`SELECT b.person_id,p.display_name,COALESCE(a.present,false),COALESCE(a.revision,0) FROM session_bookings b JOIN people p ON p.id=b.person_id AND p.active JOIN competition_claims c ON c.event_id=b.event_id AND c.person_id=b.person_id AND c.state='confirmed' LEFT JOIN session_attendance a ON a.event_id=b.event_id AND a.session_id=b.session_id AND a.person_id=b.person_id WHERE b.event_id=$1 AND b.session_id=$2 AND b.state='booked' ORDER BY p.display_name,b.person_id LIMIT 10000`,
			eventID,
			id,
		)
		if err != nil {
			return contextError("Roster", err)
		}
		defer rows.Close()
		for rows.Next() {
			var entry AttendanceEntry
			if err = rows.Scan(
				&entry.PersonID,
				&entry.DisplayName,
				&entry.Present,
				&entry.Revision,
			); err != nil {
				return contextError("Roster", err)
			}
			result = append(result, entry)
		}
		if err = rows.Err(); err != nil {
			return contextError("read roster", err)
		}
		rows.Close()
		visible := result[:0]
		for _, entry := range result {
			eligible, err := eligibleAudience(ctx, tx, st, entry.PersonID)
			if err != nil {
				return err
			}
			if eligible {
				visible = append(visible, entry)
			}
		}
		result = visible
		return nil
	})
	return result, contextError("Roster", err)
}

type AttendanceCommand struct {
	RequestID uuid.UUID `json:"request_id"`
	Present   bool      `json:"present"`
}

func (s Service) Mark(
	ctx context.Context,
	actor, eventID, id, person uuid.UUID,
	expected int64,
	c AttendanceCommand,
) (Receipt, error) {
	var result Receipt
	if c.RequestID == uuid.Nil || expected < 0 {
		return result, access.ErrInvalid
	}
	raw, err := json.Marshal(struct {
		Person   uuid.UUID         `json:"person_id"`
		Expected int64             `json:"expected_revision"`
		Command  AttendanceCommand `json:"command"`
	}{person, expected, c})
	if err != nil {
		return result, contextError("Mark", err)
	}
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	err = database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		st, err := load(ctx, tx, actor, eventID, id)
		if err != nil {
			return err
		}
		if !st.attendance {
			return access.ErrForbidden
		}
		var oldActor, oldSession uuid.UUID
		var oldHash string
		var rawReceipt []byte
		err = tx.QueryRow(ctx, `SELECT actor_id,session_id,fingerprint,result FROM session_commands WHERE event_id=$1 AND request_id=$2`, eventID, c.RequestID).
			Scan(&oldActor, &oldSession, &oldHash, &rawReceipt)
		if err == nil {
			if oldActor != actor || oldSession != id || oldHash != hash {
				return access.ErrConflict
			}
			return json.Unmarshal(rawReceipt, &result)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return contextError("Mark", err)
		}
		if !st.row.WritesAllowed || st.cancelled || st.needsReview || s.now().Before(st.start) {
			return access.ErrForbidden
		}
		var eligible bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM session_bookings b JOIN people p ON p.id=b.person_id AND p.active JOIN competition_claims c ON c.event_id=b.event_id AND c.person_id=b.person_id AND c.state='confirmed' WHERE b.event_id=$1 AND b.session_id=$2 AND b.person_id=$3 AND b.state='booked')`, eventID, id, person).
			Scan(&eligible); err != nil {
			return contextError("Mark", err)
		}
		if !eligible {
			return access.ErrForbidden
		}
		eligible, err = eligibleAudience(ctx, tx, st, person)
		if err != nil {
			return err
		}
		if !eligible {
			return access.ErrForbidden
		}
		var revision int64
		err = tx.QueryRow(ctx, `SELECT revision FROM session_attendance WHERE event_id=$1 AND session_id=$2 AND person_id=$3`, eventID, id, person).
			Scan(&revision)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return contextError("Mark", err)
		}
		if revision != expected {
			return access.ErrConflict
		}
		err = tx.QueryRow(ctx, `INSERT INTO session_attendance(event_id,session_id,person_id,present) VALUES($1,$2,$3,$4) ON CONFLICT(event_id,session_id,person_id) DO UPDATE SET present=EXCLUDED.present,revision=session_attendance.revision+1 RETURNING revision`, eventID, id, person, c.Present).
			Scan(&revision)
		if err != nil {
			return contextError("Mark", err)
		}
		state := "absent"
		if c.Present {
			state = "present"
		}
		result = Receipt{
			RequestID: c.RequestID,
			SessionID: id,
			PersonID:  person,
			Action:    "attendance",
			State:     state,
			Revision:  revision,
		}
		if err = history(
			ctx,
			tx,
			actor,
			eventID,
			id,
			&person,
			"attendance_"+state,
			st.revision,
		); err != nil {
			return err
		}
		rawReceipt, err = json.Marshal(result)
		if err != nil {
			return contextError("Mark", err)
		}
		_, err = tx.Exec(
			ctx,
			`INSERT INTO session_commands(event_id,session_id,request_id,actor_id,fingerprint,result) VALUES($1,$2,$3,$4,$5,$6)`,
			eventID,
			id,
			c.RequestID,
			actor,
			hash,
			rawReceipt,
		)
		return contextError("Mark", err)
	})
	return result, contextError("Mark", err)
}

// OriginalReceipt is a retained acknowledgement, not a claim based on a fresh
// current-state GET. Current actor/session authority is checked before disclosure.
func (s Service) OriginalReceipt(
	ctx context.Context,
	actor, eventID, id, request uuid.UUID,
) (Receipt, error) {
	var result Receipt
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		if _, err := load(ctx, tx, actor, eventID, id); err != nil {
			return err
		}
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT result FROM session_commands WHERE event_id=$1 AND session_id=$2 AND request_id=$3 AND actor_id=$4`, eventID, id, request, actor).
			Scan(&raw); err != nil {
			return access.ErrNotFound
		}
		return json.Unmarshal(raw, &result)
	})
	return result, contextError("OriginalReceipt", err)
}
