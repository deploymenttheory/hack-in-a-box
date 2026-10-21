package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
)

// convertDTO keeps HTTP types at the transport boundary. Domain DTOs have a
// deliberately explicit JSON field set; contract tests validate the wire shape.
func convertDTO[T any](value any) (T, error) {
	var result T
	data, err := json.Marshal(value)
	if err != nil {
		return result, fmt.Errorf("encode transport value: %w", err)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("decode transport value: %w", err)
	}
	return result, nil
}

func (h handlers) ListEvents(
	ctx context.Context,
	r contract.ListEventsRequestObject,
) (contract.ListEventsResponseObject, error) {
	cursor := uuid.Nil
	if r.Params.After != nil {
		cursor = *r.Params.After
	}
	_, limit := page(nil, r.Params.Limit)
	rows, err := h.Events.List(ctx, actor(ctx).ID, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	items, err := convertDTO[[]contract.Event](rows)
	if err != nil {
		return nil, err
	}
	result := contract.EventPage{Items: items}
	if len(rows) == int(limit) {
		id := rows[len(rows)-1].ID
		result.NextCursor = &id
	}
	return contract.ListEvents200JSONResponse(result), nil
}

func (h handlers) CreateEvent(
	ctx context.Context,
	r contract.CreateEventRequestObject,
) (contract.CreateEventResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[event.Input](r.Body)
	if err != nil {
		return nil, err
	}
	result, err := h.Events.Create(ctx, actor(ctx).ID, input)
	if err != nil {
		return nil, fmt.Errorf("create event: %w", err)
	}
	response, err := convertDTO[contract.CreateEvent200JSONResponse](result)
	return response, err
}

func (h handlers) GetEvent(
	ctx context.Context,
	r contract.GetEventRequestObject,
) (contract.GetEventResponseObject, error) {
	result, err := h.Events.Read(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("get event: %w", err)
	}
	response, err := convertDTO[contract.GetEvent200JSONResponse](result)
	return response, err
}

func (h handlers) UpdateEvent(
	ctx context.Context,
	r contract.UpdateEventRequestObject,
) (contract.UpdateEventResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[event.Input](r.Body)
	if err != nil {
		return nil, err
	}
	result, err := h.Events.Update(ctx, actor(ctx).ID, r.EventId, revision(r.Params.IfMatch), input)
	if err != nil {
		return nil, fmt.Errorf("update event: %w", err)
	}
	response, err := convertDTO[contract.UpdateEvent200JSONResponse](result)
	return response, err
}

func (h handlers) TransitionEvent(
	ctx context.Context,
	r contract.TransitionEventRequestObject,
) (contract.TransitionEventResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	result, err := h.Events.Transition(
		ctx,
		actor(ctx).ID,
		r.EventId,
		revision(r.Params.IfMatch),
		string(r.Body.Stage),
		r.Body.Reason,
		r.Body.AttendanceException,
	)
	if err != nil {
		return nil, fmt.Errorf("transition event: %w", err)
	}
	response, err := convertDTO[contract.TransitionEvent200JSONResponse](result)
	return response, err
}

func (h handlers) GetEventSchedule(
	ctx context.Context,
	r contract.GetEventScheduleRequestObject,
) (contract.GetEventScheduleResponseObject, error) {
	rows, err := h.Events.Schedule(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("get schedule: %w", err)
	}
	items, err := convertDTO[[]contract.ScheduleItem](rows)
	if err != nil {
		return nil, err
	}
	return contract.GetEventSchedule200JSONResponse{Items: items}, nil
}

func (h handlers) ApproveEventSchedule(
	ctx context.Context,
	r contract.ApproveEventScheduleRequestObject,
) (contract.ApproveEventScheduleResponseObject, error) {
	if err := h.Events.ApproveSchedule(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.ItemId,
		revision(r.Params.IfMatch),
	); err != nil {
		return nil, fmt.Errorf("approve schedule: %w", err)
	}
	rows, err := h.Events.Schedule(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("read approved schedule: %w", err)
	}
	items, err := convertDTO[[]contract.ScheduleItem](rows)
	if err != nil {
		return nil, err
	}
	return contract.ApproveEventSchedule200JSONResponse{Items: items}, nil
}

func (h handlers) PreviewEvent(
	_ context.Context,
	r contract.PreviewEventRequestObject,
) (contract.PreviewEventResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[event.Input](r.Body)
	if err != nil {
		return nil, err
	}
	issues := event.Validate(input, true)
	blockers, err := convertDTO[[]contract.Blocker](issues)
	if err != nil {
		return nil, err
	}
	result := contract.EventPreview{Blockers: blockers, Schedule: []contract.ScheduleItem{}}
	if !input.Configuration.LiveStart.IsZero() {
		if instant, err := schedule.ChallengePublication(
			input.Configuration.LiveStart,
			input.Configuration.Timezone,
			input.Configuration.DSTChoice,
		); err == nil {
			result.ChallengePublication = &instant
		}
	}
	if rows, err := event.Preview(input.Configuration, time.Now()); err == nil {
		items, err := convertDTO[[]contract.ScheduleItem](rows)
		if err != nil {
			return nil, err
		}
		result.Schedule = items
	}
	return contract.PreviewEvent200JSONResponse(result), nil
}

func (h handlers) ResolveEventTime(
	_ context.Context,
	r contract.ResolveEventTimeRequestObject,
) (contract.ResolveEventTimeResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	instant, err := schedule.ResolveLocal(
		r.Body.WallTime,
		r.Body.Timezone,
		string(r.Body.DstChoice),
	)
	if err != nil {
		return nil, &event.Blocked{
			Issues: []event.Issue{{Field: "local_time", Code: "local_time", Message: err.Error()}},
		}
	}
	return contract.ResolveEventTime200JSONResponse{Instant: instant}, nil
}

func (h handlers) ListEventDecisions(
	ctx context.Context,
	r contract.ListEventDecisionsRequestObject,
) (contract.ListEventDecisionsResponseObject, error) {
	rows, err := h.Events.Decisions(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("list decisions: %w", err)
	}
	result := contract.EventDecisionPage{Items: make([]contract.EventDecision, 0, len(rows))}
	for _, row := range rows {
		result.Items = append(
			result.Items,
			contract.EventDecision{
				Id:        row.ID,
				ActorId:   row.ActorID,
				Kind:      row.Kind,
				Reason:    row.Reason,
				Revision:  row.Revision,
				CreatedAt: row.CreatedAt,
			},
		)
	}
	return contract.ListEventDecisions200JSONResponse(result), nil
}
