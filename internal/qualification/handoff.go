package qualification

import (
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

// Selection identifies one exact current private qualification signoff. It is
// source metadata for results preparation, never an approved/released result.
type Selection struct {
	RunID         uuid.UUID          `json:"run_id"`
	RoundVersion  int64              `json:"round_version"`
	Reference     rubric.Reference   `json:"reference"`
	SignedBinding string             `json:"signed_binding"`
	SignerID      uuid.UUID          `json:"signer_id"`
	SignedAt      time.Time          `json:"signed_at"`
	FinalistIDs   []uuid.UUID        `json:"finalist_ids"`
	ReserveIDs    []uuid.UUID        `json:"reserve_ids"`
	Categories    []CategoryProposal `json:"categories"`
}
