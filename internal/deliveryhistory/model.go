// Package deliveryhistory exposes bounded delivery evidence and explicit operator
// recovery decisions. Providers retain message content and execution policy.
package deliveryhistory

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
)

type RecoveryProvider interface {
	ValidateRecoveryInTransaction(context.Context, pgx.Tx, jobs.Delivery) error
}

type Service struct {
	Pool      *pgxpool.Pool
	Jobs      *jobs.Service
	Providers map[string]RecoveryProvider
}

type Item struct {
	ID              uuid.UUID  `json:"id"`
	CampaignID      *uuid.UUID `json:"campaign_id"`
	CampaignVersion int64      `json:"campaign_version"`
	RecipientID     *uuid.UUID `json:"recipient_id"`
	RecipientName   string     `json:"recipient_name"`
	Channel         string     `json:"channel"`
	Purpose         string     `json:"purpose"`
	SelectedAt      time.Time  `json:"selected_at"`
	State           string     `json:"state"`
	Reason          string     `json:"reason"`
	Attempted       bool       `json:"attempted"`
	Attempts        int32      `json:"attempts"`
	Revision        int64      `json:"revision"`
	OperationState  string     `json:"operation_state"`
	Cancelled       bool       `json:"cancelled"`
}
type Cursor struct {
	At time.Time `json:"at"`
	ID uuid.UUID `json:"id"`
}
type Page struct {
	Items      []Item  `json:"items"`
	NextCursor *Cursor `json:"next_cursor"`
	Archived   bool    `json:"archived"`
	Paused     bool    `json:"paused"`
}
type Attempt struct {
	ID         uuid.UUID  `json:"id"`
	Number     int32      `json:"number"`
	StartedAt  time.Time  `json:"started_at"`
	State      string     `json:"state"`
	Reason     string     `json:"reason"`
	RecordedAt *time.Time `json:"recorded_at"`
	HTTPStatus *int32     `json:"http_status"`
	RetryAt    *time.Time `json:"retry_at"`
}
type Review struct {
	ID        uuid.UUID `json:"id"`
	ActorID   uuid.UUID `json:"actor_id"`
	Action    string    `json:"action"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}
type Detail struct {
	Item              Item       `json:"item"`
	Attempts          []Attempt  `json:"attempts"`
	NextAttemptNumber *int32     `json:"next_attempt_number"`
	Reviews           []Review   `json:"reviews"`
	NextReviewID      *uuid.UUID `json:"next_review_id"`
	Archived          bool       `json:"archived"`
	Paused            bool       `json:"paused"`
}
type Filter struct {
	CampaignID *uuid.UUID
	Channel    string
	Before     *Cursor
	Limit      int32
}
type Command struct {
	RequestID uuid.UUID `json:"request_id"`
	Action    string    `json:"action"`
	Reason    string    `json:"reason"`
}
