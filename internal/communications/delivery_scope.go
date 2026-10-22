package communications

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// DeliveryScope identifies a durable, explicitly approved automation occurrence.
// It contains no caller-selected recipients, message content or source facts.
// A zero scope is reserved for the existing one-shot campaign queue entrypoint.
type DeliveryScope struct {
	OccurrenceID uuid.UUID
}

// ScopeAudience is derived by the automation owner from the current approved
// trigger and authoritative source. Providers intersect these identities with
// the current approved campaign selector, never union them into that selector.
// Binding identifies immutable occurrence/approval/source intent; it excludes
// provider secrets and private content. Broadcast is false for private triggers.
type ScopeAudience struct {
	Binding         string
	RecipientIDs    []uuid.UUID
	Broadcast       bool
	RequireComplete bool
	AllowArchived   bool
	Approval        *ScopeApproval
	Review          *ScopeReview
}

// ScopePolicy must lock the event in tx and recheck the current occurrence,
// approval, source scope/version, pause, cancellation and missed-action review.
// Providers fail closed when a nonzero scope has no configured policy. Deferred
// external effects must call it again immediately before preparing their send.
// AllowArchived is restricted to an explicitly retained post-event follow-up;
// it does not itself bypass campaign, source or jobs authorization.
type ScopePolicy interface {
	ResolveInTransaction(
		context.Context,
		pgx.Tx,
		uuid.UUID,
		uuid.UUID,
		int64,
		DeliveryScope,
	) (ScopeAudience, error)
}

// ScopeApproval names an immutable typed lifecycle approval. It never changes
// the approval status of the generic campaign draft.
type ScopeApproval struct {
	Kind          string    `json:"kind"`
	ID            uuid.UUID `json:"id"`
	Binding       string    `json:"binding"`
	SourceID      uuid.UUID `json:"source_id"`
	SourceVersion int64     `json:"source_version"`
}

// SameScopeApproval compares immutable typed provenance without pointer identity.
func SameScopeApproval(left, right *ScopeApproval) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

// ScopeReview is internal proof of a current explicit review generation. The
// canonical scope owner derives it from persisted state; no HTTP input supplies it.
// It permits reuse of an identical historical inbox identity, never rewriting it.
type ScopeReview struct {
	OccurrenceID uuid.UUID
	Revision     int64
	Binding      string
}
