package githubapp

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// WebhookSecretBinding prevents encrypted signing keys and webhook secrets, or
// secrets belonging to different events, Apps and versions being substituted.
func WebhookSecretBinding(eventID uuid.UUID, appID, version int64) []byte {
	return []byte(fmt.Sprintf("hackbox:github-webhook-secret:v1:%s:%d:%d", eventID, appID, version))
}

func (s Service) configureWebhookSecret(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	appID int64,
	input Command,
) error {
	old, err := q.GitHubWebhookSecret(ctx, eventID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read protected webhook configuration: %w", err)
	}
	if input.Action == "disconnect" {
		return nil
	} // Integration FK already removed the secret.
	if input.WebhookSecret != "" &&
		(len(input.WebhookSecret) < 32 || len(input.WebhookSecret) > 256 || input.Action != "configure") {
		return access.ErrInvalid
	}
	if input.WebhookSecret == "" && old.AppID != appID {
		if err := q.DeleteGitHubWebhookSecret(ctx, eventID); err != nil {
			return database.ErrDatabase
		}
		return nil
	}
	if input.WebhookSecret == "" && input.Action != "rotate" {
		return nil
	}
	plain := []byte(input.WebhookSecret)
	if len(plain) == 0 {
		if old.EventID == uuid.Nil {
			return nil
		}
		plain, err = s.Keys.Open(
			old.KeyID,
			old.Encrypted,
			WebhookSecretBinding(eventID, appID, old.Version),
		)
		if err != nil {
			return fmt.Errorf("read webhook secret for rotation: %w", err)
		}
	}
	defer clear(plain)
	version := old.Version + 1
	if old.EventID == uuid.Nil {
		// A fresh random generation prevents pre-disconnect deliveries becoming
		// current again when the same App/installation/secret is configured later.
		generation, randomErr := rand.Int(rand.Reader, big.NewInt(1<<62))
		if randomErr != nil {
			return fmt.Errorf("create webhook secret generation: %w", randomErr)
		}
		version = generation.Int64() + 1
	}
	keyID, encrypted, err := s.Keys.Seal(plain, WebhookSecretBinding(eventID, appID, version))
	if err != nil {
		return fmt.Errorf("protect webhook secret: %w", err)
	}
	err = q.SaveGitHubWebhookSecret(
		ctx,
		dbgen.SaveGitHubWebhookSecretParams{
			EventID:   eventID,
			AppID:     appID,
			Version:   version,
			KeyID:     keyID,
			Encrypted: encrypted,
		},
	)
	if err != nil {
		return database.ErrDatabase
	}
	return nil
}
