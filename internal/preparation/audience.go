package preparation

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// OutstandingAssigned resolves current preparation ownership without exporting
// task text or evidence. The caller supplies queries bound to its event-locked
// transaction, so audience selection and recipient-attempt recording share the
// same authority snapshot. Seed tasks use the ordinary preparation merge rules.
func OutstandingAssigned(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID uuid.UUID,
) (bool, error) {
	roles, err := authority(ctx, q, actor, eventID)
	if errors.Is(err, access.ErrForbidden) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	row, err := q.Event(ctx, eventID)
	if err != nil {
		return false, fmt.Errorf("read preparation audience event: %w", err)
	}
	tasks, _, _, err := load(ctx, q, row)
	if err != nil {
		return false, err
	}
	for _, task := range tasks {
		if assigned(task.Definition, actor, roles) && !terminal(task.Definition.Status) {
			return true, nil
		}
	}
	return false, nil
}
