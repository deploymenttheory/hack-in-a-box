package scorecards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
)

func organizer(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID uuid.UUID,
	write bool,
) (dbgen.Event, error) {
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return row, fmt.Errorf("lock checkpoint event: %w", err)
	}
	roles, _, err := access.ContentReader(ctx, q, actor, eventID)
	if err != nil {
		return row, fmt.Errorf("read checkpoint organiser: %w", err)
	}
	if !slices.Contains(roles, "organiser") {
		return row, access.ErrForbidden
	}
	if write && !row.WritesAllowed {
		return row, access.ErrForbidden
	}
	return row, nil
}

func decodeCheckpoint(raw []byte) (Checkpoint, error) {
	var c Checkpoint
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, fmt.Errorf("read checkpoint snapshot: %w", err)
	}
	return c, nil
}

func checkpoint(ctx context.Context, q *dbgen.Queries, eventID, id uuid.UUID) (Checkpoint, error) {
	row, err := q.ScorecardCheckpoint(
		ctx,
		dbgen.ScorecardCheckpointParams{EventID: eventID, ID: id},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Checkpoint{}, access.ErrNotFound
	}
	if err != nil {
		return Checkpoint{}, fmt.Errorf("read configured checkpoint: %w", err)
	}
	return decodeCheckpoint(row.Document)
}

func resolveWindow(row dbgen.Event, c Checkpoint) (Checkpoint, error) {
	var config event.Configuration
	if err := json.Unmarshal(row.Configuration, &config); err != nil {
		return c, fmt.Errorf("read checkpoint timezone: %w", err)
	}
	start, err := schedule.ResolveLocal(
		c.Definition.OpensWallTime,
		config.Timezone,
		c.Definition.DSTChoice,
	)
	if err != nil {
		return c, fmt.Errorf("resolve checkpoint start: %w", err)
	}
	end, err := schedule.ResolveLocal(
		c.Definition.ClosesWallTime,
		config.Timezone,
		c.Definition.DSTChoice,
	)
	if err != nil {
		return c, fmt.Errorf("resolve checkpoint end: %w", err)
	}
	if !end.After(start) || config.LiveStart.IsZero() ||
		!config.SubmissionCutoff.After(config.LiveStart) {
		return c, access.ErrInvalid
	}
	if c.Purpose == "checkpoint" &&
		(start.Before(config.LiveStart) || end.After(config.SubmissionCutoff)) {
		return c, access.ErrInvalid
	}
	if c.Purpose == "mentor_final" && start.Before(config.SubmissionCutoff) {
		return c, access.ErrInvalid
	}
	c.OpensAt = start
	c.ClosesAt = end
	c.Timezone = config.Timezone
	return c, nil
}

func (s Service) Configure(
	ctx context.Context,
	actor, eventID uuid.UUID,
	revision int64,
	input CheckpointCommand,
) (Checkpoint, error) {
	var out Checkpoint
	if input.Definition.ID == uuid.Nil || input.Definition.RunID == uuid.Nil ||
		strings.TrimSpace(input.Definition.Name) == "" ||
		len(input.Definition.Name) > 160 ||
		strings.TrimSpace(input.Reason) == "" ||
		len(input.Reason) > 2000 ||
		input.Version < 0 {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := organizer(ctx, q, actor, eventID, true)
		if err != nil {
			return err
		}
		if row.Revision != revision {
			return access.ErrConflict
		}
		prior, err := checkpoint(ctx, q, eventID, input.Definition.ID)
		if err != nil && !errors.Is(err, access.ErrNotFound) {
			return err
		}
		if prior.Version != input.Version {
			return access.ErrConflict
		}
		run, err := q.RubricRun(
			ctx,
			dbgen.RubricRunParams{EventID: eventID, ID: input.Definition.RunID},
		)
		if err != nil {
			return fmt.Errorf("read checkpoint rubric run: %w", err)
		}
		if run.Status != "current" ||
			!slices.Contains([]string{"checkpoint", "mentor_final"}, run.Purpose) {
			return access.ErrInvalid
		}
		version, err := q.RubricVersion(
			ctx,
			dbgen.RubricVersionParams{
				EventID:  eventID,
				RubricID: run.RubricID,
				Version:  run.RubricVersion,
			},
		)
		if err != nil {
			return fmt.Errorf("read checkpoint rubric snapshot: %w", err)
		}
		var definition rubric.Definition
		if err = json.Unmarshal(version.Document, &definition); err != nil {
			return fmt.Errorf("decode checkpoint criteria: %w", err)
		}
		if definition.Validate() != nil || definition.PublicDigest() != version.PublicDigest {
			return access.ErrConflict
		}
		out, err = resolveWindow(
			row,
			Checkpoint{
				Definition: input.Definition,
				Version:    input.Version + 1,
				Purpose:    run.Purpose,
				Rubric: rubric.Reference{
					ID:      run.RubricID,
					Version: run.RubricVersion,
					Digest:  version.PublicDigest,
				},
				Criteria: definition.PublicDefinition,
			},
		)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(out)
		if err != nil {
			return fmt.Errorf("encode configured checkpoint: %w", err)
		}
		if err = q.InsertScorecardCheckpoint(
			ctx,
			dbgen.InsertScorecardCheckpointParams{
				EventID:   eventID,
				ID:        out.Definition.ID,
				Version:   out.Version,
				RunID:     out.Definition.RunID,
				Document:  raw,
				ActorID:   actor,
				Reason:    input.Reason,
				CreatedAt: s.now(),
			},
		); err != nil {
			return fmt.Errorf("retain configured checkpoint: %w", err)
		}
		return recordChange(
			ctx,
			q,
			eventID,
			actor,
			out.Definition.ID,
			revision,
			"scorecard.checkpoint_changed",
		)
	})
	if err != nil {
		return Checkpoint{}, fmt.Errorf("configure scorecard checkpoint: %w", err)
	}
	return out, nil
}

func (s Service) Checkpoints(
	ctx context.Context,
	actor, eventID uuid.UUID,
	after uuid.UUID,
	count int,
) (CheckpointPage, error) {
	out := CheckpointPage{Items: []Checkpoint{}}
	size, err := limit(count)
	if err != nil {
		return out, err
	}
	err = database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := organizer(ctx, q, actor, eventID, false)
		if err != nil {
			return err
		}
		out.Revision = row.Revision
		rows, err := q.ScorecardCheckpoints(
			ctx,
			dbgen.ScorecardCheckpointsParams{EventID: eventID, AfterID: after, PageSize: size + 1},
		)
		if err != nil {
			return fmt.Errorf("page scorecard checkpoints: %w", err)
		}
		if len(rows) > int(size) {
			rows = rows[:size]
			id := rows[len(rows)-1].ID
			out.NextCursor = &id
		}
		for _, row := range rows {
			c, err := decodeCheckpoint(row.Document)
			if err != nil {
				return err
			}
			out.Items = append(out.Items, c)
		}
		return nil
	})
	if err != nil {
		return CheckpointPage{}, fmt.Errorf("read scorecard checkpoints: %w", err)
	}
	return out, nil
}

// TeamCheckpoints supplies only current published common criteria to a current
// assigned mentor. Draft/future rubric definitions stay organiser-only.
func (s Service) TeamCheckpoints(
	ctx context.Context,
	actor, eventID, teamID, after uuid.UUID,
	count int,
) (CheckpointPage, error) {
	out := CheckpointPage{Items: []Checkpoint{}}
	size, err := limit(count)
	if err != nil {
		return out, err
	}
	err = database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := s.guard(ctx, q, actor, eventID, teamID)
		if err != nil {
			return err
		}
		out.Revision = row.Revision
		common, err := q.EventRubric(ctx, eventID)
		if err != nil {
			return fmt.Errorf("read actually published checkpoint criteria: %w", err)
		}
		rows, err := q.ScorecardCheckpoints(
			ctx,
			dbgen.ScorecardCheckpointsParams{EventID: eventID, AfterID: after, PageSize: size + 1},
		)
		if err != nil {
			return fmt.Errorf("page assigned checkpoint metadata: %w", err)
		}
		if len(rows) > int(size) {
			rows = rows[:size]
			id := rows[len(rows)-1].ID
			out.NextCursor = &id
		}
		for _, raw := range rows {
			cp, err := decodeCheckpoint(raw.Document)
			if err != nil {
				return err
			}
			run, err := q.RubricRun(
				ctx,
				dbgen.RubricRunParams{EventID: eventID, ID: cp.Definition.RunID},
			)
			if err != nil {
				return fmt.Errorf("read checkpoint current rubric run: %w", err)
			}
			if run.Status != "current" || !common.PublishedVersion.Valid ||
				cp.Rubric.ID != common.ID ||
				cp.Rubric.Version != common.PublishedVersion.Int64 {
				continue
			}
			out.Items = append(out.Items, cp)
		}
		return nil
	})
	if err != nil {
		return CheckpointPage{}, fmt.Errorf("read assigned checkpoint configuration: %w", err)
	}
	return out, nil
}
