package privacy

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// OperationalFieldsStateInTransaction evaluates the existing calendar retention
// deadline before an archive export reads private fields. Expiry does not wait
// for deletion maintenance; this read neither purges records nor creates policy.
// The caller holds the event lock and separately enforces the resource policy.
func OperationalFieldsStateInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	event dbgen.Event,
	now time.Time,
) (string, error) {
	if event.Stage != "completed" && event.Stage != "cancelled" && event.Stage != "archived" {
		return "available", nil
	}
	if !event.CompletedAt.Valid {
		return "unavailable", nil
	}
	settings, err := dbgen.New(tx).PrivacyPolicy(ctx, event.ID)
	if err != nil {
		return "", fmt.Errorf("read current operational retention: %w", err)
	}
	if !RetentionDeadline(event.CompletedAt.Time, settings.RetentionDays).After(now) {
		return "retention_expired", nil
	}
	return "available", nil
}
