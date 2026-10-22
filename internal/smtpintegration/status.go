package smtpintegration

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
)

type DeliveryStatus struct {
	State     string     `json:"state"`
	Reason    string     `json:"reason"`
	UpdatedAt *time.Time `json:"updated_at"`
}

// Status exposes bounded receipt metadata only to the current requesting
// organiser. It never returns recipient addresses, rendered content or headers.
func (s Service) Status(
	ctx context.Context,
	actor, eventID, testID uuid.UUID,
) (DeliveryStatus, error) {
	q := dbgen.New(s.Pool)
	if err := (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		access.StaffRead,
	); err != nil {
		return DeliveryStatus{}, fmt.Errorf("authorize mail receipt: %w", err)
	}
	roles, err := q.ActiveRoles(ctx, dbgen.ActiveRolesParams{EventID: eventID, PersonID: actor})
	if err != nil {
		return DeliveryStatus{}, fmt.Errorf("read mail receipt roles: %w", err)
	}
	if !slices.Contains(roles, "organiser") {
		return DeliveryStatus{}, access.ErrForbidden
	}
	request, err := q.CommunicationTestRequest(
		ctx,
		dbgen.CommunicationTestRequestParams{EventID: eventID, ID: testID},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeliveryStatus{}, access.ErrNotFound
	}
	if err != nil {
		return DeliveryStatus{}, fmt.Errorf("read mail test receipt: %w", err)
	}
	if request.ActorID != actor || request.Channel != "smtp" {
		return DeliveryStatus{}, access.ErrForbidden
	}
	operation, err := q.SMTPTestOperation(
		ctx,
		dbgen.SMTPTestOperationParams{EventID: eventID, TargetID: testID},
	)
	if err != nil {
		return DeliveryStatus{}, fmt.Errorf("read mail operation status: %w", err)
	}
	updated := operation.UpdatedAt
	result := DeliveryStatus{State: "pending", Reason: "awaiting_submission", UpdatedAt: &updated}
	stored, err := q.SMTPMessageSnapshot(ctx, operation.ID)
	if err == nil {
		updated = stored.UpdatedAt
		if stored.State == "accepted" || stored.State == "ambiguous" ||
			stored.State == "permanent" {
			return DeliveryStatus{
				State: stored.State,
				Reason: maildelivery.SafeReceipt(
					maildelivery.Receipt{
						State: maildelivery.State(stored.State),
						Code:  stored.ReceiptCode,
					},
				).Code,
				UpdatedAt: &updated,
			}, nil
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return DeliveryStatus{}, fmt.Errorf("read bounded SMTP receipt: %w", err)
	}
	switch {
	case operation.State == "ambiguous":
		result.State = "ambiguous"
		result.Reason = "acceptance_unknown"
	case (operation.Cancelled || operation.State == "cancelled") && err == nil && stored.State == "prepared":
		result.State = "ambiguous"
		result.Reason = "acceptance_unknown"
	case operation.Cancelled || operation.State == "cancelled":
		result.State = "cancelled"
		result.Reason = "request_cancelled"
	case err == nil && stored.State == "retryable":
		result.State = "retryable"
		result.Reason = maildelivery.SafeReceipt(
			maildelivery.Receipt{State: maildelivery.State(stored.State), Code: stored.ReceiptCode},
		).Code
	case operation.State == "permanent":
		result.State = "permanent"
		result.Reason = "request_unavailable"
	}
	return result, nil
}
