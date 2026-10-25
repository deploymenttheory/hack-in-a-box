package teamreports

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func (s Service) guard(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
	staff, write bool,
) error {
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return failure("lock event", err)
	}
	policy := access.EventPolicy{Queries: q, Clock: s.now}
	if err = policy.Authorize(ctx, actor, eventID, eventID, access.EventRead); err != nil {
		return failure("current event reader", err)
	}
	grant, err := access.TeamPermissions(ctx, q, actor, eventID, teamID)
	if err != nil {
		return failure("current team scope", err)
	}
	team, err := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: eventID, ID: teamID})
	if err != nil {
		return failure("current published team", err)
	}
	if !grant.Active || team.State != "published" || staff && !grant.Organiser ||
		!staff && !grant.Member {
		return access.ErrForbidden
	}
	if write {
		if !row.WritesAllowed || slices.Contains([]string{"archived", "cancelled"}, row.Stage) {
			return access.ErrForbidden
		}
		if err = policy.Authorize(ctx, actor, eventID, eventID, access.EventEdit); err != nil {
			return failure("current report writer", err)
		}
	}
	return nil
}

func audience(ctx context.Context, tx pgx.Tx, eventID, teamID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := tx.Query(
		ctx,
		"SELECT m.person_id FROM hack_team_members m JOIN competition_claims c ON c.event_id=m.event_id AND c.person_id=m.person_id JOIN people p ON p.id=m.person_id WHERE m.event_id=$1 AND m.team_id=$2 AND m.snapshot='published' AND c.state='confirmed' AND p.active ORDER BY m.person_id",
		eventID,
		teamID,
	)
	if err != nil {
		return nil, failure("capture confirmed published audience", err)
	}
	defer rows.Close()
	out := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			return nil, failure("audience member", err)
		}
		out = append(out, id)
	}
	if err = rows.Err(); err != nil {
		return nil, failure("audience rows", err)
	}
	if len(out) == 0 {
		return nil, access.ErrConflict
	}
	return out, nil
}

func validateBody(value Body) bool {
	if strings.TrimSpace(value.Title) == "" || len(value.Title) > 160 ||
		strings.TrimSpace(value.Summary) == "" ||
		len(value.Summary) > 8000 ||
		len(value.Highlights) > 10 ||
		len(value.NextSteps) > 10 {
		return false
	}
	for _, items := range [][]string{value.Highlights, value.NextSteps} {
		for _, text := range items {
			if strings.TrimSpace(text) == "" || len(text) > 1000 {
				return false
			}
		}
	}
	return true
}

func validate(expected int64, c Command) error {
	if expected < 0 || c.RequestID == uuid.Nil || len(c.SourceBinding) != 64 ||
		strings.TrimSpace(c.Reason) == "" ||
		len(c.Reason) > 2000 {
		return access.ErrInvalid
	}
	switch c.Action {
	case "save":
		if !validateBody(c.Body) || c.IncludeFinalScores && len(c.ScoreBinding) != 64 ||
			!c.IncludeFinalScores && c.ScoreBinding != "" {
			return access.ErrInvalid
		}
	case "release":
		if c.Body.Title != "" || c.Body.Summary != "" ||
			len(c.Body.Highlights)+len(c.Body.NextSteps) > 0 ||
			c.IncludeFinalScores || c.ScoreBinding != "" {
			return access.ErrInvalid
		}
	default:
		return access.ErrInvalid
	}
	return nil
}
