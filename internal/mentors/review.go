package mentors

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

type ReviewCoverage struct {
	Primary Assignment
	Second  Assignment
}

// ReviewCoverageInTransaction binds a huddle to actual published coverage. It
// grants no resource access and does not publish or replace an assignment.
// Replacement reviewers must pass the ordinary full coverage publication flow.
func ReviewCoverageInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, teamID uuid.UUID,
) (ReviewCoverage, error) {
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return ReviewCoverage{}, fmt.Errorf("lock review coverage: %w", err)
	}
	page, snapshot, err := load(ctx, q, row)
	if err != nil {
		return ReviewCoverage{}, fmt.Errorf("load current review coverage: %w", err)
	}
	var out ReviewCoverage
	for _, a := range page.Published {
		if a.TeamID == teamID {
			if a.Slot == "primary" {
				out.Primary = a
			}
			if a.Slot == "second" {
				out.Second = a
			}
		}
	}
	if out.Primary.PersonID == uuid.Nil || out.Second.PersonID == uuid.Nil ||
		out.Primary.PersonID == out.Second.PersonID {
		return ReviewCoverage{}, access.ErrConflict
	}
	for _, gap := range Validate(snapshot, page.Published) {
		relevant := gap.TeamID != nil && *gap.TeamID == teamID ||
			gap.PersonID != nil &&
				(*gap.PersonID == out.Primary.PersonID || *gap.PersonID == out.Second.PersonID)
		if gap.Hard && relevant {
			return ReviewCoverage{}, fmt.Errorf("%w: %w", access.ErrConflict, ErrCoverage)
		}
	}
	for _, person := range []uuid.UUID{out.Primary.PersonID, out.Second.PersonID} {
		authority, e := access.TeamPermissions(ctx, q, person, eventID, teamID)
		if e != nil {
			return ReviewCoverage{}, fmt.Errorf("authorize current reviewer: %w", e)
		}
		if !authority.Active || (!authority.Mentor && !authority.Reviewer) {
			return ReviewCoverage{}, access.ErrForbidden
		}
	}
	return out, nil
}

// AuthorizeModerationPanelInTransaction checks a named panel actor without
// granting them assignment access. Only explicit huddle membership may use it.
func AuthorizeModerationPanelInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
) error {
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("lock moderation panel authority: %w", err)
	}
	roles, _, err := access.ContentReader(ctx, q, actor, eventID)
	if err != nil {
		return fmt.Errorf("authorize current panel role: %w", err)
	}
	page, snapshot, err := load(ctx, q, row)
	if err != nil {
		return fmt.Errorf("authorize current panel role: %w", err)
	}
	var currentTeam *Team
	for _, team := range snapshot.Teams {
		if team.ID == teamID {
			copy := team
			currentTeam = &copy
		}
	}
	if currentTeam == nil || !currentTeam.Published ||
		conflicted(snapshot.Conflicts, *currentTeam, actor) {
		return access.ErrForbidden
	}
	if slices.Contains(roles, "organiser") {
		return nil
	}
	if !slices.Contains(roles, "mentor") {
		return access.ErrForbidden
	}
	for _, a := range page.Published {
		if a.TeamID == teamID && a.PersonID == actor {
			return nil
		}
	}
	return access.ErrForbidden
}
