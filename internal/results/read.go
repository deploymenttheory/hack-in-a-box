package results

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// Read is private organiser preparation. It never returns raw independent cards
// or qualification totals, and cannot be used as a public announcement reader.
func (s Service) Read(ctx context.Context, actor, eventID uuid.UUID) (Page, error) {
	var out Page
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		current, err := s.guard(ctx, tx, actor, eventID, false)
		if err != nil {
			return err
		}
		if !current.organiser {
			return access.ErrForbidden
		}
		round, err := load(ctx, tx, eventID)
		if err != nil {
			return err
		}
		source, err := s.source(ctx, tx, actor, eventID)
		if err != nil {
			return err
		}
		out = Page{
			Revision:      current.event.Revision,
			Round:         round,
			Preview:       source.preview,
			Announcements: []Announcement{},
		}
		out.Current = round.Version > 0 && round.Source.Binding != "" &&
			round.Source.Binding == source.preview.Binding &&
			len(source.preview.Blockers) == 0
		if !out.Current {
			// Retain immutable history in storage, never stale private assessment data
			// in the current projection after source or assignment authority changes.
			out.Round.Source = emptyPreview()
			out.Round.Votes = []Vote{}
			out.Round.Confirmation = nil
		}
		out.Announcements, err = published(ctx, tx, eventID)
		return err
	})
	return out, failure("read private preparation", err)
}

func published(ctx context.Context, tx pgx.Tx, eventID uuid.UUID) ([]Announcement, error) {
	rows, err := tx.Query(
		ctx,
		"SELECT document FROM result_publications WHERE event_id=$1 ORDER BY published_at,id",
		eventID,
	)
	if err != nil {
		return nil, failure("read released announcements", err)
	}
	defer rows.Close()
	out := []Announcement{}
	for rows.Next() {
		var raw []byte
		var value Announcement
		if err = rows.Scan(&raw); err != nil {
			return nil, failure("announcement row", err)
		}
		if err = json.Unmarshal(raw, &value); err != nil {
			return nil, failure("decode announced projection", err)
		}
		out = append(out, value)
	}
	return out, failure("released announcement rows", rows.Err())
}
