package moderation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
)

// Read returns an existing huddle only to its currently intended panel. Planned
// records contain no other author's drafts; open records retain exact cards.
func (s Service) Read(ctx context.Context, actor, eventID, teamID, id uuid.UUID) (Huddle, error) {
	var out Huddle
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		h, err := load(ctx, tx, eventID, teamID, id)
		if err != nil {
			return fmt.Errorf("resolve moderation dependency: %w", err)
		}
		if _, err = s.current(ctx, tx, actor, eventID, h, false); err != nil {
			return fmt.Errorf("resolve moderation dependency: %w", err)
		}
		if len(h.Cards) > 0 {
			if _, err = s.currentCards(ctx, tx, eventID, h); err != nil {
				return fmt.Errorf("resolve moderation dependency: %w", err)
			}
		}
		out = h
		return nil
	})
	if err != nil {
		return Huddle{}, fmt.Errorf("read current huddle: %w", err)
	}
	return out, nil
}

// History retains immutable original values/reasons but never acts as an
// alternate route around current huddle/card/evidence authority.
func (s Service) History(
	ctx context.Context,
	actor, eventID, teamID, id uuid.UUID,
	before *int64,
	limit int32,
) (History, error) {
	if limit < 1 || limit > 100 || before != nil && *before < 1 {
		return History{}, access.ErrInvalid
	}
	out := History{Items: []Huddle{}}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		current, err := load(ctx, tx, eventID, teamID, id)
		if err != nil {
			return fmt.Errorf("resolve moderation dependency: %w", err)
		}
		if _, err = s.current(ctx, tx, actor, eventID, current, false); err != nil {
			return fmt.Errorf("resolve moderation dependency: %w", err)
		}
		if len(current.Cards) > 0 {
			if _, err = s.currentCards(ctx, tx, eventID, current); err != nil {
				return fmt.Errorf("resolve moderation dependency: %w", err)
			}
		}
		rows, err := tx.Query(
			ctx,
			`SELECT document FROM moderation_huddle_versions WHERE event_id=$1 AND team_id=$2 AND id=$3 AND ($4::bigint IS NULL OR version<$4) ORDER BY version DESC LIMIT $5`,
			eventID,
			teamID,
			id,
			before,
			limit+1,
		)
		if err != nil {
			return fmt.Errorf("page huddle decision history: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			if err = rows.Scan(&raw); err != nil {
				return fmt.Errorf("resolve moderation dependency: %w", err)
			}
			var h Huddle
			if err = json.Unmarshal(raw, &h); err != nil {
				return fmt.Errorf("resolve moderation dependency: %w", err)
			}
			out.Items = append(out.Items, h)
		}
		if err = rows.Err(); err != nil {
			return fmt.Errorf("resolve moderation dependency: %w", err)
		}
		if len(out.Items) > int(limit) {
			out.Items = out.Items[:limit]
			v := out.Items[len(out.Items)-1].Version
			out.NextCursor = &v
		}
		return nil
	})
	if err != nil {
		return History{}, fmt.Errorf("read private huddle history: %w", err)
	}
	return out, nil
}

// List uses exclusive UUID pages over current intended records. An obsolete
// context cannot make a still-current later huddle permanently unreachable.
func (s Service) List(
	ctx context.Context,
	actor, eventID, teamID, after uuid.UUID,
	limit int32,
) (Page, error) {
	if limit < 1 || limit > 100 {
		return Page{}, access.ErrInvalid
	}
	out := Page{Items: []Huddle{}}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		row, _, err := s.guard(ctx, tx, actor, eventID, teamID, false)
		if err != nil {
			return fmt.Errorf("resolve moderation dependency: %w", err)
		}
		out.Revision = row.Revision
		cursor := after
		for {
			rows, e := tx.Query(
				ctx,
				`WITH current AS(SELECT DISTINCT ON(id) id,document FROM moderation_huddle_versions WHERE event_id=$1 AND team_id=$2 AND id>$3 ORDER BY id,version DESC)SELECT id,document FROM current WHERE document->'panel_ids' ? $4 ORDER BY id LIMIT 100`,
				eventID,
				teamID,
				cursor,
				actor.String(),
			)
			if e != nil {
				return fmt.Errorf("page intended huddles: %w", e)
			}
			batch := []Huddle{}
			for rows.Next() {
				var id uuid.UUID
				var raw []byte
				if e = rows.Scan(&id, &raw); e != nil {
					rows.Close()
					return fmt.Errorf("read moderation dependency: %w", e)
				}
				cursor = id
				var h Huddle
				if e = json.Unmarshal(raw, &h); e != nil {
					rows.Close()
					return fmt.Errorf("read moderation dependency: %w", e)
				}
				batch = append(batch, h)
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return fmt.Errorf("read moderation dependency: %w", e)
			}
			for _, h := range batch {
				_, e = s.current(ctx, tx, actor, eventID, h, false)
				if e == nil && len(h.Cards) > 0 {
					_, e = s.currentCards(ctx, tx, eventID, h)
				}
				if e != nil {
					if errors.Is(e, access.ErrForbidden) || errors.Is(e, access.ErrConflict) ||
						errors.Is(e, access.ErrNotFound) ||
						errors.Is(e, mentors.ErrCoverage) {
						continue
					}
					return fmt.Errorf("read moderation dependency: %w", e)
				}
				out.Items = append(out.Items, h)
				if len(out.Items) > int(limit) {
					out.Items = out.Items[:limit]
					last := out.Items[len(out.Items)-1].Definition.ID
					out.NextCursor = &last
					return nil
				}
			}
			if len(batch) < 100 {
				return nil
			}
		}
	})
	if err != nil {
		return Page{}, fmt.Errorf("read intended moderation huddles: %w", err)
	}
	return out, nil
}
