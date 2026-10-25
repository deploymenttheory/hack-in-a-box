package sessionbooking

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
)

type scope struct {
	row                                                   dbgen.Event
	configuration                                         event.Configuration
	item                                                  event.AgendaItem
	definition                                            Definition
	revision, sequence                                    int64
	cancelled, organiser, attendance, confirmed, audience bool
	start, end, opens, closes                             time.Time
}

// Every mutation takes the event lock first, then the session row. This serializes
// competition withdrawal and session allocation without sharing their places.
func load(ctx context.Context, tx pgx.Tx, actor, eventID, sessionID uuid.UUID) (scope, error) {
	q := dbgen.New(tx)
	var result scope
	var err error
	result.row, err = q.LockEvent(ctx, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, access.ErrNotFound
	}
	if err != nil {
		return result, contextError("load", err)
	}
	if err = (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		access.EventRead,
	); err != nil {
		return result, contextError("load", err)
	}
	if err = json.Unmarshal(result.row.Configuration, &result.configuration); err != nil {
		return result, contextError("load", err)
	}
	for _, item := range result.configuration.Schedule {
		if item.ID == sessionID {
			result.item = item
			break
		}
	}
	if result.item.ID == uuid.Nil {
		return result, access.ErrNotFound
	}
	var data []byte
	err = tx.QueryRow(ctx, `SELECT definition,revision,calendar_sequence,cancelled FROM bookable_sessions WHERE event_id=$1 AND id=$2 FOR UPDATE`, eventID, sessionID).
		Scan(&data, &result.revision, &result.sequence, &result.cancelled)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, access.ErrNotFound
	}
	if err != nil {
		return result, contextError("load", err)
	}
	if err = json.Unmarshal(data, &result.definition); err != nil {
		return result, contextError("load", err)
	}
	roles, err := q.ActiveRoles(ctx, dbgen.ActiveRolesParams{EventID: eventID, PersonID: actor})
	if err != nil {
		return result, contextError("load", err)
	}
	result.organiser = slices.Contains(roles, "organiser")
	result.attendance = result.organiser || slices.Contains(roles, "logistics") ||
		slices.Contains(result.definition.FacilitatorIDs, actor)
	claim, err := q.CompetitionState(
		ctx,
		dbgen.CompetitionStateParams{EventID: eventID, PersonID: actor},
	)
	if err != nil {
		return result, contextError("load", err)
	}
	result.confirmed = claim == "confirmed"
	result.audience = slices.Contains(result.item.Audience, "all") ||
		(result.confirmed && slices.Contains(result.item.Audience, "attendee"))
	for _, role := range roles {
		result.audience = result.audience || slices.Contains(result.item.Audience, role)
	}
	if !result.audience && !result.attendance {
		return result, access.ErrNotFound
	}
	result.start, err = event.Plan(result.configuration, result.item)
	if err != nil {
		return result, contextError("load", err)
	}
	result.end = result.start.Add(time.Duration(result.definition.DurationMinutes) * time.Minute)
	result.opens, err = schedule.ResolveLocal(
		result.definition.BookingOpens.Wall,
		result.configuration.Timezone,
		result.definition.BookingOpens.DSTChoice,
	)
	if err != nil {
		return result, contextError("load", err)
	}
	result.closes, err = schedule.ResolveLocal(
		result.definition.BookingCloses.Wall,
		result.configuration.Timezone,
		result.definition.BookingCloses.DSTChoice,
	)
	return result, contextError("load", err)
}

func validate(c event.Configuration, item event.AgendaItem, d Definition) error {
	if d.Capacity < 1 || d.Capacity > 10000 || d.DurationMinutes < 1 || d.DurationMinutes > 1440 ||
		len(d.Room) > 200 ||
		len(d.MeetingURL) > 2000 ||
		len(d.FacilitatorIDs) > 50 {
		return access.ErrInvalid
	}
	if strings.ContainsAny(d.Room, "\r\n\x00") {
		return access.ErrInvalid
	}
	if d.MeetingURL != "" {
		u, err := url.Parse(d.MeetingURL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil ||
			strings.ContainsAny(d.MeetingURL, "\r\n\x00") {
			return access.ErrInvalid
		}
	}
	start, err := event.Plan(c, item)
	if err != nil {
		return contextError("validate", err)
	}
	opens, err := schedule.ResolveLocal(d.BookingOpens.Wall, c.Timezone, d.BookingOpens.DSTChoice)
	if err != nil {
		return contextError("validate", err)
	}
	closes, err := schedule.ResolveLocal(
		d.BookingCloses.Wall,
		c.Timezone,
		d.BookingCloses.DSTChoice,
	)
	if err != nil {
		return contextError("validate", err)
	}
	if !opens.Before(closes) || closes.After(start) {
		return access.ErrInvalid
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range d.FacilitatorIDs {
		if id == uuid.Nil || seen[id] {
			return access.ErrInvalid
		}
		seen[id] = true
	}
	return nil
}

// Configure attaches optional booking to an existing agenda item; ordinary agenda
// entries do not become bookable implicitly. Capacity reductions retain places.
func (s Service) Configure(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	expected int64,
	d Definition,
) (Page, error) {
	var result Page
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return access.ErrNotFound
		}
		if err = (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.EventEdit,
		); err != nil {
			return contextError("Configure", err)
		}
		var c event.Configuration
		if err = json.Unmarshal(row.Configuration, &c); err != nil {
			return contextError("Configure", err)
		}
		var item event.AgendaItem
		for _, v := range c.Schedule {
			if v.ID == id {
				item = v
				break
			}
		}
		if item.ID == uuid.Nil {
			return access.ErrNotFound
		}
		if err = validate(c, item, d); err != nil {
			return err
		}
		for _, person := range d.FacilitatorIDs {
			if err = (access.EventPolicy{Queries: q}).Authorize(
				ctx,
				person,
				eventID,
				eventID,
				access.EventRead,
			); err != nil {
				return access.ErrInvalid
			}
		}
		var current int64
		err = tx.QueryRow(ctx, `SELECT revision FROM bookable_sessions WHERE event_id=$1 AND id=$2 FOR UPDATE`, eventID, id).
			Scan(&current)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return contextError("Configure", err)
		}
		if current != expected {
			return access.ErrConflict
		}
		data, err := json.Marshal(d)
		if err != nil {
			return contextError("Configure", err)
		}
		tag, err := tx.Exec(
			ctx,
			`INSERT INTO bookable_sessions(event_id,id,definition) VALUES($1,$2,$3) ON CONFLICT(event_id,id) DO UPDATE SET definition=EXCLUDED.definition,revision=bookable_sessions.revision+1,calendar_sequence=bookable_sessions.calendar_sequence+1 WHERE bookable_sessions.revision=$4 AND NOT bookable_sessions.cancelled`,
			eventID,
			id,
			data,
			expected,
		)
		if err != nil {
			return contextError("Configure", err)
		}
		if tag.RowsAffected() != 1 {
			return access.ErrConflict
		}
		st, err := load(ctx, tx, actor, eventID, id)
		if err != nil {
			return err
		}
		if err = history(ctx, tx, actor, eventID, id, nil, "configure", st.revision); err != nil {
			return err
		}
		result, err = s.page(ctx, tx, actor, st)
		return err
	})
	return result, contextError("Configure", err)
}

func history(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, id uuid.UUID,
	person *uuid.UUID,
	action string,
	revision int64,
) error {
	_, err := tx.Exec(
		ctx,
		`INSERT INTO session_history(event_id,session_id,actor_id,person_id,action,session_revision) VALUES($1,$2,$3,$4,$5,$6)`,
		eventID,
		id,
		actor,
		person,
		action,
		revision,
	)
	return contextError("history", err)
}

func (s Service) Read(ctx context.Context, actor, eventID, id uuid.UUID) (Page, error) {
	var result Page
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		st, err := load(ctx, tx, actor, eventID, id)
		if err != nil {
			return err
		}
		result, err = s.page(ctx, tx, actor, st)
		return err
	})
	return result, contextError("Read", err)
}

func (s Service) page(ctx context.Context, tx pgx.Tx, actor uuid.UUID, st scope) (Page, error) {
	p := Page{
		ID:                  st.item.ID,
		Revision:            st.revision,
		CalendarSequence:    st.sequence,
		Title:               st.item.Title,
		Timezone:            st.configuration.Timezone,
		Start:               st.start,
		End:                 st.end,
		BookingOpens:        st.opens,
		BookingCloses:       st.closes,
		Capacity:            st.definition.Capacity,
		Cancelled:           st.cancelled,
		Room:                st.definition.Room,
		CanManage:           st.organiser && st.row.WritesAllowed,
		CanRecordAttendance: st.attendance && st.row.WritesAllowed,
		Conflicts:           []uuid.UUID{},
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM session_bookings WHERE event_id=$1 AND session_id=$2 AND state='booked'`, st.row.ID, st.item.ID).
		Scan(&p.Booked); err != nil {
		return p, contextError("page", err)
	}
	p.Available = max(0, p.Capacity-p.Booked)
	p.CapacityShortfall = max(0, p.Booked-p.Capacity)
	b := Booking{PersonID: actor}
	var position int64
	err := tx.QueryRow(ctx, `SELECT b.state,b.revision,(SELECT count(*) FROM session_bookings w WHERE w.event_id=b.event_id AND w.session_id=b.session_id AND w.state='waitlisted' AND w.queue_sequence<=b.queue_sequence) FROM session_bookings b WHERE event_id=$1 AND session_id=$2 AND person_id=$3`, st.row.ID, st.item.ID, actor).
		Scan(&b.State, &b.Revision, &position)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return p, contextError("page", err)
	}
	if err == nil {
		if b.State == "waitlisted" {
			b.WaitlistPosition = &position
		}
		p.OwnBooking = &b
	}
	if st.attendance ||
		(st.confirmed && st.audience && p.OwnBooking != nil && p.OwnBooking.State == "booked") {
		if st.definition.MeetingURL != "" {
			link := st.definition.MeetingURL
			p.MeetingURL = &link
		}
	}
	p.CanReserve = st.row.WritesAllowed && !st.cancelled && st.confirmed && st.audience &&
		!s.now().Before(st.opens) &&
		s.now().Before(st.closes) &&
		(p.OwnBooking == nil || p.OwnBooking.State == "cancelled" || p.OwnBooking.State == "ineligible")
	return p, nil
}
