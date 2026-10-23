package repositoryaccess

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type OwnView struct {
	TeamID          uuid.UUID  `json:"team_id"`
	RepositoryID    int64      `json:"repository_id"`
	CurrentGitHubID *int64     `json:"current_github_id"`
	LinkRevision    int64      `json:"link_revision"`
	Grants          []Grant    `json:"grants"`
	ObservedAt      *time.Time `json:"observed_at"`
	Diagnostic      string     `json:"diagnostic"`
}

func (s Service) Own(ctx context.Context, actor, eventID, teamID uuid.UUID) (OwnView, error) {
	out := OwnView{TeamID: teamID, Grants: []Grant{}}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		e, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock own repository invitations: %w", err)
		}
		if !e.PublishedAt.Valid || s.now().Before(e.PublishedAt.Time) {
			return access.ErrForbidden
		}
		authority, err := access.TeamPermissions(ctx, q, actor, eventID, teamID)
		if err != nil {
			return err
		}
		if !authority.Active {
			return access.ErrForbidden
		}
		if !authority.Member {
			if !authority.Mentor && !authority.Reviewer {
				return access.ErrForbidden
			}
			roster, err := (mentors.Service{}).ReadInTransaction(ctx, tx, actor, eventID)
			if err != nil {
				return err
			}
			current := false
			for _, assignment := range roster.Published {
				if assignment.TeamID == teamID && assignment.PersonID == actor && !s.now().Before(assignment.Window.Start) && s.now().Before(assignment.Window.End) {
					current = true
				}
			}
			if !current {
				return access.ErrForbidden
			}
		}
		r, err := repository(ctx, q, eventID, teamID)
		if err != nil {
			return err
		}
		out.RepositoryID = r.GithubRepositoryID.Int64
		out.LinkRevision, err = q.GitHubLinkRevision(ctx, actor)
		if err != nil {
			return fmt.Errorf("read current own GitHub revision: %w", err)
		}
		account, err := q.GitHubAccount(ctx, actor)
		if err == nil {
			out.CurrentGitHubID = &account.GithubID
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read current own GitHub identity: %w", err)
		}
		grants, err := readGrants(ctx, tx, eventID, teamID)
		if err != nil {
			return err
		}
		for _, grant := range grants {
			if grant.Member.PersonID == actor {
				out.Grants = append(out.Grants, grant)
			}
		}
		_, out.ObservedAt, out.Diagnostic, err = readObservation(ctx, tx, eventID, teamID)
		return err
	})
	return out, err
}
