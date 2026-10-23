// Package repositories owns the durable mapping between published teams and
// private GitHub repositories. Provider identities are immutable once proven.
package repositories

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// Reader is for trusted integration owners. Browser reads must use Service.List
// to apply current event authority. A disbanded team retains this mapping but its
// competition entry is inactive; repository activity never awards a score.
type Reader struct{ Pool *pgxpool.Pool }

func (r Reader) ByGitHubID(ctx context.Context, id int64) (dbgen.TeamRepository, error) {
	row, err := dbgen.New(r.Pool).RepositoryByGitHubID(ctx, pgtype.Int8{Int64: id, Valid: true})
	return row, repositoryError(err)
}

func (r Reader) ForTeam(
	ctx context.Context,
	eventID, teamID uuid.UUID,
) (dbgen.TeamRepository, error) {
	row, err := dbgen.New(r.Pool).
		TeamRepository(ctx, dbgen.TeamRepositoryParams{EventID: eventID, TeamID: teamID})
	return row, repositoryError(err)
}

func Name(eventID, teamID uuid.UUID) string {
	return "hackbox-" + strings.ReplaceAll(
		eventID.String(),
		"-",
		"",
	) + "-" + strings.ReplaceAll(
		teamID.String(),
		"-",
		"",
	)
}
