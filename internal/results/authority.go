package results

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

type authority struct {
	event     dbgen.Event
	organiser bool
	judge     bool
	mentor    bool
}

func failure(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return access.ErrNotFound
	}
	return fmt.Errorf("results %s: %w", operation, err)
}

func (s Service) guard(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	write bool,
) (authority, error) {
	return s.guardRoles(ctx, tx, actor, eventID, write, false)
}

func (s Service) guardRoles(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	write, allowMentor bool,
) (authority, error) {
	var out authority
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return out, failure("lock event", err)
	}
	if err = (access.EventPolicy{Queries: q, Clock: s.now}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		access.EventRead,
	); err != nil {
		return out, failure("current event access", err)
	}
	roles, err := q.ActiveRoles(ctx, dbgen.ActiveRolesParams{EventID: eventID, PersonID: actor})
	if err != nil {
		return out, failure("current roles", err)
	}
	out = authority{
		event:     row,
		organiser: slices.Contains(roles, "organiser"),
		judge:     slices.Contains(roles, "judge"),
		mentor:    slices.Contains(roles, "mentor"),
	}
	if !out.organiser && !out.judge && (!allowMentor || !out.mentor) {
		return out, access.ErrForbidden
	}
	var dates event.Configuration
	if err = json.Unmarshal(row.Configuration, &dates); err != nil {
		return out, failure("event dates", err)
	}
	if !row.PublishedAt.Valid || s.now().Before(row.PublishedAt.Time) ||
		dates.SubmissionCutoff.IsZero() ||
		s.now().Before(dates.SubmissionCutoff) {
		return out, access.ErrForbidden
	}
	if write &&
		(!row.WritesAllowed || !slices.Contains([]string{"live", "assessment"}, row.Stage)) {
		return out, access.ErrForbidden
	}
	return out, nil
}
