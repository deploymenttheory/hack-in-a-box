package certificates

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func (s Service) view(
	ctx context.Context,
	tx pgx.Tx,
	event dbgen.Event,
	actor uuid.UUID,
	staff bool,
	c Certificate,
) (View, error) {
	out := View{Certificate: c, History: []History{}}
	if !staff && c.PersonID != actor {
		return View{}, access.ErrNotFound
	}
	current, err := loadCertificate(ctx, tx, event.ID, c.ID)
	if err != nil {
		return out, failure("read", err)
	}
	if current.Version == c.Version && c.State == "issued" {
		eligibility, _, err := s.eligibility(ctx, tx, event, c.Target)
		if err != nil && !errors.Is(err, access.ErrInvalid) && !errors.Is(err, access.ErrNotFound) {
			return out, failure("read", err)
		}
		out.Current = err == nil && eligibility.Eligible && eligibility.Binding == c.Binding
	}
	if staff {
		rows, err := tx.Query(
			ctx,
			"SELECT document,actor_id,reason,request_id FROM certificate_versions WHERE event_id=$1 AND id=$2 ORDER BY version",
			event.ID,
			c.ID,
		)
		if err != nil {
			return out, failure("read", err)
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			var h History
			if err = rows.Scan(&raw, &h.ActorID, &h.Reason, &h.RequestID); err != nil {
				return out, failure("read", err)
			}
			if err = json.Unmarshal(raw, &h.Certificate); err != nil {
				return out, failure("read", err)
			}
			out.History = append(out.History, h)
		}
		if err = rows.Err(); err != nil {
			return out, failure("read", err)
		}
	}
	return out, nil
}

func (s Service) Read(ctx context.Context, actor, eventID, id uuid.UUID) (View, error) {
	var out View
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		row, staff, err := s.guard(ctx, tx, actor, eventID, false)
		if err != nil {
			return failure("read", err)
		}
		current, err := loadCertificate(ctx, tx, eventID, id)
		if err != nil {
			return failure("read", err)
		}
		out, err = s.view(ctx, tx, row, actor, staff, current)
		return failure("read", err)
	})
	return out, failure("read", err)
}
