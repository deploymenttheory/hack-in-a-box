package outcomes

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
)

// The native review composition is versioned with the plan, independently of a
// generic campaign. Only an organiser's explicit approval authorises execution.
// It never imports draft event variables into an ordinary owner's message.
type ReviewMessage struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type ReviewProof struct {
	PlanID               uuid.UUID         `json:"plan_id"`
	PlanVersion          int64             `json:"plan_version"`
	OutcomeID            uuid.UUID         `json:"outcome_id"`
	OutcomeVersion       int64             `json:"outcome_version"`
	OwnerID              uuid.UUID         `json:"owner_id"`
	ActorID              uuid.UUID         `json:"actor_id"`
	ConfigurationBinding string            `json:"configuration_binding"`
	PrivacyRevision      int64             `json:"privacy_revision"`
	Template             ReviewMessage     `json:"template"`
	Channels             map[string]string `json:"channels"`
	DueAt                time.Time         `json:"due_at"`
	RetainUntil          time.Time         `json:"retain_until"`
	Binding              string            `json:"binding"`
}

type PlanView struct {
	ReviewPlan
	CurrentVersion int64    `json:"current_version"`
	EffectiveState string   `json:"effective_state"`
	Blockers       []string `json:"blockers"`
}

type PlanPage struct {
	Items []PlanView `json:"items"`
}

func proofDigest(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (m ReviewMessage) valid() bool {
	if !required(m.Subject, 500) || !required(m.Body, 20000) {
		return false
	}
	values := map[string]string{
		"outcome.url":    "/ideas/one",
		"review.date":    "A reviewed date",
		"recipient.name": "Current owner",
	}
	_, a := communications.Render(m.Subject, "text", values)
	_, b := communications.Render(m.Body, "text", values)
	return len(a) == 0 && len(b) == 0
}
