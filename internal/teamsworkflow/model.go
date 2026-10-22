package teamsworkflow

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
)

type Service struct {
	Pool     *pgxpool.Pool
	Keys     *secretbox.Keyring
	Jobs     *jobs.Service
	Composer communications.Service
	Now      func() time.Time
	network  *adapter
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s Service) sender() *adapter {
	if s.network != nil {
		return s.network
	}
	return newAdapter()
}

// Only this explicit metadata projection may leave the domain. No plaintext URL,
// key identifier, ciphertext, private message or raw provider error is returned.
type Connection struct {
	ID        uuid.UUID `json:"id"`
	Version   int64     `json:"version"`
	Label     string    `json:"label"`
	Owner     string    `json:"owner"`
	CoOwner   string    `json:"co_owner"`
	Active    bool      `json:"active"`
	Available bool      `json:"available"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Receipt struct {
	ID                uuid.UUID  `json:"id"`
	Kind              string     `json:"kind"`
	ConnectionVersion int64      `json:"connection_version"`
	CampaignID        *uuid.UUID `json:"campaign_id"`
	CreatedAt         time.Time  `json:"created_at"`
	Status            string     `json:"status"`
	Reason            string     `json:"reason"`
	HTTPStatus        *int32     `json:"http_status"`
	RetryAt           *time.Time `json:"retry_at"`
	RecordedAt        *time.Time `json:"recorded_at"`
	Attempts          int32      `json:"attempts"`
}
type Page struct {
	Revision        int64       `json:"revision"`
	EncryptionReady bool        `json:"encryption_ready"`
	Connection      *Connection `json:"connection"`
	History         []Receipt   `json:"history"`
}
type Command struct {
	Action   string `json:"action"`
	Label    string `json:"label"`
	Owner    string `json:"owner"`
	CoOwner  string `json:"co_owner"`
	Endpoint string `json:"endpoint"`
}
type BroadcastInput struct {
	Version int64 `json:"version"`
}

type TestInput struct {
	RequestID uuid.UUID `json:"request_id"`
}

func (s Service) Readiness(
	ctx context.Context,
	eventID, actor uuid.UUID,
) (communications.ChannelReadiness, error) {
	r := communications.ChannelReadiness{
		Channel:        "teams",
		Reason:         "not_configured",
		SelfTestReason: "broadcast_only",
		SetupURL:       "#teams-workflow-setup",
	}
	page, err := s.Read(ctx, actor, eventID)
	if err != nil {
		return r, err
	}
	if page.Connection != nil {
		r.ConfigRevision = revision(page.Connection.ID, page.Connection.Version)
		r.Available = page.Connection.Active && page.Connection.Available && s.Jobs != nil
		if r.Available {
			r.Reason = "ready"
		}
	}
	return r, nil
}
