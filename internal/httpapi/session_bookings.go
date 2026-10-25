package httpapi

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/sessionbooking"
)

func (h handlers) ListBookableSessions(
	ctx context.Context,
	r contract.ListBookableSessionsRequestObject,
) (contract.ListBookableSessionsResponseObject, error) {
	cursor := uuid.Nil
	if r.Params.After != nil {
		cursor = *r.Params.After
	}
	_, limit := page(nil, r.Params.Limit)
	own := r.Params.Own != nil && *r.Params.Own
	result, err := h.SessionBookings.List(ctx, actor(ctx).ID, r.EventId, cursor, int(limit), own)
	if err != nil {
		return nil, fmt.Errorf("session operation: %w", err)
	}
	return convertDTO[contract.ListBookableSessions200JSONResponse](result)
}

func (h handlers) GetBookableSession(
	ctx context.Context,
	r contract.GetBookableSessionRequestObject,
) (contract.GetBookableSessionResponseObject, error) {
	result, err := h.SessionBookings.Read(ctx, actor(ctx).ID, r.EventId, r.SessionId)
	if err != nil {
		return nil, fmt.Errorf("session operation: %w", err)
	}
	return convertDTO[contract.GetBookableSession200JSONResponse](result)
}

func (h handlers) ConfigureBookableSession(
	ctx context.Context,
	r contract.ConfigureBookableSessionRequestObject,
) (contract.ConfigureBookableSessionResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	definition, err := convertDTO[sessionbooking.Definition](r.Body)
	if err != nil {
		return nil, err
	}
	result, err := h.SessionBookings.Configure(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.SessionId,
		revision(r.Params.IfMatch),
		definition,
	)
	if err != nil {
		return nil, fmt.Errorf("session operation: %w", err)
	}
	return convertDTO[contract.ConfigureBookableSession200JSONResponse](result)
}

func (h handlers) ChangeSessionBooking(
	ctx context.Context,
	r contract.ChangeSessionBookingRequestObject,
) (contract.ChangeSessionBookingResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	command, err := convertDTO[sessionbooking.Command](r.Body)
	if err != nil {
		return nil, err
	}
	command.ExpectedRevision = revision(r.Params.IfMatch)
	result, err := h.SessionBookings.Change(ctx, actor(ctx).ID, r.EventId, r.SessionId, command)
	if err != nil {
		return nil, fmt.Errorf("session operation: %w", err)
	}
	return convertDTO[contract.ChangeSessionBooking200JSONResponse](result)
}

func (h handlers) CancelBookableSession(
	ctx context.Context,
	r contract.CancelBookableSessionRequestObject,
) (contract.CancelBookableSessionResponseObject, error) {
	result, err := h.SessionBookings.CancelSession(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.SessionId,
		revision(r.Params.IfMatch),
	)
	if err != nil {
		return nil, fmt.Errorf("session operation: %w", err)
	}
	return convertDTO[contract.CancelBookableSession200JSONResponse](result)
}

func (h handlers) GetSessionBookingReceipt(
	ctx context.Context,
	r contract.GetSessionBookingReceiptRequestObject,
) (contract.GetSessionBookingReceiptResponseObject, error) {
	result, err := h.SessionBookings.OriginalReceipt(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.SessionId,
		r.RequestId,
	)
	if err != nil {
		return nil, fmt.Errorf("session operation: %w", err)
	}
	return convertDTO[contract.GetSessionBookingReceipt200JSONResponse](result)
}

func (h handlers) GetSessionAttendance(
	ctx context.Context,
	r contract.GetSessionAttendanceRequestObject,
) (contract.GetSessionAttendanceResponseObject, error) {
	items, err := h.SessionBookings.Roster(ctx, actor(ctx).ID, r.EventId, r.SessionId)
	if err != nil {
		return nil, fmt.Errorf("session operation: %w", err)
	}
	rows, err := convertDTO[[]contract.SessionAttendanceEntry](items)
	if err != nil {
		return nil, err
	}
	return contract.GetSessionAttendance200JSONResponse{Items: rows}, nil
}

func (h handlers) RecordSessionAttendance(
	ctx context.Context,
	r contract.RecordSessionAttendanceRequestObject,
) (contract.RecordSessionAttendanceResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	command, err := convertDTO[sessionbooking.AttendanceCommand](r.Body)
	if err != nil {
		return nil, err
	}
	result, err := h.SessionBookings.Mark(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.SessionId,
		r.PersonId,
		revision(r.Params.IfMatch),
		command,
	)
	if err != nil {
		return nil, fmt.Errorf("session operation: %w", err)
	}
	return convertDTO[contract.RecordSessionAttendance200JSONResponse](result)
}

func (h handlers) PreviewSessionNotifications(
	ctx context.Context,
	r contract.PreviewSessionNotificationsRequestObject,
) (contract.PreviewSessionNotificationsResponseObject, error) {
	result, err := h.SessionBookings.PreviewNotifications(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.SessionId,
	)
	if err != nil {
		return nil, fmt.Errorf("preview session notifications: %w", err)
	}
	return convertDTO[contract.PreviewSessionNotifications200JSONResponse](result)
}

func (h handlers) ConfigureSessionNotifications(
	ctx context.Context,
	r contract.ConfigureSessionNotificationsRequestObject,
) (contract.ConfigureSessionNotificationsResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	channels, err := convertDTO[[]string](r.Body.Channels)
	if err != nil {
		return nil, err
	}
	result, err := h.SessionBookings.SaveNotificationConfiguration(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.SessionId,
		revision(r.Params.IfMatch),
		channels,
	)
	if err != nil {
		return nil, fmt.Errorf("configure session notifications: %w", err)
	}
	return convertDTO[contract.ConfigureSessionNotifications200JSONResponse](result)
}

func (h handlers) ApproveSessionNotifications(
	ctx context.Context,
	r contract.ApproveSessionNotificationsRequestObject,
) (contract.ApproveSessionNotificationsResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	result, err := h.SessionBookings.ApproveNotifications(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.SessionId,
		r.Body.Version,
		r.Body.Fingerprint,
	)
	if err != nil {
		return nil, fmt.Errorf("approve session notifications: %w", err)
	}
	return convertDTO[contract.ApproveSessionNotifications200JSONResponse](result)
}

func (h handlers) GetSessionNotificationHistory(
	ctx context.Context,
	r contract.GetSessionNotificationHistoryRequestObject,
) (contract.GetSessionNotificationHistoryResponseObject, error) {
	var after int64
	if r.Params.After != nil {
		after = *r.Params.After
	}
	_, limit := page(nil, r.Params.Limit)
	result, err := h.SessionBookings.NotificationHistory(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.SessionId,
		after,
		int(limit),
	)
	if err != nil {
		return nil, fmt.Errorf("session notification history: %w", err)
	}
	return convertDTO[contract.GetSessionNotificationHistory200JSONResponse](result)
}

func (h handlers) GetOwnSessionCancellation(
	ctx context.Context,
	r contract.GetOwnSessionCancellationRequestObject,
) (contract.GetOwnSessionCancellationResponseObject, error) {
	result, err := h.SessionBookings.OwnCancellation(ctx, actor(ctx).ID, r.EventId, r.SessionId)
	if err != nil {
		return nil, fmt.Errorf("own session cancellation: %w", err)
	}
	return convertDTO[contract.GetOwnSessionCancellation200JSONResponse](result)
}
