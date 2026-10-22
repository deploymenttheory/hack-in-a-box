package communications

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ChannelProvider reports actual adapter readiness. Setup URLs and the supported
// self-test shape are assigned by the campaign service, not by browser input.
// Teams Workflows is broadcast-only and never supports an organiser-only test.
type ChannelProvider interface {
	Readiness(context.Context, uuid.UUID, uuid.UUID) (ChannelReadiness, error)
}

type ChannelReadiness struct {
	Channel           string `json:"channel"`
	Available         bool   `json:"available"`
	SupportsSelfTest  bool   `json:"supports_self_test"`
	SelfTestAvailable bool   `json:"self_test_available"`
	Reason            string `json:"reason"`
	SelfTestReason    string `json:"self_test_reason"`
	SetupURL          string `json:"setup_url"`
}

type CampaignTiming struct {
	Kind         string `json:"kind"`
	WallTime     string `json:"wall_time"`
	DSTChoice    string `json:"dst_choice"`
	Anchor       string `json:"anchor"`
	Days         int    `json:"days"`
	Minutes      int    `json:"minutes"`
	BusinessDays bool   `json:"business_days"`
}

type CampaignDefinition struct {
	ID              uuid.UUID      `json:"id"`
	Name            string         `json:"name"`
	TemplateID      uuid.UUID      `json:"template_id"`
	AudienceID      uuid.UUID      `json:"audience_id"`
	Channels        []string       `json:"channels"`
	IncludeCalendar bool           `json:"include_calendar"`
	Timing          CampaignTiming `json:"timing"`
}

type CampaignApproval struct {
	PreviewID       uuid.UUID `json:"preview_id"`
	TemplateVersion int64     `json:"template_version"`
	AudienceVersion int64     `json:"audience_version"`
	Fingerprint     string    `json:"fingerprint"`
	Binding         string    `json:"binding"`
	ActorID         uuid.UUID `json:"actor_id"`
	ApprovedAt      time.Time `json:"approved_at"`
	DueAt           time.Time `json:"due_at"`
}

type Campaign struct {
	Definition CampaignDefinition `json:"definition"`
	Version    int64              `json:"version"`
	PreviewID  *uuid.UUID         `json:"preview_id"`
	Approval   *CampaignApproval  `json:"approval"`
	Cancelled  bool               `json:"cancelled"`
}

type CampaignView struct {
	Campaign Campaign   `json:"campaign"`
	Status   string     `json:"status"`
	DueAt    *time.Time `json:"due_at"`
	Blockers []string   `json:"blockers"`
}

type CampaignPage struct {
	Revision  int64              `json:"revision"`
	Timezone  string             `json:"timezone"`
	CanManage bool               `json:"can_manage"`
	Campaigns []CampaignView     `json:"campaigns"`
	Providers []ChannelReadiness `json:"providers"`
}

type CampaignDetail struct {
	Revision      int64              `json:"revision"`
	Timezone      string             `json:"timezone"`
	CanManage     bool               `json:"can_manage"`
	Campaign      CampaignView       `json:"campaign"`
	Preview       *Preview           `json:"preview"`
	Providers     []ChannelReadiness `json:"providers"`
	TestRequestID *uuid.UUID         `json:"test_request_id"`
}

type CampaignCommand struct {
	Action     string              `json:"action"`
	Version    int64               `json:"version"`
	Definition *CampaignDefinition `json:"definition"`
	Channel    string              `json:"channel"`
	Reason     string              `json:"reason"`
}

// TestMessage is resolved only for the requesting current organiser. PlainText
// is literal interpolated text. Providers must escape it for branded HTML and
// resolve the fixed actor's verified delivery destination privately.
type TestMessage struct {
	ID              uuid.UUID
	EventID         uuid.UUID
	ActorID         uuid.UUID
	Channel         string
	Purpose         string
	Format          string
	Subject         string
	PlainText       string
	IncludeCalendar bool
}

// ApprovedCampaign is the transaction-bound execution seam for #82. Resolving
// it never enqueues or sends. The caller retains its actual recipient attempts.
type ApprovedCampaign struct {
	Definition CampaignDefinition
	Version    int64
	DueAt      time.Time
	Preview    Preview
}
