package event

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

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
)

var ErrAgenda = errors.New(
	"agenda items need a title, valid audience, live_start or submission_cutoff anchor and bounded offsets",
)

func Plan(c Configuration, item AgendaItem) (time.Time, error) {
	if strings.TrimSpace(item.Title) == "" || len(item.Title) > 160 || len(item.Audience) == 0 ||
		len(item.Audience) > 8 ||
		item.Minutes < -1440 ||
		item.Minutes > 1440 {
		return time.Time{}, ErrAgenda
	}
	for _, role := range item.Audience {
		if !slices.Contains(
			[]string{
				"attendee",
				"organiser",
				"mentor",
				"judge",
				"mc",
				"logistics",
				"photographer",
				"all",
			},
			role,
		) {
			return time.Time{}, ErrAgenda
		}
	}
	anchor := c.LiveStart
	switch item.Anchor {
	case "live_start":
	case "submission_cutoff":
		anchor = c.SubmissionCutoff
	default:
		return time.Time{}, ErrAgenda
	}
	if anchor.IsZero() {
		return time.Time{}, ErrAgenda
	}
	var instant time.Time
	var err error
	if item.BusinessDays {
		instant, err = c.Calendar.Offset(anchor, c.Timezone, item.Days, c.DSTChoice)
	} else {
		instant, err = schedule.CalendarOffset(anchor, c.Timezone, item.Days, c.DSTChoice)
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("resolve agenda date: %w", err)
	}
	return instant.Add(time.Duration(item.Minutes) * time.Minute), nil
}

type Scheduled struct {
	Item     AgendaItem `json:"item"`
	DueAt    time.Time  `json:"due_at"`
	Status   string     `json:"status"`
	Revision int64      `json:"revision"`
}

func Preview(c Configuration, now time.Time) ([]Scheduled, error) {
	result := make([]Scheduled, 0, len(c.Schedule))
	for _, item := range c.Schedule {
		due, err := Plan(c, item)
		if err != nil {
			return nil, err
		}
		status := "draft"
		if !due.After(now) {
			status = "needs_review"
		}
		result = append(result, Scheduled{Item: item, DueAt: due, Status: status, Revision: 1})
	}
	return result, nil
}

func replan(ctx context.Context, q *dbgen.Queries, event uuid.UUID, c Configuration) error {
	existing, err := q.ScheduleItems(ctx, event)
	if err != nil {
		return fmt.Errorf("load previous schedule: %w", err)
	}
	for _, row := range existing {
		if err := q.CancelScheduleOperation(
			ctx,
			dbgen.CancelScheduleOperationParams{EventID: event, ID: row.ID},
		); err != nil {
			return fmt.Errorf("cancel obsolete operation: %w", err)
		}
		if err := q.CancelScheduleItem(
			ctx,
			dbgen.CancelScheduleItemParams{EventID: event, ID: row.ID},
		); err != nil {
			return fmt.Errorf("cancel obsolete schedule: %w", err)
		}
	}
	for _, item := range c.Schedule {
		due, err := Plan(c, item)
		if err != nil {
			return err
		}
		data, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("encode agenda item: %w", err)
		}
		if err := q.UpsertScheduleItem(
			ctx,
			dbgen.UpsertScheduleItemParams{
				EventID:    event,
				ID:         item.ID,
				Definition: data,
				DueAt:      due,
			},
		); err != nil {
			return fmt.Errorf("recalculate agenda item: %w", err)
		}
	}
	return nil
}

func (s Service) Schedule(ctx context.Context, actor, event uuid.UUID) ([]Scheduled, error) {
	q := dbgen.New(s.Pool)
	if err := (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		event,
		event,
		access.StaffRead,
	); err != nil {
		return nil, fmt.Errorf("authorize schedule: %w", err)
	}
	rows, err := q.ScheduleItems(ctx, event)
	if err != nil {
		return nil, fmt.Errorf("load schedule: %w", err)
	}
	result := make([]Scheduled, 0, len(rows))
	for _, row := range rows {
		var item AgendaItem
		if err := json.Unmarshal(row.Definition, &item); err != nil {
			return nil, fmt.Errorf("decode schedule: %w", err)
		}
		status := "draft"
		switch {
		case row.ExecutedAt.Valid:
			status = "executed"
		case row.Cancelled:
			status = "cancelled"
		case !row.DueAt.After(s.now()):
			status = "needs_review"
		case row.Approved:
			status = "approved"
		}
		result = append(
			result,
			Scheduled{Item: item, DueAt: row.DueAt, Status: status, Revision: row.Revision},
		)
	}
	return result, nil
}

func (s Service) ApproveSchedule(
	ctx context.Context,
	actor, event, item uuid.UUID,
	revision int64,
) error {
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, event)
		if err != nil {
			return fmt.Errorf("lock schedule event: %w", err)
		}
		if err := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			event,
			event,
			access.EventEdit,
		); err != nil {
			return fmt.Errorf("authorize schedule approval: %w", err)
		}
		if row.Revision != revision {
			return access.ErrConflict
		}
		current, err := q.ScheduleItems(ctx, event)
		if err != nil {
			return fmt.Errorf("load approval state: %w", err)
		}
		for _, existing := range current {
			if existing.ID == item && existing.Approved && !existing.Cancelled &&
				!existing.ExecutedAt.Valid &&
				existing.DueAt.After(s.now()) {
				return nil
			}
		}
		changed, err := q.ApproveScheduleItem(
			ctx,
			dbgen.ApproveScheduleItemParams{EventID: event, ID: item, DueAt: s.now()},
		)
		if err != nil {
			return fmt.Errorf("approve schedule: %w", err)
		}
		if changed != 1 {
			return &Blocked{
				Issues: []Issue{
					{
						Field:   "schedule",
						Code:    "not_approvable",
						Message: "Only future, unexecuted agenda items can be approved. Review past-due items explicitly.",
					},
				},
			}
		}
		return recordAudit(ctx, q, actor, event, "schedule.approved", item)
	})
	if err != nil {
		return fmt.Errorf("approve schedule transaction: %w", err)
	}
	return nil
}
