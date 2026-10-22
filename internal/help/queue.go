package help

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

type Summary struct {
	ID                uuid.UUID  `json:"id"`
	TeamID            uuid.UUID  `json:"team_id"`
	Category          string     `json:"category"`
	Status            string     `json:"status"`
	AssigneeID        *uuid.UUID `json:"assignee_id"`
	RequestedAt       time.Time  `json:"requested_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	WaitingSeconds    int64      `json:"waiting_seconds"`
	ResponseSeconds   *int64     `json:"response_seconds"`
	CompletionSeconds *int64     `json:"completion_seconds"`
	Overdue           bool       `json:"overdue"`
}
type Queue struct {
	Items     []Summary `json:"items"`
	Waiting   int       `json:"waiting"`
	Claimed   int       `json:"claimed"`
	Resolved  int       `json:"resolved"`
	Cancelled int       `json:"cancelled"`
	CanManage bool      `json:"can_manage"`
}

func summary(r Request, settings Settings, now time.Time) Summary {
	item := Summary{
		ID:          r.ID,
		TeamID:      r.TeamID,
		Category:    r.Category,
		Status:      r.Status,
		AssigneeID:  r.AssigneeID,
		RequestedAt: r.RequestedAt,
		UpdatedAt:   r.UpdatedAt,
	}
	if r.Status == "waiting" {
		item.WaitingSeconds = max(0, int64(now.Sub(r.WaitingSince)/time.Second))
		item.Overdue = !now.Before(r.WaitingSince.Add(threshold(settings, r.Category)))
	}
	if r.FirstClaimedAt != nil {
		seconds := max(0, int64(r.FirstClaimedAt.Sub(r.RequestedAt)/time.Second))
		item.ResponseSeconds = &seconds
	}
	if r.ResolvedAt != nil {
		seconds := max(0, int64(r.ResolvedAt.Sub(r.RequestedAt)/time.Second))
		item.CompletionSeconds = &seconds
	}
	return item
}

func (s Service) List(ctx context.Context, actor, eventID uuid.UUID) (Queue, error) {
	result := Queue{Items: []Summary{}}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := lock(ctx, q, eventID)
		if err != nil {
			return err
		}
		if err = (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.EventRead,
		); err != nil {
			return wrap("authorize help queue", err)
		}
		config, err := settings(ctx, q, eventID)
		if err != nil {
			return err
		}
		roles, err := q.ActiveRoles(ctx, dbgen.ActiveRolesParams{EventID: eventID, PersonID: actor})
		if err != nil {
			return wrap("read queue roles", err)
		}
		for _, role := range roles {
			if role == "organiser" {
				result.CanManage = row.WritesAllowed
			}
		}
		rows, err := helpRequests(ctx, q, eventID)
		if err != nil {
			return wrap("load help queue", err)
		}
		for _, record := range rows {
			r := request(record)
			if _, err = s.authorizeRead(ctx, q, row, actor, r); err != nil {
				if errors.Is(err, access.ErrNotFound) || errors.Is(err, event.ErrBlocked) {
					continue
				}
				return err
			}
			var changed bool
			r, changed, err = s.reconcile(ctx, q, row, r)
			if err != nil {
				return err
			}
			if changed {
				if err = s.emit(ctx, tx, q, row, r); err != nil {
					return err
				}
			}
			result.Items = append(result.Items, summary(r, config, s.now()))
			switch r.Status {
			case "waiting":
				result.Waiting++
			case "claimed":
				result.Claimed++
			case "resolved":
				result.Resolved++
			case "cancelled":
				result.Cancelled++
			}
		}
		return nil
	})
	return result, wrap("read help queue", err)
}

func (s Service) Settings(ctx context.Context, actor, eventID uuid.UUID) (Settings, error) {
	var result Settings
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, err := lock(ctx, q, eventID); err != nil {
			return err
		}
		if err := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.EventRead,
		); err != nil {
			return wrap("authorize help settings", err)
		}
		roles, err := q.ActiveRoles(ctx, dbgen.ActiveRolesParams{EventID: eventID, PersonID: actor})
		if err != nil {
			return wrap("read settings roles", err)
		}
		organiser := false
		for _, role := range roles {
			organiser = organiser || role == "organiser"
		}
		if !organiser {
			return access.ErrForbidden
		}
		result, err = settings(ctx, q, eventID)
		return err
	})
	return result, wrap("read help settings", err)
}

func (s Service) Configure(
	ctx context.Context,
	actor, eventID uuid.UUID,
	revision int64,
	input Settings,
) (Settings, error) {
	var result Settings
	if err := validateSettings(input); err != nil {
		return result, err
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := lock(ctx, q, eventID)
		if err != nil {
			return err
		}
		if err = (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.EventEdit,
		); err != nil {
			return wrap("authorize help configuration", err)
		}
		if !row.WritesAllowed {
			return access.ErrForbidden
		}
		current, err := settings(ctx, q, eventID)
		if err != nil {
			return err
		}
		if current.Revision != revision {
			return access.ErrConflict
		}
		input.Revision = current.Revision + 1
		if input.MeetingHosts == nil {
			input.MeetingHosts = []string{}
		}
		if err = q.SetHelpSettings(
			ctx,
			dbgen.SetHelpSettingsParams{
				EventID:                   eventID,
				Revision:                  input.Revision,
				SetupThresholdSeconds:     input.SetupThresholdSeconds,
				LogisticsThresholdSeconds: input.LogisticsThresholdSeconds,
				OutputThresholdSeconds:    input.OutputThresholdSeconds,
				MeetingHosts:              input.MeetingHosts,
			},
		); err != nil {
			return wrap("configure help follow-up", err)
		}
		result = input
		return nil
	})
	return result, wrap("configure help settings", err)
}

// MaintainInTransaction creates each due operational follow-up once per waiting
// cycle and threshold version. The outer scheduler supplies its event lock;
// this method owns no unattended scheduler or external provider delivery.
func (s Service) MaintainInTransaction(ctx context.Context, tx pgx.Tx, eventID uuid.UUID) error {
	q := dbgen.New(tx)
	row, err := lock(ctx, q, eventID)
	if err != nil {
		return err
	}
	if !row.WritesAllowed {
		return nil
	}
	config, err := settings(ctx, q, eventID)
	if err != nil {
		return err
	}
	rows, err := helpRequests(ctx, q, eventID)
	if err != nil {
		return wrap("load help follow-up sources", err)
	}
	for _, record := range rows {
		r := request(record)
		if r.Category == "competition_output" {
			if _, readErr := s.authorizeRead(ctx, q, row, r.RequesterID, r); readErr != nil {
				if errors.Is(readErr, access.ErrNotFound) || errors.Is(readErr, event.ErrBlocked) {
					continue
				}
				return readErr
			}
		}
		r, changed, reconcileErr := s.reconcile(ctx, q, row, r)
		if reconcileErr != nil {
			return reconcileErr
		}
		if changed {
			if err = s.emit(ctx, tx, q, row, r); err != nil {
				return err
			}
		}
		if r.Status != "waiting" ||
			s.now().Before(r.WaitingSince.Add(threshold(config, r.Category))) {
			continue
		}
		inserted, insertErr := q.InsertHelpFollowup(
			ctx,
			dbgen.InsertHelpFollowupParams{
				EventID:          eventID,
				RequestID:        r.ID,
				WaitingCycle:     r.WaitingCycle,
				SettingsRevision: config.Revision,
				CreatedAt:        s.now(),
				ThresholdSeconds: int64(threshold(config, r.Category) / time.Second),
			},
		)
		if insertErr != nil {
			return wrap("retain help follow-up", insertErr)
		}
		if inserted == 0 {
			continue
		}
		r.Revision++
		r.UpdatedAt = s.now()
		if err = persist(ctx, q, r, "overdue", nil); err != nil {
			return err
		}
		if err = s.emit(ctx, tx, q, row, r); err != nil {
			return err
		}
	}
	return nil
}

// Keyset queries preserve every request without a permanent event-history cap.
// The queue is projected in original chronological order after bounded reads.
func helpRequests(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
) ([]dbgen.HelpRequest, error) {
	result := []dbgen.HelpRequest{}
	after := uuid.Nil
	for {
		page, err := q.HelpRequestsPage(
			ctx,
			dbgen.HelpRequestsPageParams{EventID: eventID, AfterID: after},
		)
		if err != nil {
			return nil, wrap("page native help requests", err)
		}
		result = append(result, page...)
		if len(page) < 1000 {
			break
		}
		after = page[len(page)-1].ID
	}
	slices.SortFunc(result, func(a, b dbgen.HelpRequest) int {
		if c := a.RequestedAt.Compare(b.RequestedAt); c != 0 {
			return c
		}
		return bytes.Compare(a.ID[:], b.ID[:])
	})
	return result, nil
}
