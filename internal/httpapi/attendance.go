package httpapi

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/attendance"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

func (h handlers) GetAttendance(
	ctx context.Context,
	r contract.GetAttendanceRequestObject,
) (contract.GetAttendanceResponseObject, error) {
	day := ""
	if r.Params.Day != nil {
		day = *r.Params.Day
	}
	result, err := h.Attendance.Read(ctx, actor(ctx).ID, r.EventId, day)
	if err != nil {
		return nil, fmt.Errorf("attendance roster: %w", err)
	}
	return convertDTO[contract.GetAttendance200JSONResponse](result)
}

func (h handlers) ConfigureCatering(
	ctx context.Context,
	r contract.ConfigureCateringRequestObject,
) (contract.ConfigureCateringResponseObject, error) {
	if r.Body == nil || r.Body.Buffer < 0 || r.Body.Buffer > 10000 {
		return nil, access.ErrInvalid
	}
	result, err := h.Attendance.Configure(
		ctx,
		actor(ctx).ID,
		r.EventId,
		revision(r.Params.IfMatch),
		int32(r.Body.Buffer),
	)
	if err != nil {
		return nil, fmt.Errorf("catering settings: %w", err)
	}
	return convertDTO[contract.ConfigureCatering200JSONResponse](result)
}

func (h handlers) RecordAttendance(
	ctx context.Context,
	r contract.RecordAttendanceRequestObject,
) (contract.RecordAttendanceResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	command, err := convertDTO[attendance.Command](r.Body)
	if err != nil {
		return nil, err
	}
	result, err := h.Attendance.Mark(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.PersonId,
		r.Day,
		revision(r.Params.IfMatch),
		command,
	)
	if err != nil {
		return nil, fmt.Errorf("arrival update: %w", err)
	}
	return convertDTO[contract.RecordAttendance200JSONResponse](result)
}

func (h handlers) AttendanceHistory(
	ctx context.Context,
	r contract.AttendanceHistoryRequestObject,
) (contract.AttendanceHistoryResponseObject, error) {
	result, err := h.Attendance.History(ctx, actor(ctx).ID, r.EventId, r.PersonId, r.Day)
	if err != nil {
		return nil, fmt.Errorf("arrival history: %w", err)
	}
	items, err := convertDTO[[]contract.AttendanceChange](result)
	if err != nil {
		return nil, err
	}
	return contract.AttendanceHistory200JSONResponse{Items: items}, nil
}

func (h handlers) GetLogistics(
	ctx context.Context,
	r contract.GetLogisticsRequestObject,
) (contract.GetLogisticsResponseObject, error) {
	cursor := uuid.Nil
	if r.Params.After != nil {
		cursor = *r.Params.After
	}
	_, limit := page(nil, r.Params.Limit)
	result, err := h.Attendance.Logistics(ctx, actor(ctx).ID, r.EventId, cursor, limit, false)
	if err != nil {
		return nil, fmt.Errorf("operational roster: %w", err)
	}
	return convertDTO[contract.GetLogistics200JSONResponse](result)
}

func (h handlers) ExportLogistics(
	ctx context.Context,
	r contract.ExportLogisticsRequestObject,
) (contract.ExportLogisticsResponseObject, error) {
	cursor := uuid.Nil
	if r.Params.After != nil {
		cursor = *r.Params.After
	}
	_, limit := page(nil, r.Params.Limit)
	result, err := h.Attendance.Logistics(ctx, actor(ctx).ID, r.EventId, cursor, limit, true)
	if err != nil {
		return nil, fmt.Errorf("operational roster export: %w", err)
	}
	body, err := convertDTO[contract.LogisticsPage](result)
	if err != nil {
		return nil, err
	}
	disposition := "attachment; filename=logistics.json"
	return contract.ExportLogistics200JSONResponse{
		Body:    body,
		Headers: contract.ExportLogistics200ResponseHeaders{ContentDisposition: &disposition},
	}, nil
}
