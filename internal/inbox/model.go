// Package inbox owns private recipient notifications and their current-authority projection.
package inbox

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
)

type Service struct {
	Pool           *pgxpool.Pool
	ScopePolicy    communications.ScopePolicy
	Communications *communications.Service
	Owners         map[string]ResourceOwner
	Now            func() time.Time
}

// Source is a typed owning-domain reference. It contains no recipient address,
// arbitrary destination URL, private title or source prose.
type Source struct {
	Kind    string    `json:"kind"`
	ID      uuid.UUID `json:"id"`
	Version int64     `json:"version"`
}

type Destination struct {
	SuppressEventName bool      `json:"-"`
	Kind              string    `json:"kind"`
	ResourceID        uuid.UUID `json:"resource_id"`
}

// ResourceOwner must recheck the source's current audience, publication and
// resource authority under the caller's locked event transaction. Missing owners
// are unavailable: facts alone never fabricate an implemented resource journey.
type ResourceOwner interface {
	AuthorizeNotification(
		context.Context,
		pgx.Tx,
		uuid.UUID,
		uuid.UUID,
		Source,
	) (Destination, error)
}

type Item struct {
	ID        uuid.UUID  `json:"id"`
	EventID   uuid.UUID  `json:"event_id"`
	EventName string     `json:"event_name"`
	Label     string     `json:"label"`
	CreatedAt time.Time  `json:"created_at"`
	ReadAt    *time.Time `json:"read_at"`
}
type Detail struct {
	Item        Item         `json:"item"`
	Subject     string       `json:"subject"`
	Body        string       `json:"body"`
	Format      string       `json:"format"`
	Destination *Destination `json:"destination"`
}
type Page struct {
	Items      []Item     `json:"items"`
	NextCursor *uuid.UUID `json:"next_cursor"`
	Unread     int        `json:"unread"`
}

type Delivery struct {
	OccurrenceID uuid.UUID
	ScopeReview  *communications.ScopeReview
	ScopeBinding string
	Provenance   communications.InboxProvenance
	ID           uuid.UUID
	EventID      uuid.UUID
	RecipientID  uuid.UUID
	Source       Source
	Subject      string
	Body         string
	Format       string
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
