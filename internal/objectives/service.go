package objectives

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

type (
	Service struct {
		Pool  *pgxpool.Pool
		Clock func() time.Time
	}
	Version struct {
		Reference  Reference  `json:"reference"`
		Definition Definition `json:"definition"`
		Retired    bool       `json:"retired"`
		Reason     string     `json:"reason"`
		ActorID    uuid.UUID  `json:"actor_id"`
		At         time.Time  `json:"at"`
	}
)

type View struct {
	Version
	CurrentVersion int64    `json:"current_version"`
	CanManage      bool     `json:"can_manage"`
	PlanningGaps   []string `json:"planning_gaps"`
}
type Command struct {
	Action     string      `json:"action"`
	Definition *Definition `json:"definition"`
	Reason     string      `json:"reason"`
}
type (
	List struct {
		Items      []View     `json:"items"`
		NextCursor *uuid.UUID `json:"next_cursor"`
	}
	History struct {
		Items      []Version `json:"items"`
		NextCursor *int64    `json:"next_cursor"`
	}
)

func (s Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

func authorize(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	write bool,
) (dbgen.Event, error) {
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, access.ErrNotFound
	}
	if err != nil {
		return row, fmt.Errorf("lock objectives event: %w", err)
	}
	action := access.StaffRead
	if write {
		action = access.EventEdit
	}
	if err = (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		action,
	); err != nil {
		return row, fmt.Errorf("authorize objectives: %w", err)
	}
	return row, nil
}

func load(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	ref Reference,
) (Version, error) {
	var out Version
	row, err := q.ObjectiveVersion(
		ctx,
		dbgen.ObjectiveVersionParams{EventID: eventID, ObjectiveID: ref.ID, Version: ref.Version},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, access.ErrNotFound
	}
	if err != nil {
		return out, fmt.Errorf("read immutable objective: %w", err)
	}
	if err = json.Unmarshal(row.Definition, &out.Definition); err != nil {
		return out, fmt.Errorf("decode objective definition: %w", err)
	}
	out.Reference = ref
	out.Retired = row.Retired
	out.Reason = row.Reason
	out.ActorID = row.ActorID
	out.At = row.CreatedAt
	return out, nil
}

func (s Service) Read(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	version int64,
) (View, error) {
	var out View
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		row, err := authorize(ctx, tx, actor, eventID, false)
		if err != nil {
			return err
		}
		q := dbgen.New(tx)
		head, err := q.ObjectiveHead(ctx, dbgen.ObjectiveHeadParams{EventID: eventID, ID: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read objective head: %w", err)
		}
		if version == 0 {
			version = head.CurrentVersion
		}
		v, err := load(ctx, q, eventID, Reference{ID: id, Version: version})
		if err != nil {
			return err
		}
		out, err = project(ctx, tx, row, v, head.CurrentVersion)
		return err
	})
	if err != nil {
		return out, fmt.Errorf("read objectives configuration: %w", err)
	}
	return out, nil
}

func (s Service) Change(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	expected int64,
	c Command,
) (View, error) {
	var out View
	if err := validateCommand(id, expected, c); err != nil {
		return out, err
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := authorize(ctx, tx, actor, eventID, true); err != nil {
			return err
		}
		oqx := dbgen.New(tx)
		head, err := oqx.ObjectiveHead(ctx, dbgen.ObjectiveHeadParams{EventID: eventID, ID: id})
		exists := err == nil
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read objective write base: %w", err)
		}
		if (!exists && expected != 0) || (exists && head.CurrentVersion != expected) {
			return access.ErrConflict
		}
		var d Definition
		if c.Action == "retire" {
			if !exists {
				return access.ErrNotFound
			}
			old, err := load(ctx, oqx, eventID, Reference{ID: id, Version: expected})
			if err != nil {
				return err
			}
			if old.Retired {
				return access.ErrConflict
			}
			d = old.Definition
		} else {
			d = *c.Definition
			if d.OwnerID != nil {
				if err = (access.EventPolicy{Queries: q}).Authorize(
					ctx,
					*d.OwnerID,
					eventID,
					eventID,
					access.StaffRead,
				); err != nil {
					return access.ErrInvalid
				}
			}
		}
		next := expected + 1
		raw, err := json.Marshal(d)
		if err != nil {
			return fmt.Errorf("encode objective snapshot: %w", err)
		}
		if !exists {
			if err = oqx.CreateObjectiveHead(
				ctx,
				dbgen.CreateObjectiveHeadParams{EventID: eventID, ID: id},
			); err != nil {
				return fmt.Errorf("create objective identity: %w", err)
			}
		}
		now := s.now()
		if err = oqx.AppendObjectiveVersion(
			ctx,
			dbgen.AppendObjectiveVersionParams{
				EventID:     eventID,
				ObjectiveID: id,
				Version:     next,
				Definition:  raw,
				Retired:     c.Action == "retire",
				Reason:      c.Reason,
				ActorID:     actor,
				CreatedAt:   now,
			},
		); err != nil {
			return fmt.Errorf("append objective version: %w", err)
		}
		if err = oqx.AdvanceObjectiveHead(
			ctx,
			dbgen.AdvanceObjectiveHeadParams{EventID: eventID, ID: id, CurrentVersion: next},
		); err != nil {
			return fmt.Errorf("advance objective head: %w", err)
		}
		if err = audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &eventID,
				TargetID: id,
				Action:   audit.DefinitionsUpdated,
				Reason:   audit.OrganiserDecision,
			},
		); err != nil {
			return fmt.Errorf("audit objective change: %w", err)
		}
		out = View{
			Version: Version{
				Reference:  Reference{ID: id, Version: next},
				Definition: d,
				Retired:    c.Action == "retire",
				Reason:     c.Reason,
				ActorID:    actor,
				At:         now,
			},
			CurrentVersion: next,
			CanManage:      true,
			PlanningGaps:   d.PlanningGaps(),
		}
		return nil
	})
	if err != nil {
		return out, fmt.Errorf("change objectives configuration: %w", err)
	}
	return out, nil
}

func validateCommand(id uuid.UUID, expected int64, c Command) error {
	if id == uuid.Nil || expected < 0 || strings.TrimSpace(c.Reason) == "" ||
		!validText(c.Reason, 4000) ||
		(c.Action != "save" && c.Action != "retire") ||
		(c.Action == "save" && c.Definition == nil) ||
		(c.Action == "retire" && c.Definition != nil) {
		return access.ErrInvalid
	}
	if c.Definition != nil {
		if err := c.Definition.Validate(); err != nil {
			return err
		}
	}

	return nil
}

func project(
	ctx context.Context,
	tx pgx.Tx,
	row dbgen.Event,
	v Version,
	current int64,
) (View, error) {
	out := View{
		Version:        v,
		CurrentVersion: current,
		CanManage:      row.WritesAllowed,
		PlanningGaps:   v.Definition.PlanningGaps(),
	}
	if v.Definition.OwnerID != nil {
		err := (access.EventPolicy{Queries: dbgen.New(tx)}).Authorize(
			ctx,
			*v.Definition.OwnerID,
			row.ID,
			row.ID,
			access.StaffRead,
		)
		if errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) {
			out.PlanningGaps = append(out.PlanningGaps, "owner_unavailable")
		} else if err != nil {
			return out, fmt.Errorf("check current objective owner: %w", err)
		}
	}
	return out, nil
}
