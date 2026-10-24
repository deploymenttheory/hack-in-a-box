// Package today composes current event-reader context without owning tasks,
// notifications, lifecycle decisions, or presentation control.
package today

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/presentation"
)

type Page struct {
	Event              event.View           `json:"event"`
	Roles              []string             `json:"roles"`
	ParticipationState string               `json:"participation_state"`
	Program            presentation.Program `json:"program"`
}

type Service struct {
	Events       event.Service
	Presentation presentation.Service
}

func (s Service) Read(ctx context.Context, actor, eventID uuid.UUID) (Page, error) {
	var result Page
	err := database.Within(ctx, s.Events.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(ctx, eventID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return access.ErrNotFound
			}
			return fmt.Errorf("lock Today event: %w", err)
		}
		if err := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.EventRead,
		); err != nil {
			return fmt.Errorf("authorize Today event reader: %w", err)
		}
		current, err := s.Events.ReadInTransaction(ctx, tx, actor, eventID)
		if err != nil {
			return fmt.Errorf("read current Today event: %w", err)
		}
		roles, err := q.ActiveRoles(ctx, dbgen.ActiveRolesParams{EventID: eventID, PersonID: actor})
		if err != nil {
			return fmt.Errorf("read current Today roles: %w", err)
		}
		participation, err := q.CompetitionState(
			ctx,
			dbgen.CompetitionStateParams{EventID: eventID, PersonID: actor},
		)
		if err != nil {
			return fmt.Errorf("read own participation: %w", err)
		}
		program, err := s.Presentation.ReadProgramIn(ctx, tx, actor, eventID)
		if err != nil {
			return fmt.Errorf("read published Today room program: %w", err)
		}
		if roles == nil {
			roles = []string{}
		}
		result = Page{
			Event:              current,
			Roles:              roles,
			ParticipationState: participation,
			Program:            program,
		}
		return nil
	})
	if err != nil {
		return Page{}, fmt.Errorf("read Today: %w", err)
	}
	return result, nil
}
