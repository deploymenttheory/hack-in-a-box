package communications

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// OutcomePlans is registered only by the native outcome owner. Its approval is
// separate from generic campaign approval and cannot be supplied by HTTP input.
// Execution rechecks current source, owner, approver, retention and providers;
// historical read checks deliberately do not require a still-pending reminder.
type OutcomePlans interface {
	InspectOutcomePlanInTransaction(
		context.Context,
		pgx.Tx,
		uuid.UUID,
		uuid.UUID,
		uuid.UUID,
		int64,
	) (NativeOutcomePlanState, error)
	ResolveOutcomePlanInTransaction(
		context.Context,
		pgx.Tx,
		uuid.UUID,
		uuid.UUID,
		int64,
		string,
		uuid.UUID,
		bool,
		bool,
	) (ApprovedCampaign, error)
	AuthorizeOutcomeInboxInTransaction(
		context.Context,
		pgx.Tx,
		uuid.UUID,
		uuid.UUID,
		InboxProvenance,
	) error
}

// NativeOutcomePlanState is returned only after the owning domain has checked
// the command actor against the current event and exact native plan revision.
type NativeOutcomePlanState struct {
	ID          uuid.UUID
	Version     int64
	State       string
	ActorID     uuid.UUID
	DueAt       time.Time
	RetainUntil time.Time
	Approval    *ScopeApproval
}
