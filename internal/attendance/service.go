package attendance

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
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

type Service struct {
	Pool *pgxpool.Pool
	Now  func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func lock(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID uuid.UUID,
	action access.Action,
) (dbgen.Event, error) {
	row, err := q.LockEvent(ctx, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, access.ErrNotFound
	}
	if err != nil {
		return row, fmt.Errorf("lock attendance event: %w", err)
	}
	if err := (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		action,
	); err != nil {
		return row, fmt.Errorf("authorize attendance: %w", err)
	}
	return row, nil
}

func configuration(row dbgen.Event) (event.Configuration, []string, error) {
	c := event.DefaultConfiguration()
	if err := json.Unmarshal(row.Configuration, &c); err != nil {
		return c, nil, fmt.Errorf("read attendance dates: %w", err)
	}
	days := []string{}
	if c.LiveStart.IsZero() || c.LiveEnd.IsZero() || !c.LiveEnd.After(c.LiveStart) {
		return c, days, nil
	}
	zone, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return c, nil, fmt.Errorf("attendance timezone: %w", access.ErrInvalid)
	}
	// Iterate calendar dates, not elapsed 24-hour periods. End is exclusive.
	start, _ := time.Parse(time.DateOnly, c.LiveStart.In(zone).Format(time.DateOnly))
	end, _ := time.Parse(
		time.DateOnly,
		c.LiveEnd.Add(-time.Nanosecond).In(zone).Format(time.DateOnly),
	)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if len(days) >= 366 {
			return c, nil, access.ErrInvalid
		}
		days = append(days, d.Format(time.DateOnly))
	}
	return c, days, nil
}

func chooseDay(
	requested string,
	days []string,
	timezone string,
	now time.Time,
) (string, pgtype.Date, error) {
	if requested == "" && len(days) == 0 {
		return "", pgtype.Date{}, nil
	}
	if requested == "" {
		zone, err := time.LoadLocation(timezone)
		if err != nil {
			return "", pgtype.Date{}, access.ErrInvalid
		}
		requested = now.In(zone).Format(time.DateOnly)
		if requested < days[0] {
			requested = days[0]
		}
		if requested > days[len(days)-1] {
			requested = days[len(days)-1]
		}
	}
	d, err := time.Parse(time.DateOnly, requested)
	if err != nil || !slices.Contains(days, requested) {
		return "", pgtype.Date{}, access.ErrInvalid
	}
	return requested, pgtype.Date{Time: d, Valid: true}, nil
}

func (s Service) Read(ctx context.Context, actor, eventID uuid.UUID, day string) (Page, error) {
	var result Page
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := lock(ctx, q, actor, eventID, access.RosterRead)
		if err != nil {
			return err
		}
		result, err = s.page(ctx, q, actor, row, day)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("read attendance roster: %w", err)
	}
	return result, nil
}

func (s Service) page(
	ctx context.Context,
	q *dbgen.Queries,
	actor uuid.UUID,
	row dbgen.Event,
	day string,
) (Page, error) {
	c, days, err := configuration(row)
	if err != nil {
		return Page{}, err
	}
	now := s.now()
	day, date, err := chooseDay(day, days, c.Timezone, now)
	if err != nil {
		return Page{}, err
	}
	roster, err := q.AttendanceRoster(
		ctx,
		dbgen.AttendanceRosterParams{EventID: row.ID, AttendanceDay: date},
	)
	if err != nil {
		return Page{}, fmt.Errorf("load attendance roster: %w", err)
	}
	staff, err := q.AttendanceStaff(ctx, row.ID)
	if err != nil {
		return Page{}, fmt.Errorf("load attendance staff: %w", err)
	}
	buffer, err := q.CateringBuffer(ctx, row.ID)
	if err != nil {
		return Page{}, fmt.Errorf("load catering buffer: %w", err)
	}
	policy := access.EventPolicy{Queries: q}
	result := Page{
		Revision:   row.Revision,
		Day:        day,
		Days:       days,
		Timezone:   c.Timezone,
		RosterAsOf: now,
		Items:      []Entry{},
		CanCheckIn: date.Valid &&
			policy.Authorize(ctx, actor, row.ID, row.ID, access.AttendanceManage) == nil,
		CanConfigure:     policy.Authorize(ctx, actor, row.ID, row.ID, access.EventEdit) == nil,
		CanReadLogistics: policy.Authorize(ctx, actor, row.ID, row.ID, access.ProfileExport) == nil,
		Counts: Counts{
			ConfirmedCompetitors: len(roster),
			MinimumCompetitors:   c.MinimumAttendance,
			Staff:                len(staff),
			CateringBuffer:       buffer,
		},
	}
	for _, p := range roster {
		entry := Entry{
			PersonID:    p.ID,
			DisplayName: p.DisplayName,
			Active:      p.Active,
			Present:     p.Present,
			Revision:    p.Revision,
		}
		if p.UpdatedAt.Valid {
			at := p.UpdatedAt.Time
			entry.UpdatedAt = &at
		}
		result.Items = append(result.Items, entry)
		if p.Present {
			result.Counts.CheckedInCompetitors++
		}
	}
	for _, p := range staff {
		if p.Competing {
			result.Counts.StaffAlsoCompeting++
		}
	}
	uniquePeople := len(roster) + len(staff) - result.Counts.StaffAlsoCompeting
	result.Counts.CateringTotal = uniquePeople + int(buffer)
	return result, nil
}

func (s Service) Configure(
	ctx context.Context,
	actor, eventID uuid.UUID,
	revision int64,
	buffer int32,
) (Page, error) {
	var result Page
	if buffer < 0 || buffer > 10000 {
		return result, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := lock(ctx, q, actor, eventID, access.EventEdit)
		if err != nil {
			return err
		}
		if row.Revision != revision {
			return access.ErrConflict
		}
		if err := q.SaveCateringBuffer(
			ctx,
			dbgen.SaveCateringBufferParams{EventID: eventID, Buffer: buffer},
		); err != nil {
			return fmt.Errorf("save catering buffer: %w", err)
		}
		row, err = q.BumpEventRevision(ctx, eventID)
		if err != nil {
			return fmt.Errorf("advance catering revision: %w", err)
		}
		if err := audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &eventID,
				TargetID: eventID,
				Action:   audit.CateringConfigured,
				Reason:   audit.OrganiserDecision,
			},
		); err != nil {
			return fmt.Errorf("audit catering setting: %w", err)
		}
		result, err = s.page(ctx, q, actor, row, "")
		return err
	})
	if err != nil {
		return result, fmt.Errorf("configure catering: %w", err)
	}
	return result, nil
}

func (s Service) Mark(
	ctx context.Context,
	actor, eventID, personID uuid.UUID,
	day string,
	expected int64,
	command Command,
) (Entry, error) {
	var result Entry
	if command.RequestID == uuid.Nil || personID == uuid.Nil || expected < 0 ||
		len(command.Reason) > 500 ||
		day == "" {
		return result, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := lock(ctx, q, actor, eventID, access.AttendanceManage)
		if err != nil {
			return err
		}
		c, days, err := configuration(row)
		if err != nil {
			return err
		}
		_, date, err := chooseDay(day, days, c.Timezone, s.now())
		if err != nil {
			return err
		}
		person, err := q.Person(ctx, personID)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read arrival person: %w", err)
		}
		previous, err := q.AttendanceRecord(
			ctx,
			dbgen.AttendanceRecordParams{EventID: eventID, PersonID: personID, AttendanceDay: date},
		)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read previous attendance: %w", err)
		}
		claim, err := q.CompetitionState(
			ctx,
			dbgen.CompetitionStateParams{EventID: eventID, PersonID: personID},
		)
		if err != nil {
			return fmt.Errorf("check confirmed place: %w", err)
		}
		if command.Present && (claim != "confirmed" || !person.Active) {
			return access.ErrForbidden
		}
		if claim != "confirmed" && previous.Revision == 0 {
			return access.ErrNotFound
		}
		replay, err := sameRequest(ctx, q, actor, eventID, personID, date, expected, command)
		if err != nil {
			return err
		}
		if !replay {
			previous, err = save(
				ctx,
				q,
				actor,
				eventID,
				personID,
				date,
				previous,
				expected,
				command,
			)
			if err != nil {
				return err
			}
		}
		result = Entry{
			PersonID:    personID,
			DisplayName: person.DisplayName,
			Active:      person.Active,
			Present:     previous.Present,
			Revision:    previous.Revision,
		}
		if previous.Revision > 0 {
			result.UpdatedAt = &previous.UpdatedAt
		}
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("record attendance: %w", err)
	}
	return result, nil
}

func sameRequest(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID, personID uuid.UUID,
	date pgtype.Date,
	expected int64,
	c Command,
) (bool, error) {
	previous, err := q.AttendanceRequest(
		ctx,
		dbgen.AttendanceRequestParams{EventID: eventID, RequestID: c.RequestID},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read arrival retry: %w", err)
	}
	if previous.ActorID != actor || previous.PersonID != personID ||
		!previous.AttendanceDay.Time.Equal(date.Time) ||
		previous.ExpectedRevision != expected ||
		previous.Present != c.Present ||
		previous.Reason != c.Reason {
		return false, access.ErrConflict
	}
	return true, nil
}

func save(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID, personID uuid.UUID,
	date pgtype.Date,
	previous dbgen.AttendanceRecord,
	expected int64,
	c Command,
) (dbgen.AttendanceRecord, error) {
	changed := previous.Present != c.Present
	// A new scan is a no-op when the person is already in the requested state.
	// A stale scan after a correction cannot change it back.
	if changed && previous.Revision != expected {
		return previous, access.ErrConflict
	}
	if changed && (previous.Revision > 0 || !c.Present) && strings.TrimSpace(c.Reason) == "" {
		return previous, &event.Blocked{
			Issues: []event.Issue{
				{
					Field:   "reason",
					Code:    "correction_reason",
					Message: "Record a reason for correcting attendance.",
				},
			},
		}
	}
	if changed {
		saved, err := q.SaveAttendance(
			ctx,
			dbgen.SaveAttendanceParams{
				EventID:       eventID,
				PersonID:      personID,
				AttendanceDay: date,
				Present:       c.Present,
			},
		)
		if err != nil {
			return previous, fmt.Errorf("save arrival: %w", err)
		}
		previous = saved
	}
	if err := q.RecordAttendanceChange(
		ctx,
		dbgen.RecordAttendanceChangeParams{
			EventID:          eventID,
			RequestID:        c.RequestID,
			PersonID:         personID,
			AttendanceDay:    date,
			Present:          c.Present,
			ExpectedRevision: expected,
			Revision:         previous.Revision,
			ActorID:          actor,
			Reason:           c.Reason,
			Changed:          changed,
		},
	); err != nil {
		return previous, fmt.Errorf("record arrival history: %w", err)
	}
	if changed {
		if err := audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &eventID,
				TargetID: personID,
				Action:   audit.AttendanceChanged,
				Reason:   audit.UserRequest,
				Metadata: audit.Metadata{ReferenceID: &c.RequestID, Revision: &previous.Revision},
			},
		); err != nil {
			return previous, fmt.Errorf("audit arrival: %w", err)
		}
	}
	return previous, nil
}

func (s Service) History(
	ctx context.Context,
	actor, eventID, personID uuid.UUID,
	day string,
) ([]Change, error) {
	result := []Change{}
	date, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return nil, access.ErrInvalid
	}
	err = database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, err := lock(ctx, q, actor, eventID, access.RosterRead); err != nil {
			return err
		}
		rows, err := q.AttendanceHistory(
			ctx,
			dbgen.AttendanceHistoryParams{
				EventID:       eventID,
				PersonID:      personID,
				AttendanceDay: pgtype.Date{Time: date, Valid: true},
			},
		)
		if err != nil {
			return fmt.Errorf("read arrival history: %w", err)
		}
		for _, r := range rows {
			result = append(
				result,
				Change{
					RequestID: r.RequestID,
					ActorID:   r.ActorID,
					Present:   r.Present,
					Revision:  r.Revision,
					Reason:    r.Reason,
					At:        r.CreatedAt,
				},
			)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("attendance history: %w", err)
	}
	return result, nil
}
