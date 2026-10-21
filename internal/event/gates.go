package event

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
)

func gateBlockers(
	row dbgen.Event,
	c Configuration,
	f dbgen.EventGateFact,
	count int64,
	to string,
	now time.Time,
	exception bool,
	reason string,
) []Issue {
	issues := []Issue{}
	add := func(field, code, message string) {
		issues = append(issues, Issue{Field: field, Code: code, Message: message})
	}
	if to == "registration" {
		issues = Validate(Input{Name: row.Name, Slug: row.Slug, Configuration: c}, true)
		if !c.LiveStart.IsZero() {
			if publication, err := schedule.ChallengePublication(
				c.LiveStart,
				c.Timezone,
				c.DSTChoice,
			); err == nil &&
				!publication.After(now) {
				add(
					"live_start",
					"preserve_preparation_week",
					"Choose dates that allow the full preparation week before opening registration.",
				)
			}
		}
		return issues
	}
	if to == "preparation" || to == "publication" || to == "live" {
		if !f.AttendanceReady {
			add(
				"minimum_attendance",
				"attendance_unavailable",
				"Registration must supply verified confirmed-attendee facts.",
			)
		}
		attendanceWaived := to == "live" && row.PublishedAt.Valid && exception && reason != ""
		if count < int64(c.MinimumAttendance) && !attendanceWaived {
			add(
				"minimum_attendance",
				"attendance_shortfall",
				"Confirmed competitors are below the configured minimum. Waitlisted people, offers and staff do not count.",
			)
		}
	}
	if to == "publication" || to == "live" {
		if !f.TeamsReady || f.TotalTeams == 0 || f.FrozenTeams != f.TotalTeams ||
			int64(f.AssignedAttendees) != count ||
			!row.TeamFrozenAt.Valid {
			add(
				"team_freeze",
				"teams_unready",
				"Every confirmed attendee needs an organiser-confirmed frozen team.",
			)
		}
	}
	if to == "live" {
		if !row.PublishedAt.Valid {
			add(
				"challenge_publication",
				"unpublished",
				"The challenge must be published before the live hack. If publication was missed, postpone to preserve the full preparation week or cancel.",
			)
		}
		if now.Before(c.LiveStart) {
			add("live_start", "too_early", "The live hack has not started yet.")
		}
	}
	if to == "assessment" && row.Stage != "completed" {
		if now.Before(c.SubmissionCutoff) {
			add(
				"submission_cutoff",
				"too_early",
				"Final assessment opens after the submission cutoff.",
			)
		}
		if !f.SubmissionsReady || f.SettledSubmissions != f.TotalTeams {
			add(
				"submission_cutoff",
				"unsettled",
				"Every team needs a frozen final submission or an explicit not-submitted/withdrawn outcome.",
			)
		}
	}
	return issues
}

// RecordAttendanceReady is called by the registration owner after its real
// capacity/registration implementation is installed. Counts remain derived
// directly from confirmed competition claims in the same database transaction.
func RecordAttendanceReady(ctx context.Context, tx pgx.Tx, event uuid.UUID) error {
	if _, err := dbgen.New(tx).LockEvent(ctx, event); err != nil {
		return fmt.Errorf("lock attendance facts: %w", err)
	}
	if _, err := tx.Exec(
		ctx,
		"UPDATE event_gate_facts SET attendance_ready=true WHERE event_id=$1",
		event,
	); err != nil {
		return fmt.Errorf("record attendance readiness: %w", err)
	}
	return nil
}

// PublicationReadiness is consumed by #69 inside the same transaction as the
// publication write. The publisher also owns the exact scheduled-time check.
func PublicationReadiness(ctx context.Context, tx pgx.Tx, event uuid.UUID) ([]Issue, error) {
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, event)
	if err != nil {
		return nil, fmt.Errorf("lock publication gates: %w", err)
	}
	if row.Stage != "preparation" {
		return []Issue{
			{
				Field:   "stage",
				Code:    "not_preparing",
				Message: "Only an event in preparation may publish its challenge.",
			},
		}, nil
	}
	c, err := decode(row)
	if err != nil {
		return nil, fmt.Errorf("decode publication rules: %w", err)
	}
	f, err := q.EventFacts(ctx, event)
	if err != nil {
		return nil, fmt.Errorf("read publication facts: %w", err)
	}
	count, err := q.ConfirmedAttendees(ctx, event)
	if err != nil {
		return nil, fmt.Errorf("count publication attendees: %w", err)
	}
	return gateBlockers(row, c, f, count, "publication", time.Time{}, false, ""), nil
}

// RecordTeamFacts and RecordSubmissionFacts are transactional seams for #59 and
// #60. They are never exposed as client-editable lifecycle inputs.
func RecordTeamFacts(
	ctx context.Context,
	tx pgx.Tx,
	event uuid.UUID,
	assigned, total, frozen int32,
	freezeAt *time.Time,
) error {
	if assigned < 0 || total < 0 || frozen < 0 || frozen > total {
		return ErrBlocked
	}
	if _, err := dbgen.New(tx).LockEvent(ctx, event); err != nil {
		return fmt.Errorf("lock team facts: %w", err)
	}
	if _, err := tx.Exec(
		ctx,
		"UPDATE event_gate_facts SET teams_ready=true,assigned_attendees=$2,total_teams=$3,frozen_teams=$4 WHERE event_id=$1",
		event,
		assigned,
		total,
		frozen,
	); err != nil {
		return fmt.Errorf("record team facts: %w", err)
	}
	if freezeAt != nil {
		if _, err := tx.Exec(
			ctx,
			"UPDATE events SET team_frozen_at=COALESCE(team_frozen_at,$2) WHERE id=$1",
			event,
			*freezeAt,
		); err != nil {
			return fmt.Errorf("record completed freeze: %w", err)
		}
	}
	return nil
}

func RecordSubmissionFacts(ctx context.Context, tx pgx.Tx, event uuid.UUID, settled int32) error {
	if settled < 0 {
		return ErrBlocked
	}
	if _, err := dbgen.New(tx).LockEvent(ctx, event); err != nil {
		return fmt.Errorf("lock submission facts: %w", err)
	}
	if _, err := tx.Exec(
		ctx,
		"UPDATE event_gate_facts SET submissions_ready=true,settled_submissions=$2 WHERE event_id=$1",
		event,
		settled,
	); err != nil {
		return fmt.Errorf("record submission facts: %w", err)
	}
	return nil
}
