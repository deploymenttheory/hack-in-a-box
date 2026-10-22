package githubapp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
)

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)

type (
	Service struct {
		Pool     *pgxpool.Pool
		Keys     *secretbox.Keyring
		Provider Provider
	}
	Status struct {
		Revision       int64          `json:"revision"`
		Configured     bool           `json:"configured"`
		Configuration  *Configuration `json:"configuration,omitempty"`
		InstallationID int64          `json:"installation_id"`
		Diagnostics    Diagnostics    `json:"diagnostics"`
	}
	Command struct {
		Action string `json:"action"`
		Configuration
		PrivateKey string `json:"private_key"`
	}
)

func binding(row dbgen.GithubAppIntegration) []byte {
	return []byte(
		fmt.Sprintf(
			"hackbox:github-app-private-key:v1:%s:%d:%d",
			row.EventID,
			row.AppID,
			row.CredentialVersion,
		),
	)
}

func config(row dbgen.GithubAppIntegration) Configuration {
	return Configuration{
		AppID:              row.AppID,
		AppSlug:            row.AppSlug,
		OrganizationID:     row.OrganizationID,
		Organization:       row.Organization,
		TemplateRepository: row.TemplateRepository,
	}
}
func hash(value string) []byte        { sum := sha256.Sum256([]byte(value)); return sum[:] }
func sessionHash(value string) string { return hex.EncodeToString(hash(value)) }

func authority(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID uuid.UUID,
	revision *int64,
) (dbgen.Event, error) {
	event, err := q.LockEvent(ctx, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return event, access.ErrNotFound
	}
	if err != nil {
		return event, database.ErrDatabase
	}
	if err = (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		access.EventEdit,
	); err != nil {
		return event, fmt.Errorf("authorize GitHub App configuration: %w", err)
	}
	if revision != nil && event.Revision != *revision {
		return event, access.ErrConflict
	}
	return event, nil
}

func (s Service) Status(ctx context.Context, actor, eventID uuid.UUID) (Status, error) {
	var result Status
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		event, err := authority(ctx, q, actor, eventID, nil)
		if err != nil {
			return err
		}
		result = Status{Revision: event.Revision, Diagnostics: diagnostic("not_configured")}
		row, err := q.GitHubAppIntegration(ctx, eventID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return database.ErrDatabase
		}
		c := config(row)
		result.Configured = true
		result.Configuration = &c
		result.InstallationID = row.InstallationID
		result.Diagnostics = diagnostic("installation_required")
		if row.InstallationID == 0 {
			return nil
		}
		plain, err := s.Keys.Open(row.KeyID, row.PrivateKeyEncrypted, binding(row))
		if err != nil {
			result.Diagnostics = diagnostic("credentials_unavailable")
			return nil
		}
		defer clear(plain)
		if s.Provider == nil {
			result.Diagnostics = diagnostic("provider_unavailable")
			return nil
		}
		result.Diagnostics = s.Provider.Inspect(ctx, c, plain, row.InstallationID)
		return nil
	})
	if err != nil {
		return Status{}, fmt.Errorf("read GitHub App status: %w", err)
	}
	return result, nil
}

func (s Service) Configure(
	ctx context.Context,
	actor, eventID uuid.UUID,
	revision int64,
	input Command,
) error {
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, err := authority(ctx, q, actor, eventID, &revision); err != nil {
			return err
		}
		old, err := q.GitHubAppIntegration(ctx, eventID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return database.ErrDatabase
		}
		if input.Action == "disconnect" {
			if err = q.DeleteGitHubAppIntegration(ctx, eventID); err != nil {
				return database.ErrDatabase
			}
		} else {
			row, err := s.prepare(eventID, old, input)
			if err != nil {
				return err
			}
			if err = q.SaveGitHubAppIntegration(
				ctx,
				dbgen.SaveGitHubAppIntegrationParams{
					EventID:             eventID,
					AppID:               row.AppID,
					AppSlug:             row.AppSlug,
					OrganizationID:      row.OrganizationID,
					Organization:        row.Organization,
					TemplateRepository:  row.TemplateRepository,
					CredentialVersion:   row.CredentialVersion,
					KeyID:               row.KeyID,
					PrivateKeyEncrypted: row.PrivateKeyEncrypted,
					InstallationID:      row.InstallationID,
				},
			); err != nil {
				return database.ErrDatabase
			}
		}
		if err = q.ClearGitHubAppAttempts(ctx, eventID); err != nil {
			return database.ErrDatabase
		}
		if err = audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &eventID,
				TargetID: eventID,
				Action:   audit.GitHubIntegrationConfigured,
				Reason:   audit.OrganiserDecision,
			},
		); err != nil {
			return database.ErrDatabase
		}
		if _, err = q.BumpEventRevision(ctx, eventID); err != nil {
			return database.ErrDatabase
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("configure GitHub App: %w", err)
	}
	return nil
}

func (s Service) prepare(
	eventID uuid.UUID,
	old dbgen.GithubAppIntegration,
	input Command,
) (dbgen.GithubAppIntegration, error) {
	if input.Action == "rotate" {
		if old.EventID == uuid.Nil || input.PrivateKey != "" {
			return old, access.ErrInvalid
		}
		plain, err := s.Keys.Open(old.KeyID, old.PrivateKeyEncrypted, binding(old))
		if err != nil {
			return old, fmt.Errorf("decrypt App credentials: %w", err)
		}
		defer clear(plain)
		old.CredentialVersion++
		old.KeyID, old.PrivateKeyEncrypted, err = s.Keys.Seal(plain, binding(old))
		if err != nil {
			return old, fmt.Errorf("rotate App credentials: %w", err)
		}
		return old, nil
	}
	if input.Action != "configure" || input.AppID <= 0 || input.OrganizationID <= 0 ||
		!namePattern.MatchString(input.AppSlug) ||
		!namePattern.MatchString(input.Organization) ||
		!namePattern.MatchString(input.TemplateRepository) {
		return old, access.ErrInvalid
	}
	row := dbgen.GithubAppIntegration{
		EventID:            eventID,
		AppID:              input.AppID,
		AppSlug:            input.AppSlug,
		OrganizationID:     input.OrganizationID,
		Organization:       input.Organization,
		TemplateRepository: input.TemplateRepository,
		CredentialVersion:  old.CredentialVersion + 1,
	}
	plain := []byte(input.PrivateKey)
	if len(plain) == 0 && old.AppID == input.AppID && old.EventID != uuid.Nil {
		var err error
		plain, err = s.Keys.Open(old.KeyID, old.PrivateKeyEncrypted, binding(old))
		if err != nil {
			return row, fmt.Errorf("read retained App credentials: %w", err)
		}
	}
	defer clear(plain)
	if len(plain) == 0 || len(plain) > 32768 || s.Provider == nil ||
		!s.Provider.ValidateKey(input.AppID, plain) {
		return row, access.ErrInvalid
	}
	var err error
	row.KeyID, row.PrivateKeyEncrypted, err = s.Keys.Seal(plain, binding(row))
	if err != nil {
		return row, fmt.Errorf("protect App credentials: %w", err)
	}
	return row, nil
}

func (s Service) Begin(
	ctx context.Context,
	actor, eventID uuid.UUID,
	session string,
	revision int64,
) (string, error) {
	if session == "" {
		return "", access.ErrUnauthenticated
	}
	state := rand.Text() + rand.Text()
	var slug string
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, err := authority(ctx, q, actor, eventID, &revision); err != nil {
			return err
		}
		person, err := q.Person(ctx, actor)
		if err != nil {
			return database.ErrDatabase
		}
		if _, err = q.FindSession(ctx, sessionHash(session)); err != nil {
			return access.ErrUnauthenticated
		}
		row, err := q.GitHubAppIntegration(ctx, eventID)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrInvalid
		}
		if err != nil {
			return database.ErrDatabase
		}
		slug = row.AppSlug
		if err = q.ClearGitHubAppAttempts(ctx, eventID); err != nil {
			return database.ErrDatabase
		}
		if err = q.PutGitHubAppAttempt(
			ctx,
			dbgen.PutGitHubAppAttemptParams{
				StateHash:       hash(state),
				EventID:         eventID,
				ActorID:         actor,
				SessionHash:     sessionHash(session),
				SessionRevision: person.SessionRevision,
				EventRevision:   revision,
				ExpiresAt:       time.Now().Add(5 * time.Minute),
			},
		); err != nil {
			return database.ErrDatabase
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("begin GitHub App installation: %w", err)
	}
	return "https://github.com/apps/" + url.PathEscape(
		slug,
	) + "/installations/new?state=" + url.QueryEscape(
		state,
	), nil
}

func (s Service) Complete(
	ctx context.Context,
	actor uuid.UUID,
	session, state string,
	installationID int64,
) (uuid.UUID, error) {
	if session == "" || state == "" || len(state) > 256 || installationID <= 0 {
		return uuid.Nil, ErrIntegration
	}
	attempt, err := dbgen.New(s.Pool).
		ConsumeGitHubAppAttempt(ctx, dbgen.ConsumeGitHubAppAttemptParams{StateHash: hash(state), ActorID: actor, SessionHash: sessionHash(session)})
	if err != nil {
		return uuid.Nil, ErrIntegration
	}
	err = database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, err := authority(ctx, q, actor, attempt.EventID, &attempt.EventRevision); err != nil {
			return err
		}
		person, err := q.Person(ctx, actor)
		if err != nil || person.SessionRevision != attempt.SessionRevision {
			return access.ErrUnauthenticated
		}
		if _, err = q.FindSession(ctx, attempt.SessionHash); err != nil {
			return access.ErrUnauthenticated
		}
		row, err := q.GitHubAppIntegration(ctx, attempt.EventID)
		if err != nil {
			return ErrIntegration
		}
		plain, err := s.Keys.Open(row.KeyID, row.PrivateKeyEncrypted, binding(row))
		if err != nil {
			return fmt.Errorf("read App credentials: %w", err)
		}
		defer clear(plain)
		if s.Provider == nil {
			return ErrIntegration
		}
		health := s.Provider.Inspect(ctx, config(row), plain, installationID)
		// Missing permissions can be repaired on the proven installation; mismatched,
		// removed, suspended or unverified installations must never be attached.
		if health.Code != "healthy" && health.Code != "permissions_missing" &&
			health.Code != "template_inaccessible" &&
			health.Code != "template_invalid" {
			return ErrIntegration
		}
		if err = q.SetGitHubAppInstallation(
			ctx,
			dbgen.SetGitHubAppInstallationParams{
				EventID:        attempt.EventID,
				InstallationID: installationID,
			},
		); err != nil {
			return database.ErrDatabase
		}
		if err = audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &attempt.EventID,
				TargetID: attempt.EventID,
				Action:   audit.GitHubIntegrationInstalled,
				Reason:   audit.OrganiserDecision,
			},
		); err != nil {
			return database.ErrDatabase
		}
		if _, err = q.BumpEventRevision(ctx, attempt.EventID); err != nil {
			return database.ErrDatabase
		}
		return nil
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("complete GitHub App installation: %w", err)
	}
	return attempt.EventID, nil
}
