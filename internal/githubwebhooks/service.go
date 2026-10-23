package githubwebhooks

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
)

const (
	HandlerKind = "github_webhook_activity"
	MaxPending  = 1000
)

var ErrCapacity = errors.New("webhook work queue is full")

// MembershipHints receives only authenticated, current-source member evidence.
// The owning service must reconcile through current provider reads; action and
// receipt time cannot establish occurrence order or invitation acceptance.
type MembershipHints interface {
	MemberChangedInTransaction(
		context.Context,
		pgx.Tx,
		uuid.UUID,
		uuid.UUID,
		int64,
		int64,
		string,
		time.Time,
	) error
}

type Service struct {
	Membership MembershipHints
	Pool       *pgxpool.Pool
	Keys       *secretbox.Keyring
	Jobs       *jobs.Service
}

type Headers struct{ DeliveryID, Event, Signature string }

// authenticate does not parse payloads or mutate any domain state. The same
// check runs again under the event lock, so disconnect/rotation cannot race it.
func (s Service) authenticate(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	raw []byte,
	signature string,
) (dbgen.GithubAppIntegration, dbgen.GithubWebhookSecret, error) {
	app, err := q.GitHubAppIntegration(ctx, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return app, dbgen.GithubWebhookSecret{}, access.ErrForbidden
	}
	if err != nil {
		return app, dbgen.GithubWebhookSecret{}, database.ErrDatabase
	}
	secret, err := q.GitHubWebhookSecret(ctx, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return app, secret, access.ErrForbidden
	}
	if err != nil {
		return app, secret, database.ErrDatabase
	}
	if secret.AppID != app.AppID || app.InstallationID <= 0 {
		return app, secret, access.ErrForbidden
	}
	plain, err := s.Keys.Open(
		secret.KeyID,
		secret.Encrypted,
		githubapp.WebhookSecretBinding(eventID, app.AppID, secret.Version),
	)
	if err != nil {
		return app, secret, secretbox.ErrUnavailable
	}
	defer clear(plain)
	if !validSignature(raw, plain, signature) {
		return app, secret, access.ErrForbidden
	}
	return app, secret, nil
}

// Receive supports the single webhook URL configured on a GitHub App across
// events. The sole unauthenticated read is immutable numeric repository routing;
// no activity is selected or written until Ingest authenticates the raw bytes.
func (s Service) Receive(ctx context.Context, h Headers, raw []byte) error {
	if len(raw) == 0 || int64(len(raw)) > MaxBodyBytes {
		return access.ErrInvalid
	}
	var routing struct {
		Repository struct {
			ID int64 `json:"id"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(raw, &routing); err != nil || routing.Repository.ID <= 0 {
		return access.ErrInvalid
	}
	repository, err := dbgen.New(s.Pool).
		RepositoryByGitHubID(ctx, pgtype.Int8{Int64: routing.Repository.ID, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return access.ErrForbidden
	}
	if err != nil {
		return database.ErrDatabase
	}
	return s.Ingest(ctx, repository.EventID, h, raw)
}

func (s Service) Ingest(ctx context.Context, eventID uuid.UUID, h Headers, raw []byte) error {
	if len(raw) == 0 || int64(len(raw)) > MaxBodyBytes {
		return access.ErrInvalid
	}
	if _, _, err := s.authenticate(ctx, dbgen.New(s.Pool), eventID, raw, h.Signature); err != nil {
		return err
	}
	deliveryID, err := uuid.Parse(h.DeliveryID)
	if err != nil || deliveryID == uuid.Nil {
		return access.ErrInvalid
	}
	received := time.Now().UTC().Truncate(time.Microsecond)
	selected, err := selectPayload(h.Event, raw, received)
	if err != nil {
		return err
	}
	err = database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		return s.ingestLocked(ctx, tx, q, eventID, deliveryID, h, raw, selected, received)
	})
	if err != nil {
		return fmt.Errorf("ingest authenticated GitHub webhook: %w", err)
	}
	return nil
}

func (s Service) ingestLocked(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	eventID, deliveryID uuid.UUID,
	h Headers,
	raw []byte,
	selected selected,
	received time.Time,
) error {
	digest := sha256.Sum256(raw)

	event, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return database.ErrDatabase
	}
	app, secret, err := s.authenticate(ctx, q, eventID, raw, h.Signature)
	if err != nil {
		return err
	}
	repository, err := q.RepositoryByGitHubID(
		ctx,
		pgtype.Int8{Int64: selected.repositoryID, Valid: true},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return access.ErrForbidden
	}
	if err != nil {
		return database.ErrDatabase
	}
	if repository.EventID != eventID || repository.AppID != app.AppID ||
		repository.InstallationID != app.InstallationID ||
		selected.installationID != app.InstallationID ||
		repository.OrganizationID != app.OrganizationID ||
		selected.organizationID != app.OrganizationID {
		return access.ErrForbidden
	}
	current, err := sourceCurrent(
		ctx,
		q,
		event,
		repository,
		selected.kind,
		selected.providerAt,
		received,
	)
	if err != nil {
		return err
	}
	if !current {
		return access.ErrForbidden
	}
	// A repeated GUID must identify identical authenticated selected source and
	// bytes. Reusing it for different content never silently acknowledges work.
	previous, err := q.GitHubWebhookDelivery(ctx, deliveryID)
	if err == nil {
		if previous.EventID != eventID || previous.Kind != selected.kind ||
			previous.RepositoryID != selected.repositoryID ||
			!bytes.Equal(previous.PayloadHash, digest[:]) {
			return access.ErrConflict
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return database.ErrDatabase
	}
	var pending int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM github_webhook_deliveries WHERE event_id=$1 AND disposition='pending'", eventID).
		Scan(&pending); err != nil {
		return database.ErrDatabase
	}
	if pending >= MaxPending {
		return ErrCapacity
	}
	if s.Jobs == nil {
		return database.ErrDatabase
	}
	inserted, err := q.InsertGitHubWebhookDelivery(
		ctx,
		dbgen.InsertGitHubWebhookDeliveryParams{
			ID:             deliveryID,
			EventID:        eventID,
			TeamID:         repository.TeamID,
			RepositoryID:   selected.repositoryID,
			AppID:          app.AppID,
			InstallationID: app.InstallationID,
			Provenance:     repository.Provenance,
			SecretVersion:  secret.Version,
			PayloadHash:    digest[:],
			Kind:           selected.kind,
			Action:         selected.action,
			Ref:            selected.ref,
			AfterSha:       selected.after,
			MemberID:       selected.memberID,
			ProviderAt:     selected.providerAt,
			ReceivedAt:     received,
		},
	)
	if err != nil {
		return database.ErrDatabase
	}
	if inserted != 1 {
		return access.ErrConflict
	}
	op, err := s.Jobs.Enqueue(
		ctx,
		tx,
		dbgen.InsertOperationParams{
			EventID:      eventID,
			TargetID:     deliveryID,
			OperationKey: "github-webhook:" + deliveryID.String(),
			Kind:         HandlerKind,
			DueAt:        received,
			Approved:     true,
		},
	)
	if err != nil {
		return fmt.Errorf("enqueue authenticated webhook references: %w", err)
	}
	err = q.LinkGitHubWebhookOperation(
		ctx,
		dbgen.LinkGitHubWebhookOperationParams{ID: deliveryID, OperationID: &op.ID},
	)
	if err != nil {
		return database.ErrDatabase
	}
	return nil
}

func sourceCurrent(
	ctx context.Context,
	q *dbgen.Queries,
	event dbgen.Event,
	repository dbgen.TeamRepository,
	kind string,
	providerAt pgtype.Timestamptz,
	received time.Time,
) (bool, error) {
	if !event.WritesAllowed || repository.State != "ready" {
		return false, nil
	}
	team, err := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: event.ID, ID: repository.TeamID})
	if err != nil {
		return false, database.ErrDatabase
	}
	if team.State != "published" {
		return false, nil
	}
	if kind == "push" {
		// Actual common publication, not the scheduled date or live stage, enables
		// ordinary work. This records no formal submission at any stage.
		if !event.PublishedAt.Valid || received.Before(event.PublishedAt.Time) ||
			(providerAt.Valid && providerAt.Time.Before(event.PublishedAt.Time)) {
			return false, nil
		}
	}
	return true, nil
}

func (s Service) Eligible(ctx context.Context, d jobs.Delivery) (bool, error) {
	row, err := dbgen.New(s.Pool).GitHubWebhookDelivery(ctx, d.TargetID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, database.ErrDatabase
	}
	return row.EventID == d.EventID && row.OperationID != nil && *row.OperationID == d.ID, nil
}

func currentDeliverySource(
	ctx context.Context,
	q *dbgen.Queries,
	event dbgen.Event,
	row dbgen.GithubWebhookDelivery,
) (bool, error) {
	app, appErr := q.GitHubAppIntegration(ctx, row.EventID)
	secret, secretErr := q.GitHubWebhookSecret(ctx, row.EventID)
	repository, repoErr := q.RepositoryByGitHubID(
		ctx,
		pgtype.Int8{Int64: row.RepositoryID, Valid: true},
	)
	for _, err := range []error{appErr, secretErr, repoErr} {
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return false, database.ErrDatabase
		}
	}
	current := appErr == nil && secretErr == nil && repoErr == nil && app.AppID == row.AppID &&
		app.InstallationID == row.InstallationID &&
		app.OrganizationID == repository.OrganizationID &&
		secret.AppID == row.AppID &&
		secret.Version == row.SecretVersion &&
		repository.EventID == row.EventID &&
		repository.TeamID == row.TeamID &&
		repository.Provenance == row.Provenance &&
		repository.AppID == row.AppID &&
		repository.InstallationID == row.InstallationID
	var err error
	if current {
		current, err = sourceCurrent(
			ctx,
			q,
			event,
			repository,
			row.Kind,
			row.ProviderAt,
			row.ReceivedAt,
		)
		if err != nil {
			return false, err
		}
	}
	return current, nil
}

func (s Service) Deliver(ctx context.Context, d jobs.Delivery) jobs.Outcome {
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		event, err := q.LockEvent(ctx, d.EventID)
		if err != nil {
			return database.ErrDatabase
		}
		op, err := q.LockOperation(ctx, d.ID)
		if err != nil {
			return database.ErrDatabase
		}
		if !event.WritesAllowed || !op.Approved || op.Cancelled || op.Kind != HandlerKind ||
			op.TargetID != d.TargetID ||
			op.EventID != d.EventID {
			return access.ErrForbidden
		}
		row, err := q.GitHubWebhookDelivery(ctx, d.TargetID)
		if err != nil {
			return database.ErrDatabase
		}
		if row.EventID != d.EventID || row.OperationID == nil || *row.OperationID != op.ID {
			return access.ErrForbidden
		}
		if row.Disposition != "pending" {
			return nil
		}
		disposition := "discarded"
		current, err := currentDeliverySource(ctx, q, event, row)
		if err != nil {
			return err
		}
		if current {
			disposition = "recorded"
		}
		changed, err := q.RecordGitHubWebhookDisposition(
			ctx,
			dbgen.RecordGitHubWebhookDispositionParams{ID: row.ID, Disposition: disposition},
		)
		if err != nil {
			return database.ErrDatabase
		}
		if changed == 0 || !current {
			return nil
		}
		if row.Kind == "member" && s.Membership != nil {
			if err := s.Membership.MemberChangedInTransaction(
				ctx,
				tx,
				row.EventID,
				row.TeamID,
				row.RepositoryID,
				row.MemberID,
				row.Action,
				row.ReceivedAt,
			); err != nil {
				return fmt.Errorf("queue current repository member reconciliation: %w", err)
			}
		}
		var pushes, invitations int64
		if row.Kind == "push" {
			pushes = 1
		}
		if row.Kind == "member" && row.Action == "added" {
			invitations = 1
		}
		if err := q.AddGitHubRepositoryActivity(
			ctx,
			dbgen.AddGitHubRepositoryActivityParams{
				EventID:        row.EventID,
				TeamID:         row.TeamID,
				Pushes:         pushes,
				Invitations:    invitations,
				LastProviderAt: row.ProviderAt,
				LastReceivedAt: row.ReceivedAt,
			},
		); err != nil {
			return database.ErrDatabase
		}
		if err := audit.Append(
			ctx,
			q,
			audit.Record{
				EventID:  &row.EventID,
				TargetID: row.ID,
				Action:   audit.GitHubActivityRecorded,
				Reason:   audit.Authentication,
				Metadata: audit.Metadata{ReferenceID: row.OperationID},
			},
		); err != nil {
			return database.ErrDatabase
		}
		return nil
	})
	if err != nil {
		return jobs.Retryable
	}
	return jobs.Delivered
}

// All effects are a single local transaction with a delivery disposition guard;
// repeating after a crash cannot apply the same activity twice.
func (s Service) Reconcile(ctx context.Context, d jobs.Delivery) jobs.Outcome {
	return s.Deliver(ctx, d)
}
