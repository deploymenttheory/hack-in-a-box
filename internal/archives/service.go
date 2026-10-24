package archives

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func active(ctx context.Context, q *dbgen.Queries, actor uuid.UUID) error {
	p, err := q.Person(ctx, actor)
	if errors.Is(err, pgx.ErrNoRows) {
		return access.ErrForbidden
	}
	if err != nil {
		return fmt.Errorf("read archive identity: %w", err)
	}
	if !p.Active {
		return access.ErrForbidden
	}
	return nil
}

func archived(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID uuid.UUID,
	action access.Action,
) (dbgen.Event, error) {
	e, err := q.LockEvent(ctx, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, access.ErrNotFound
	}
	if err != nil {
		return e, fmt.Errorf("lock archive event: %w", err)
	}
	if err = (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		action,
	); err != nil {
		return e, wrap("authorise event", err)
	}
	if e.Stage != "archived" {
		return e, access.ErrConflict
	}
	return e, nil
}

func (s Service) summary(ctx context.Context, q *dbgen.Queries, e dbgen.Event) (Summary, error) {
	out := Summary{
		EventID:       e.ID,
		EventName:     e.Name,
		EventRevision: e.Revision,
		ObservedAt:    s.now(),
		Metrics:       []Metric{},
		Sources: []SourceAvailability{
			{
				Source: "actual_attendance",
				State:  "unavailable",
				Reason: "Attendance records are observations, not a complete physical attendance census. Missing or absent records do not establish how many people physically attended.",
			},
			{
				Source: "final_judging",
				State:  "unavailable",
				Reason: "Final judging data is unavailable; no assessment result is inferred.",
			},
			{
				Source: "published_results",
				State:  "unavailable",
				Reason: "Published results are unavailable; no rank or award is inferred.",
			},
		},
	}
	c, err := q.ArchiveParticipationCounts(ctx, e.ID)
	if err != nil {
		return out, fmt.Errorf("read native archive participation facts: %w", err)
	}
	state, reason := "available", "Native formal outcomes are retained; completed entries describe frozen submissions only."
	if c.MissingSubmissionOutcomes > 0 {
		state, reason = "incomplete", "Some native formal outcomes are absent. Missing records are unavailable, not open, not submitted, judged or completed."
	}
	out.Sources = append(
		out.Sources,
		SourceAvailability{Source: "formal_submissions", State: state, Reason: reason},
	)
	out.Metrics = []Metric{
		{
			Key:        "registrations",
			Value:      c.RegistrationRecords,
			Unit:       "registration_records",
			Source:     "registrations.kind=attendee",
			Definition: "All retained attendee application records, including withdrawal/reapplication; not unique people.",
		},
		{
			Key:        "registered_people",
			Value:      c.RegisteredPeople,
			Unit:       "people",
			Source:     "distinct registrations.person_id where kind=attendee",
			Definition: "Distinct people with a retained attendee registration record, regardless of current admission state.",
		},
		{
			Key:        "confirmed_attendees",
			Value:      c.ConfirmedAttendees,
			Unit:       "people",
			Source:     "competition_claims.state=confirmed",
			Definition: "Current confirmed competitors; waitlists, offers and staff are excluded.",
		},
		{
			Key:        "recorded_attendance_people",
			Value:      c.RecordedAttendance,
			Unit:       "people",
			Source:     "distinct attendance_records.person_id with present=true",
			Definition: "Distinct people with at least one currently recorded present day, across all recorded persons in this event. Zero means no recorded presence, not that nobody physically attended. The recorded cohort may include people without an attendee registration; no registration rate is implied.",
		},
		{
			Key:        "competition_entries",
			Value:      c.EntryRecords,
			Unit:       "entry_records",
			Source:     "competition_entries",
			Definition: "One native competition entry record per team/event, including withdrawn or unused entries.",
		},
		{
			Key:               "completed_entries",
			Value:             c.CompletedEntries,
			Unit:              "entry_records",
			Denominator:       &c.EntryRecords,
			DenominatorSource: "competition_entries",
			Source:            "active non-disbanded entry with native frozen submission outcome",
			Definition:        "A frozen formal submission, not a judged, awarded or adopted project. Unused/disbanded entries are excluded.",
		},
		{
			Key:               "not_submitted_entries",
			Value:             c.NotSubmittedEntries,
			Unit:              "entry_records",
			Denominator:       &c.EntryRecords,
			DenominatorSource: "competition_entries",
			Source:            "submission_states.outcome=not_submitted",
			Definition:        "Only explicitly recorded native not-submitted outcomes; missing records are not inferred.",
		},
		{
			Key:               "withdrawn_entries",
			Value:             c.WithdrawnEntries,
			Unit:              "entry_records",
			Denominator:       &c.EntryRecords,
			DenominatorSource: "competition_entries",
			Source:            "submission_states.outcome=withdrawn",
			Definition:        "Only native withdrawn outcomes; no submission or result is fabricated.",
		},
		{
			Key:               "unavailable_submission_outcomes",
			Value:             c.MissingSubmissionOutcomes,
			Unit:              "entry_records",
			Denominator:       &c.EntryRecords,
			DenominatorSource: "competition_entries",
			Source:            "competition entries without a native submission state",
			Definition:        "Unavailable formal outcomes, including never-held archives; not inferred not-submitted entries.",
		},
		{
			Key:               "unused_entries",
			Value:             c.UnusedEntries,
			Unit:              "entry_records",
			Denominator:       &c.EntryRecords,
			DenominatorSource: "competition_entries",
			Source:            "competition_entries.active=false or hack_teams.state=disbanded",
			Definition:        "Unused/unscored entry repositories; no transfer or awarded result is inferred.",
		},
	}
	return out, nil
}

func (s Service) view(
	ctx context.Context,
	q *dbgen.Queries,
	e dbgen.Event,
	actor uuid.UUID,
) (View, error) {
	out := View{
		EventID:       e.ID,
		EventName:     e.Name,
		EventRevision: e.Revision,
		ReadOnly:      true,
		SummaryState:  "restricted",
	}
	if e.CompletedAt.Valid {
		out.CompletedAt = &e.CompletedAt.Time
	}
	err := (access.EventPolicy{Queries: q}).Authorize(ctx, actor, e.ID, e.ID, access.StaffRead)
	if errors.Is(err, access.ErrForbidden) {
		return out, nil
	}
	if err != nil {
		return out, wrap("read summary", err)
	}
	value, err := s.summary(ctx, q, e)
	if err != nil {
		return out, wrap("read summary", err)
	}
	out.Summary = &value
	out.SummaryState = "available"
	return out, nil
}

func (s Service) Read(ctx context.Context, actor, eventID uuid.UUID) (View, error) {
	var out View
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		e, err := archived(ctx, q, actor, eventID, access.EventRead)
		if err != nil {
			return wrap("read native source", err)
		}
		out, err = s.view(ctx, q, e, actor)
		return wrap("read native source", err)
	})
	return out, wrap("read", err)
}

func (s Service) List(ctx context.Context, actor, after uuid.UUID, limit int32) (Page, error) {
	out := Page{Items: []View{}}
	if limit < 1 || limit > 100 {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if err := active(ctx, q, actor); err != nil {
			return wrap("read native source", err)
		}
		rows, err := q.ArchivedEventsForReader(
			ctx,
			dbgen.ArchivedEventsForReaderParams{
				ActorID:  actor,
				AfterID:  after,
				PageSize: limit + 1,
			},
		)
		if err != nil {
			return wrap("read native source", err)
		}
		more := len(rows) > int(limit)
		if more {
			rows = rows[:limit]
		}
		for _, row := range rows {
			e, err := archived(ctx, q, actor, row.ID, access.EventRead)
			if err != nil {
				return wrap("read native source", err)
			}
			item, err := s.view(ctx, q, e, actor)
			if err != nil {
				return wrap("read native source", err)
			}
			out.Items = append(out.Items, item)
		}
		if more {
			last := rows[len(rows)-1].ID
			out.NextCursor = &last
		}
		return nil
	})
	return out, wrap("list", err)
}

func (s Service) Reports(
	ctx context.Context,
	actor, after uuid.UUID,
	limit int32,
) (ReportPage, error) {
	out := ReportPage{
		Items:  []Summary{},
		Cohort: "Current-organiser archived events on this cursor page; no global total or cross-event unique-person count is implied.",
	}
	if limit < 1 || limit > 100 {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if err := active(ctx, q, actor); err != nil {
			return wrap("read native source", err)
		}
		rows, err := q.ArchivedReportingEvents(
			ctx,
			dbgen.ArchivedReportingEventsParams{
				ActorID:  actor,
				AfterID:  after,
				PageSize: limit + 1,
			},
		)
		if err != nil {
			return wrap("read native source", err)
		}
		more := len(rows) > int(limit)
		if more {
			rows = rows[:limit]
		}
		for _, row := range rows {
			e, err := archived(ctx, q, actor, row.ID, access.StaffRead)
			if err != nil {
				return wrap("read native source", err)
			}
			item, err := s.summary(ctx, q, e)
			if err != nil {
				return wrap("read native source", err)
			}
			out.Items = append(out.Items, item)
		}
		if more {
			last := rows[len(rows)-1].ID
			out.NextCursor = &last
		}
		return nil
	})
	return out, wrap("report", err)
}

func (s Service) ExportSummary(ctx context.Context, actor, eventID uuid.UUID) (Summary, error) {
	var out Summary
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		e, err := archived(ctx, q, actor, eventID, access.StaffRead)
		if err != nil {
			return wrap("read native source", err)
		}
		out, err = s.summary(ctx, q, e)
		if err != nil {
			return wrap("read native source", err)
		}
		count := len(out.Metrics)
		return audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &eventID,
				TargetID: eventID,
				Action:   audit.ArchiveExported,
				Reason:   audit.UserRequest,
				Metadata: audit.Metadata{Count: &count, Revision: &e.Revision},
			},
		)
	})
	return out, wrap("export summary", err)
}

func wrap(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("archive %s: %w", operation, err)
}
