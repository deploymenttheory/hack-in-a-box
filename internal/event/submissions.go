package event

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// A published deliverable's schedule link is part of its advertised deadline.
// Ordinary agenda edits must not silently reopen that deadline for competitors.
func submissionDeadlineChange(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	old, next Configuration,
	reason string,
) (bool, error) {
	if !row.PublishedAt.Valid {
		return false, nil
	}
	raw, err := q.EventDefinitions(ctx, row.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read published deliverable deadlines: %w", err)
	}
	var defs struct {
		Deliverables []struct {
			ScheduleID *uuid.UUID `json:"schedule_id"`
		} `json:"deliverables"`
	}
	if err = json.Unmarshal(raw.Definitions, &defs); err != nil {
		return false, fmt.Errorf("decode published deadline links: %w", err)
	}
	changed := false
	for _, d := range defs.Deliverables {
		if d.ScheduleID == nil {
			continue
		}
		before, err := linkedDeadline(old, *d.ScheduleID)
		if err != nil {
			return false, err
		}
		after, err := linkedDeadline(next, *d.ScheduleID)
		if err != nil {
			return false, err
		}
		if !before.Equal(after) {
			changed = true
		}
	}
	if changed && (strings.TrimSpace(reason) == "" || len(reason) > 2000) {
		return false, &Blocked{
			Issues: []Issue{
				{
					Field:   "schedule",
					Code:    "deliverable_deadline_reason",
					Message: "Changing a published deliverable deadline requires an organiser reason.",
				},
			},
		}
	}
	return changed, nil
}

func linkedDeadline(c Configuration, id uuid.UUID) (time.Time, error) {
	for _, item := range c.Schedule {
		if item.ID == id {
			at, err := Plan(c, item)
			if err != nil {
				return at, err
			}
			if at.Before(c.LiveStart) || at.After(c.SubmissionCutoff) {
				return at, &Blocked{
					Issues: []Issue{
						{
							Field:   "schedule",
							Code:    "deliverable_deadline",
							Message: "Competition deliverable deadlines must remain within the live submission window.",
						},
					},
				}
			}
			return at, nil
		}
	}
	return time.Time{}, &Blocked{
		Issues: []Issue{
			{
				Field:   "schedule",
				Code:    "deliverable_deadline",
				Message: "Retain the schedule items used by published competition deliverables.",
			},
		},
	}
}
