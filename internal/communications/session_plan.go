package communications

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SessionPlans is implemented only by the native session owner. A typed approval
// covers its versioned update/cancellation contract, never arbitrary recipients,
// prose or URLs supplied to a generic campaign. Cancellation after event
// withdrawal has a separate minimal own-booking projection.
type SessionPlans interface {
	ResolveSessionNotificationInTransaction(
		context.Context,
		pgx.Tx,
		uuid.UUID,
		uuid.UUID,
		int64,
		ScopeApproval,
		uuid.UUID,
	) (ApprovedCampaign, error)
	AuthorizeSessionInboxInTransaction(
		context.Context,
		pgx.Tx,
		uuid.UUID,
		uuid.UUID,
		ScopeApproval,
	) (bool, error)
}

type NativeSessionEffectGuard interface {
	CheckSessionNotificationApprovalInTransaction(
		context.Context,
		pgx.Tx,
		uuid.UUID,
		uuid.UUID,
	) error
}
