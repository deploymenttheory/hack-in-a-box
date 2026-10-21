package permissions

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// Relinquish is used by an authenticated person's own application withdrawal.
// The caller owns the transaction and verifies the application's person ID.
func Relinquish(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, person uuid.UUID,
	role string,
) error {
	if !slices.Contains(Roles, role) {
		return access.ErrInvalid
	}
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("lock staff withdrawal: %w", err)
	}
	if role == "organiser" {
		count, err := q.ActiveOrganisers(ctx, eventID)
		if err != nil {
			return fmt.Errorf("count withdrawal organisers: %w", err)
		}
		if (row.OwnerID != nil && *row.OwnerID == person) || count <= 1 {
			return ErrLastOrganiser
		}
	}
	if err := q.SetMembership(
		ctx,
		dbgen.SetMembershipParams{EventID: eventID, PersonID: person, Role: role, Active: false},
	); err != nil {
		return fmt.Errorf("relinquish selected role: %w", err)
	}
	if err := audit.Append(
		ctx,
		q,
		audit.Record{
			ActorID:  &person,
			EventID:  &eventID,
			TargetID: person,
			Action:   audit.RoleRevoked,
			Reason:   audit.UserRequest,
			Metadata: audit.Metadata{Role: &role},
		},
	); err != nil {
		return fmt.Errorf("audit staff withdrawal: %w", err)
	}
	return nil
}
