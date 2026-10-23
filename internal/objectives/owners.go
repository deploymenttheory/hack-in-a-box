package objectives

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

type (
	Owner struct {
		ID          uuid.UUID `json:"id"`
		DisplayName string    `json:"display_name"`
	}
	Owners struct {
		Items []Owner `json:"items"`
	}
)

// Owners exposes only current active organisers in this event, after current
// organiser read authority. It is not a generic person or participant directory.
func (s Service) Owners(ctx context.Context, actor, eventID uuid.UUID) (Owners, error) {
	out := Owners{Items: []Owner{}}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := authorize(ctx, tx, actor, eventID, false); err != nil {
			return err
		}
		ids, err := q.CommunicationCurrentOrganisers(ctx, eventID)
		if err != nil {
			return fmt.Errorf("read objective owner candidates: %w", err)
		}
		for _, id := range ids {
			p, err := q.Person(ctx, id)
			if err != nil {
				return fmt.Errorf("read current objective owner name: %w", err)
			}
			out.Items = append(out.Items, Owner{ID: p.ID, DisplayName: p.DisplayName})
		}
		return nil
	})
	if err != nil {
		return out, fmt.Errorf("read objective owners: %w", err)
	}
	return out, nil
}
