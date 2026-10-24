// Package media owns consent-dependent photographer submissions and explicitly
// reviewed galleries. Privacy, files and content retain their native authority.
package media

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
)

type Service struct {
	Pool    *pgxpool.Pool
	Clock   func() time.Time
	Files   files.Service
	Privacy privacy.Service
}

func (s Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

type Metadata struct {
	ChallengeRelated bool        `json:"challenge_related"`
	Kind             string      `json:"kind"`
	Title            string      `json:"title"`
	Caption          string      `json:"caption"`
	CapturedAt       *time.Time  `json:"captured_at"`
	RecordingURL     string      `json:"recording_url"`
	People           []uuid.UUID `json:"people"`
}
type Review struct {
	ContentReviewed        bool `json:"content_reviewed"`
	IdentificationReviewed bool `json:"identification_reviewed"`
	NoIdentifiablePeople   bool `json:"no_identifiable_people"`
}
type Document struct {
	ID         uuid.UUID                     `json:"id"`
	Version    int64                         `json:"version"`
	State      string                        `json:"state"`
	OwnerID    uuid.UUID                     `json:"owner_id"`
	Metadata   Metadata                      `json:"metadata"`
	FileID     *uuid.UUID                    `json:"file_id"`
	Review     *Review                       `json:"review"`
	Consent    []privacy.PhotoConsentBinding `json:"consent"`
	ReviewedBy *uuid.UUID                    `json:"reviewed_by"`
	ReleasedBy *uuid.UUID                    `json:"released_by"`
	ReviewedAt *time.Time                    `json:"reviewed_at"`
	ReleasedAt *time.Time                    `json:"released_at"`
	ActorID    *uuid.UUID                    `json:"actor_id"`
	Action     string                        `json:"action"`
	Reason     string                        `json:"reason"`
	ChangedAt  time.Time                     `json:"changed_at"`
}
type Command struct {
	Action    string    `json:"action"`
	Version   int64     `json:"version"`
	CommandID uuid.UUID `json:"command_id"`
	Metadata  *Metadata `json:"metadata,omitempty"`
	Review    *Review   `json:"review,omitempty"`
	Reason    string    `json:"reason"`
}
type Upload struct {
	ID       uuid.UUID
	Metadata Metadata
	Reason   string
	File     files.Upload
}
type PersonIndicator struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Consent  string    `json:"consent"`
	Eligible bool      `json:"eligible"`
}
type Item struct {
	// AcknowledgedVersion identifies an accepted write intent. All other fields
	// remain the current safe projection, even when an old intent is retried.
	AcknowledgedVersion *int64            `json:"acknowledged_version,omitempty"`
	ChallengeRelated    bool              `json:"challenge_related"`
	ID                  uuid.UUID         `json:"id"`
	Version             int64             `json:"version"`
	Kind                string            `json:"kind"`
	Title               string            `json:"title"`
	Caption             string            `json:"caption"`
	CapturedAt          *time.Time        `json:"captured_at"`
	State               string            `json:"state"`
	ConsentState        string            `json:"consent_state"`
	RecordingURL        string            `json:"recording_url"`
	CanReadHistory      bool              `json:"can_read_history"`
	CanReadOriginal     bool              `json:"can_read_original"`
	CanEdit             bool              `json:"can_edit"`
	CanReview           bool              `json:"can_review"`
	CanPublish          bool              `json:"can_publish"`
	CanWithdraw         bool              `json:"can_withdraw"`
	People              []PersonIndicator `json:"people"`
}
type Page struct {
	Revision   int64      `json:"revision"`
	CanCollect bool       `json:"can_collect"`
	CanReview  bool       `json:"can_review"`
	Items      []Item     `json:"items"`
	NextCursor *uuid.UUID `json:"next_cursor"`
}
type PublishedReference struct {
	ID      uuid.UUID
	Version int64
	Kind    string
	Title   string
	URL     string
}
