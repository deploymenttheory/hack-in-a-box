package githubidentity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/oauth2"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

type (
	Account struct {
		GitHubID   int64     `json:"github_id"`
		Login      string    `json:"login"`
		VerifiedAt time.Time `json:"verified_at"`
	}
	Status struct {
		Enabled                  bool     `json:"enabled"`
		Revision                 int64    `json:"revision"`
		Account                  *Account `json:"account"`
		InvitationsNeedingReview int64    `json:"invitations_needing_review"`
	}
)

func hash(value string) []byte { sum := sha256.Sum256([]byte(value)); return sum[:] }
func current(ctx context.Context, q *dbgen.Queries, actor uuid.UUID) (dbgen.Person, error) {
	p, err := q.LockIdentity(ctx, actor)
	if err != nil || !p.Active {
		return p, access.ErrUnauthenticated
	}
	return p, nil
}

func (s *Service) Status(ctx context.Context, actor uuid.UUID) (Status, error) {
	var result Status
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, err := current(ctx, q, actor); err != nil {
			return err
		}
		var err error
		result.Enabled = s.Enabled()
		result.Revision, err = q.GitHubLinkRevision(ctx, actor)
		if err != nil {
			return fmt.Errorf("read link revision: %w", err)
		}
		result.InvitationsNeedingReview, err = q.GitHubInvitationsNeedingReview(ctx, actor)
		if err != nil {
			return fmt.Errorf("read invitation impact: %w", err)
		}
		row, err := q.GitHubAccount(ctx, actor)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read linked identity: %w", err)
		}
		result.Account = &Account{
			GitHubID:   row.GithubID,
			Login:      row.Login,
			VerifiedAt: row.VerifiedAt,
		}
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("read GitHub link: %w", err)
	}
	return result, nil
}

func (s *Service) Begin(
	ctx context.Context,
	actor uuid.UUID,
	session string,
	revision int64,
	ack bool,
) (string, error) {
	if !s.Enabled() {
		return "", ErrDisabled
	}
	if !ack || session == "" {
		return "", access.ErrInvalid
	}
	state, verifier := rand.Text()+rand.Text(), oauth2.GenerateVerifier()
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		person, err := current(ctx, q, actor)
		if err != nil {
			return err
		}
		if err := q.EnsureGitHubLinkState(ctx, actor); err != nil {
			return fmt.Errorf("prepare link state: %w", err)
		}
		version, err := q.GitHubLinkRevision(ctx, actor)
		if err != nil {
			return fmt.Errorf("read current link revision: %w", err)
		}
		if version != revision {
			return access.ErrConflict
		}
		if err := q.PutGitHubLinkAttempt(
			ctx,
			dbgen.PutGitHubLinkAttemptParams{
				StateHash:         hash(state),
				PersonID:          actor,
				SessionHash:       hash(session),
				VerifierEncrypted: s.encrypt(actor, []byte(verifier)),
				LinkRevision:      version,
				SessionRevision:   person.SessionRevision,
				ExpiresAt:         time.Now().Add(5 * time.Minute),
			},
		); err != nil {
			return fmt.Errorf("save verification attempt: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("begin GitHub verification: %w", err)
	}
	return s.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier)), nil
}

func (s *Service) Complete(
	ctx context.Context,
	actor uuid.UUID,
	session, state, code string,
) error {
	if !s.Enabled() {
		return ErrDisabled
	}
	if session == "" || state == "" || code == "" || len(state) > 256 || len(code) > 2048 {
		return ErrLink
	}
	attempt, err := dbgen.New(s.Pool).
		ConsumeGitHubLinkAttempt(ctx, dbgen.ConsumeGitHubLinkAttemptParams{StateHash: hash(state), PersonID: actor, SessionHash: hash(session)})
	if err != nil {
		return ErrLink
	}
	verifier, err := s.decrypt(actor, attempt.VerifierEncrypted)
	if err != nil {
		return err
	}
	result, err := s.verify(ctx, actor, code, string(verifier))
	if err != nil {
		return err
	}
	err = database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		person, err := current(ctx, q, actor)
		if err != nil {
			return err
		}
		if person.SessionRevision != attempt.SessionRevision {
			return access.ErrUnauthenticated
		}
		if _, err := q.FindSession(ctx, hex.EncodeToString(hash(session))); err != nil {
			return access.ErrUnauthenticated
		}
		version, err := q.GitHubLinkRevision(ctx, actor)
		if err != nil {
			return fmt.Errorf("read callback revision: %w", err)
		}
		if version != attempt.LinkRevision {
			return access.ErrConflict
		}
		previous, err := q.GitHubAccount(ctx, actor)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read previous GitHub link: %w", err)
		}
		action := audit.GitHubLinked
		if previous.GithubID != 0 && previous.GithubID != result.ID {
			action = audit.GitHubRelinked
		}
		row, err := q.SaveGitHubAccount(
			ctx,
			dbgen.SaveGitHubAccountParams{
				PersonID:       actor,
				GithubID:       result.ID,
				Login:          result.Login,
				TokenEncrypted: result.Token,
			},
		)
		if err != nil {
			var conflict *pgconn.PgError
			if errors.As(err, &conflict) && conflict.Code == "23505" {
				return access.ErrConflict
			}
			return fmt.Errorf("save verified GitHub account: %w", err)
		}
		if previous.GithubID != result.ID {
			if _, err := q.FlagGitHubInvitations(ctx, actor); err != nil {
				return fmt.Errorf("flag affected invitations: %w", err)
			}
		} else {
			if err := q.RefreshGitHubInvitationBindings(
				ctx,
				dbgen.RefreshGitHubInvitationBindingsParams{
					PersonID:     actor,
					LinkRevision: version + 1,
					GithubID:     result.ID,
				},
			); err != nil {
				return fmt.Errorf("refresh unchanged invitation identity: %w", err)
			}
		}
		if err := q.AdvanceGitHubLink(ctx, actor); err != nil {
			return fmt.Errorf("advance verified link: %w", err)
		}
		return audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				TargetID: row.ID,
				Action:   action,
				Reason:   audit.UserRequest,
			},
		)
	})
	if err != nil {
		return fmt.Errorf("complete GitHub verification: %w", err)
	}
	return nil
}

func (s *Service) Unlink(ctx context.Context, actor uuid.UUID, revision int64, ack bool) error {
	if !ack {
		return access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, err := current(ctx, q, actor); err != nil {
			return err
		}
		if err := q.EnsureGitHubLinkState(ctx, actor); err != nil {
			return fmt.Errorf("prepare unlink: %w", err)
		}
		version, err := q.GitHubLinkRevision(ctx, actor)
		if err != nil {
			return fmt.Errorf("read unlink revision: %w", err)
		}
		if version != revision {
			return access.ErrConflict
		}
		if err := q.RemoveGitHubAccount(ctx, actor); err != nil {
			return fmt.Errorf("remove verified link: %w", err)
		}
		if err := q.ClearGitHubLinkAttempts(ctx, actor); err != nil {
			return fmt.Errorf("cancel outstanding verification: %w", err)
		}
		if _, err := q.FlagGitHubInvitations(ctx, actor); err != nil {
			return fmt.Errorf("flag pending invitations: %w", err)
		}
		if err := q.AdvanceGitHubLink(ctx, actor); err != nil {
			return fmt.Errorf("advance removed link: %w", err)
		}
		return audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				TargetID: actor,
				Action:   audit.GitHubUnlinked,
				Reason:   audit.UserRequest,
			},
		)
	})
	if err != nil {
		return fmt.Errorf("unlink GitHub account: %w", err)
	}
	return nil
}
