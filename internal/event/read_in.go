package event

import (
	"context"
	"fmt"
	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ReadInTransaction retains the native event policy/projection when composing a
// current-authority read under the caller's event lock; it opens no second pool connection.
func (s Service) ReadInTransaction(ctx context.Context, tx pgx.Tx, actor, id uuid.UUID) (View, error) {
	return s.read(ctx, dbgen.New(tx), actor, id)
}

func (s Service) read(ctx context.Context, q *dbgen.Queries, actor, id uuid.UUID) (View, error) {
	if err := (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		id,
		id,
		access.EventRead,
	); err != nil {
		return View{}, fmt.Errorf("authorize event: %w", err)
	}
	row, err := q.Event(ctx, id)
	if err != nil {
		return View{}, fmt.Errorf("load event: %w", err)
	}
	return s.view(ctx, q, actor, row)
}
