package smtpintegration

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
)

// The native session owner calls this before replacing its current approval
// source. A changed configuration cannot erase or replay an original uncertain
// SMTP attempt; its existing provider review/disposition remains authoritative.
func (s Service) CheckSessionNotificationApprovalInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, sessionID uuid.UUID,
) error {
	var unresolved bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM smtp_campaign_messages m JOIN session_notification_occurrences n ON n.event_id=m.event_id AND n.id=m.occurrence_id JOIN operations o ON o.id=m.operation_id WHERE n.event_id=$1 AND n.session_id=$2 AND (o.state IN ('pending','running') OR NOT o.cancelled AND (m.state='prepared' OR o.state='ambiguous')))`, eventID, sessionID).
		Scan(&unresolved); err != nil {
		return fmt.Errorf("check original session SMTP effects: %w", err)
	}
	if unresolved {
		return access.ErrConflict
	}
	return nil
}
