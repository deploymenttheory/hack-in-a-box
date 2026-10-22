package communications

import (
	"context"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// FactOwner checks a real owning-domain source under the caller's event-locked
// transaction. A registered owner is authoritative: unknown/stale source data
// must deny, never fall back to a generic owner ID. Implementations return no
// private source prose and cannot supply recipients or destination URLs.
type FactOwner interface {
	AuthorizeCommunicationFact(
		context.Context,
		*dbgen.Queries,
		uuid.UUID,
		dbgen.CommunicationFact,
	) (bool, error)
}
