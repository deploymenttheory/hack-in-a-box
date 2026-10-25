// Package resultpublication defines only safe explicitly announced results.
// Consumers depend on these immutable data types, never on private assessment owners.
package resultpublication

import (
	"time"

	"github.com/google/uuid"
)

type NamedTeam struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// Announcement contains only a deliberately released snapshot. Its type cannot
// carry private preparation, raw assessments, votes, evidence or draft prose.
type Announcement struct {
	ID          uuid.UUID        `json:"id"`
	Kind        string           `json:"kind"`
	Version     int64            `json:"version"`
	Finalists   []NamedTeam      `json:"finalists"`
	Reserves    []NamedTeam      `json:"reserves"`
	Awards      []AnnouncedAward `json:"awards"`
	PublishedAt time.Time        `json:"published_at"`
}
type AnnouncedAward struct {
	PrizeID    uuid.UUID  `json:"prize_id"`
	Name       string     `json:"name"`
	CategoryID *uuid.UUID `json:"category_id"`
	Team       NamedTeam  `json:"team"`
	Amount     string     `json:"amount"`
	Currency   string     `json:"currency"`
}

// Source is routing metadata for one actual explicit publication.
type Source struct {
	ID          uuid.UUID
	Version     int64
	PublishedAt time.Time
}
