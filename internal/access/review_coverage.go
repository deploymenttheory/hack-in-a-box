package access

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// ReviewCoverageAuthority projects the current authority to open review coverage.
// It reads no roster, assessment or private coordination content. The owning
// service still validates its configuration and restricts the returned content.
// A nil error means allowed; the boolean identifies the organiser projection.
// An allowed mentor returns false with a nil error.
func ReviewCoverageAuthority(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID uuid.UUID,
) (bool, error) {
	if err := (EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		EventRead,
	); err != nil {
		return false, fmt.Errorf("authorize review coverage event: %w", err)
	}
	roles, _, err := ContentReader(ctx, q, actor, eventID)
	if err != nil {
		return false, fmt.Errorf("read current review coverage role: %w", err)
	}
	organiser := slices.Contains(roles, "organiser")
	if !organiser && !slices.Contains(roles, "mentor") {
		return false, ErrForbidden
	}
	return organiser, nil
}
