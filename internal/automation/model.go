// Package automation binds reviewed campaigns to durable schedules and real
// lifecycle occurrences. Providers own their recipient delivery and receipts.
package automation

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
)

const HandlerKind = "communication.dispatch"

type ProviderQueue func(context.Context, pgx.Tx, uuid.UUID, uuid.UUID, int64, communications.DeliveryScope) (ChannelReport, error)

type ChannelReport struct {
	Queued         int            `json:"queued"`
	Unavailable    int            `json:"unavailable"`
	LocalPersisted int            `json:"local_persisted"`
	Preexisting    map[string]int `json:"preexisting"`
}

type ReviewGuard func(context.Context, pgx.Tx, uuid.UUID, uuid.UUID, int64, communications.DeliveryScope) error

type Service struct {
	SourceMaintenance func(context.Context, pgx.Tx, uuid.UUID) error
	ReviewGuards      map[string]ReviewGuard
	Pool              *pgxpool.Pool
	Composer          *communications.Service
	Jobs              *jobs.Service
	Providers         map[string]ProviderQueue
	Sources           map[string]SourcePolicy
	Now               func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// Definition is reviewed independently from the exact campaign approval. A
// lifecycle trigger never silently rewrites the campaign's approved due time.
type Definition struct {
	SourceVersion          int64      `json:"source_version"`
	ID                     uuid.UUID  `json:"id"`
	CampaignID             uuid.UUID  `json:"campaign_id"`
	CampaignVersion        int64      `json:"campaign_version"`
	Trigger                string     `json:"trigger"`
	SourceID               *uuid.UUID `json:"source_id"`
	MaximumLatenessSeconds int64      `json:"maximum_lateness_seconds"`
	RetainAfterArchive     bool       `json:"retain_after_archive"`
	RetainUntil            *time.Time `json:"retain_until"`
}
type Binding struct {
	PublicationApprovalID *uuid.UUID `json:"publication_approval_id"`
	DueAt                 time.Time  `json:"due_at,omitzero"`
	Definition            Definition `json:"definition"`
	Version               int64      `json:"version"`
	CampaignApproval      string     `json:"campaign_approval"`
	ActorID               uuid.UUID  `json:"actor_id"`
	ApprovedAt            *time.Time `json:"approved_at"`
	Cancelled             bool       `json:"cancelled"`
}
type Source struct {
	Kind       string
	ID         uuid.UUID
	Revision   int64
	OccurredAt time.Time
}

// SourcePolicy is implemented by an owning domain. It returns routing metadata
// only after checking the actual current source publication/authority. Private
// question/help bodies and assessment prose never enter this module.
type SourcePolicy interface {
	DiscoverInTransaction(context.Context, pgx.Tx, uuid.UUID, Definition) ([]Source, error)
	ResolveInTransaction(context.Context, pgx.Tx, uuid.UUID, Source) (SourceAudience, error)
}
type SourceAudience struct {
	RequireComplete bool
	RecipientIDs    []uuid.UUID
	Broadcast       bool
	// RetainedFollowUp is true only for an open, retained outcome follow-up.
	RetainedFollowUp bool
}
type Occurrence struct {
	ReviewRevision int64                    `json:"review_revision"`
	ChannelReports map[string]ChannelReport `json:"channel_reports"`
	ID             uuid.UUID                `json:"id"`
	BindingID      uuid.UUID                `json:"binding_id"`
	BindingVersion int64                    `json:"binding_version"`
	SourceKind     string                   `json:"source_kind"`
	SourceID       uuid.UUID                `json:"source_id"`
	SourceRevision int64                    `json:"source_revision"`
	DueAt          time.Time                `json:"due_at"`
	Deadline       time.Time                `json:"deadline"`
	State          string                   `json:"state"`
	Reason         string                   `json:"reason"`
	ReviewUntil    *time.Time               `json:"review_until"`
}
type Page struct {
	Revision    int64        `json:"revision"`
	Paused      bool         `json:"paused"`
	Bindings    []Binding    `json:"bindings"`
	Occurrences []Occurrence `json:"occurrences"`
}
type Command struct {
	PreviewPlanID uuid.UUID   `json:"preview_plan_id"`
	Action        string      `json:"action"`
	Definition    *Definition `json:"definition"`
	BindingID     uuid.UUID   `json:"binding_id"`
	Version       int64       `json:"version"`
	OccurrenceID  uuid.UUID   `json:"occurrence_id"`
	Reason        string      `json:"reason"`
}
