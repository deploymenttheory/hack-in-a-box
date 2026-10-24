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
	ConfigRevision    string `json:"config_revision"`
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
	ChannelRevisions map[string]string `json:"channel_revisions"`
	PreviewID        uuid.UUID         `json:"preview_id"`
	TemplateVersion  int64             `json:"template_version"`
	AudienceVersion  int64             `json:"audience_version"`
	Fingerprint      string            `json:"fingerprint"`
	Binding          string            `json:"binding"`
	ActorID          uuid.UUID         `json:"actor_id"`
	ApprovedAt       time.Time         `json:"approved_at"`
	DueAt            time.Time         `json:"due_at"`
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
	ReviewedTestFingerprint string              `json:"reviewed_test_fingerprint,omitempty"`
	Action                  string              `json:"action"`
	Version                 int64               `json:"version"`
	Definition              *CampaignDefinition `json:"definition,omitempty"`
	Channel                 string              `json:"channel,omitempty"`
	Reason                  string              `json:"reason"`
}

// TestMessage is resolved only for the requesting current organiser. PlainText
// is literal interpolated text. Providers must escape it for branded HTML and
// resolve the fixed actor's verified delivery destination privately.
type TestMessage struct {
	// Optional private binding for a reviewed organiser-only TEST; never an address.
	ReviewedTestBinding string    `json:"reviewed_test_binding,omitempty"`
	ID                  uuid.UUID `json:"id"`
	EventID             uuid.UUID `json:"event_id"`
	ActorID             uuid.UUID `json:"actor_id"`
	Channel             string    `json:"channel"`
	Purpose             string    `json:"purpose"`
	Format              string    `json:"format"`
	Subject             string    `json:"subject"`
	PlainText           string    `json:"plain_text"`
	IncludeCalendar     bool      `json:"include_calendar"`
	ProviderRevision    string    `json:"provider_revision"`
}

// ApprovedCampaign is the transaction-bound execution seam for #82. Resolving
// it never enqueues or sends. The caller retains its actual recipient attempts.
type ApprovedCampaign struct {
	ScopeApproval *ScopeApproval
	Approval      CampaignApproval
	Purpose       string
	Format        string
	Definition    CampaignDefinition
	Version       int64
	DueAt         time.Time
	Preview       Preview
}

// BroadcastMessage contains one common literal projection and no recipient list.
// The adapter must compare its current connection version with the approval's
// channel revision in the same transaction that records a delivery attempt.
type BroadcastMessage struct {
	ScopeApproval   *ScopeApproval
	EventID         uuid.UUID
	CampaignID      uuid.UUID
	CampaignVersion int64
	Approval        CampaignApproval
	DueAt           time.Time
	Purpose         string
	Subject         string
	PlainText       string
}
