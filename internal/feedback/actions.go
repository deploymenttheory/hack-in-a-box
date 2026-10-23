package feedback

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func decodeAction(raw []byte) (Action, error) {
	var a Action
	err := json.Unmarshal(raw, &a)
	return a, wrap("decode immutable support action", err)
}

func actionVersion(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, id uuid.UUID,
	version int64,
) (Action, error) {
	raw, err := q.FeedbackActionVersion(
		ctx,
		dbgen.FeedbackActionVersionParams{EventID: eventID, ID: id, Version: version},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Action{}, access.ErrNotFound
	}
	if err != nil {
		return Action{}, wrap("read support action", err)
	}
	return decodeAction(raw)
}

func (s Service) actionView(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	actor uuid.UUID,
	h dbgen.FeedbackHead,
	a Action,
) (ActionView, error) {
	out := ActionView{Action: a}
	authority, err := s.authority(ctx, q, actor, row.ID, h.TeamID, h.Category)
	if err != nil {
		return out, err
	}
	if err = s.gate(row, h.Category); err != nil {
		return out, err
	}
	if !h.PublishedVersion.Valid {
		return out, access.ErrNotFound
	}
	manager := authority.Organiser || authority.Helper
	out.CanEdit = manager && row.WritesAllowed
	out.CanUpdateStatus = row.WritesAllowed &&
		(manager || (authority.TeamMember && actor == a.OwnerID))
	_, err = s.authority(ctx, q, a.OwnerID, row.ID, h.TeamID, h.Category)
	if err != nil && !errors.Is(err, access.ErrForbidden) && !errors.Is(err, access.ErrNotFound) {
		return out, err
	}
	out.OwnerAvailable = err == nil
	cfg, err := settings(ctx, q, row.ID)
	if err != nil {
		return out, err
	}
	out.Overdue = s.overdue(a, h.Category, cfg)
	return out, nil
}

func (s Service) Actions(
	ctx context.Context,
	actor, eventID, feedbackID, after uuid.UUID,
	limit int32,
) (Actions, error) {
	out := Actions{Items: []ActionView{}}
	if _, err := pageLimit(limit); err != nil {
		return out, err
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := lock(ctx, q, eventID)
		if err != nil {
			return err
		}
		h, err := head(ctx, q, eventID, feedbackID)
		if err != nil {
			return err
		}
		if _, err = s.project(ctx, tx, q, row, actor, h); err != nil {
			return err
		}
		if !h.PublishedVersion.Valid {
			return access.ErrNotFound
		}
		rows, err := q.FeedbackActionHeadsPage(
			ctx,
			dbgen.FeedbackActionHeadsPageParams{
				EventID:    eventID,
				FeedbackID: feedbackID,
				AfterID:    after,
				PageSize:   limit + 1,
			},
		)
		if err != nil {
			return wrap("page support actions", err)
		}
		if len(rows) > int(limit) {
			rows = rows[:limit]
			id := rows[len(rows)-1].ID
			out.NextCursor = &id
		}
		for _, r := range rows {
			a, err := actionVersion(ctx, q, eventID, r.ID, r.CurrentVersion)
			if err != nil {
				return err
			}
			v, err := s.actionView(ctx, q, row, actor, h, a)
			if err != nil {
				return err
			}
			out.Items = append(out.Items, v)
		}
		return nil
	})
	return out, wrap("list team support actions", err)
}

func (s Service) ChangeAction(
	ctx context.Context,
	actor, eventID, feedbackID, id uuid.UUID,
	expected int64,
	commandID uuid.UUID,
	in ActionCommand,
) (ActionView, error) {
	var out ActionView
	if id == uuid.Nil || expected < 0 ||
		!slices.Contains([]string{"details", "status"}, in.Action) ||
		(in.Action == "details") != (in.Input != nil) ||
		(in.Action == "details" && in.Status != "") {
		return out, access.ErrInvalid
	}
	if in.Input != nil {
		if err := in.Input.validate(); err != nil {
			return out, err
		}
	}
	hash, err := digest(struct {
		Expected   int64         `json:"expected"`
		FeedbackID uuid.UUID     `json:"feedback_id"`
		Command    ActionCommand `json:"command"`
	}{expected, feedbackID, in})
	if err != nil {
		return out, err
	}
	err = database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		var err error
		out, err = s.changeActionInTransaction(
			ctx,
			tx,
			q,
			actor,
			eventID,
			feedbackID,
			id,
			expected,
			commandID,
			in,
			hash,
		)
		return err
	})
	return out, wrap("change support action", err)
}

func prepareAction(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, feedbackID, id uuid.UUID,
	expected int64,
	in ActionCommand,
) (Action, bool, error) {
	ah, err := q.FeedbackActionHead(ctx, dbgen.FeedbackActionHeadParams{EventID: eventID, ID: id})
	fresh := errors.Is(err, pgx.ErrNoRows)
	if err != nil && !fresh {
		return Action{}, fresh, wrap("read action identity", err)
	}
	if fresh && (expected != 0 || in.Input == nil) {
		return Action{}, fresh, access.ErrNotFound
	}
	if !fresh && ah.FeedbackID != feedbackID {
		return Action{}, fresh, access.ErrNotFound
	}
	if fresh {
		return Action{ID: id, FeedbackID: feedbackID, Status: "open", Cycle: 1}, true, nil
	}
	a, err := actionVersion(ctx, q, eventID, id, ah.CurrentVersion)
	return a, false, err
}

func (s Service) changeActionInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	actor, eventID, feedbackID, id uuid.UUID,
	expected int64,
	commandID uuid.UUID,
	in ActionCommand,
	hash string,
) (ActionView, error) {
	var out ActionView
	row, err := lock(ctx, q, eventID)
	if err != nil {
		return out, err
	}
	if !row.WritesAllowed {
		return out, access.ErrForbidden
	}
	h, err := head(ctx, q, eventID, feedbackID)
	if err != nil {
		return out, err
	}
	p, err := s.project(ctx, tx, q, row, actor, h)
	if err != nil {
		return out, err
	}
	if p.Published == nil {
		return out, access.ErrNotFound
	}
	a, fresh, err := prepareAction(ctx, q, eventID, feedbackID, id, expected, in)
	if err != nil {
		return out, err
	}
	view, err := s.actionView(ctx, q, row, actor, h, a)
	if err != nil {
		return out, err
	}
	if in.Action == "details" && !view.CanEdit || in.Action == "status" && !view.CanUpdateStatus {
		return out, access.ErrForbidden
	}
	v, replay, err := duplicate(ctx, q, eventID, actor, commandID, id, "action", hash)
	if err != nil {
		return out, err
	}
	if replay {
		a, err = actionVersion(ctx, q, eventID, id, v)
		if err != nil {
			return out, err
		}
		out, err = s.actionView(ctx, q, row, actor, h, a)
		out.CommandVersion = &v
		return out, err
	}
	if a.Version != expected {
		return out, access.ErrConflict
	}
	a, err = s.applyAction(ctx, q, eventID, h, view, in)
	if err != nil {
		return out, err
	}
	a.Version++
	a.ChangedBy = actor
	a.ChangedAt = s.now()
	if err = retainAction(ctx, q, eventID, feedbackID, a, fresh); err != nil {
		return out, err
	}
	if err = command(ctx, q, eventID, actor, commandID, id, "action", hash, a.Version); err != nil {
		return out, err
	}
	if err = record(ctx, q, eventID, actor, id, a.Version); err != nil {
		return out, err
	}
	out, err = s.actionView(ctx, q, row, actor, h, a)
	out.CommandVersion = &a.Version
	return out, err
}

func (s Service) applyAction(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	h dbgen.FeedbackHead,
	view ActionView,
	in ActionCommand,
) (Action, error) {
	a := view.Action
	switch in.Action {
	case "details":
		if _, err := s.authority(
			ctx,
			q,
			in.Input.OwnerID,
			eventID,
			h.TeamID,
			h.Category,
		); err != nil {
			return a, err
		}
		if a.Version > 0 && (a.OwnerID != in.Input.OwnerID || !a.DueAt.Equal(in.Input.DueAt)) {
			a.Cycle++
		}
		a.Title, a.Body, a.OwnerID, a.DueAt = in.Input.Title, in.Input.Body, in.Input.OwnerID, in.Input.DueAt.UTC().
			Truncate(time.Microsecond)
	case "status":
		if !slices.Contains([]string{"open", "in_progress", "done", "cancelled"}, in.Status) {
			return a, access.ErrInvalid
		}
		if !view.CanEdit &&
			(!slices.Contains([]string{"open", "in_progress"}, a.Status) || !slices.Contains([]string{"in_progress", "done"}, in.Status)) {
			return a, access.ErrForbidden
		}
		if a.Status == in.Status {
			return a, access.ErrConflict
		}
		if slices.Contains([]string{"done", "cancelled"}, a.Status) &&
			!slices.Contains([]string{"done", "cancelled"}, in.Status) {
			a.Cycle++
		}
		a.Status = in.Status
	}
	return a, nil
}

func retainAction(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, feedbackID uuid.UUID,
	a Action,
	fresh bool,
) error {
	raw, err := json.Marshal(a)
	if err != nil {
		return wrap("encode support action", err)
	}
	if fresh {
		if err = q.InsertFeedbackActionHead(
			ctx,
			dbgen.InsertFeedbackActionHeadParams{
				EventID:    eventID,
				FeedbackID: feedbackID,
				ID:         a.ID,
			},
		); err != nil {
			return wrap("create support action", err)
		}
	}
	if err = q.InsertFeedbackActionVersion(
		ctx,
		dbgen.InsertFeedbackActionVersionParams{
			EventID:  eventID,
			ID:       a.ID,
			Version:  a.Version,
			Document: raw,
		},
	); err != nil {
		return wrap("retain support action history", err)
	}
	return wrap(
		"advance support action",
		q.UpdateFeedbackActionHead(
			ctx,
			dbgen.UpdateFeedbackActionHeadParams{
				EventID:        eventID,
				ID:             a.ID,
				CurrentVersion: a.Version,
			},
		),
	)
}

func (s Service) ActionHistory(
	ctx context.Context,
	actor, eventID, feedbackID, id uuid.UUID,
	before int64,
	limit int32,
) (ActionHistory, error) {
	out := ActionHistory{Items: []Action{}}
	if _, err := pageLimit(limit); err != nil {
		return out, err
	}
	if before == 0 {
		before = math.MaxInt64
	}
	if before < 1 {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := lock(ctx, q, eventID)
		if err != nil {
			return err
		}
		h, err := head(ctx, q, eventID, feedbackID)
		if err != nil {
			return err
		}
		p, err := s.project(ctx, tx, q, row, actor, h)
		if err != nil {
			return err
		}
		if p.Published == nil {
			return access.ErrNotFound
		}
		ah, err := q.FeedbackActionHead(
			ctx,
			dbgen.FeedbackActionHeadParams{EventID: eventID, ID: id},
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return wrap("read support history identity", err)
		}
		if ah.FeedbackID != feedbackID {
			return access.ErrNotFound
		}
		rows, err := q.FeedbackActionHistoryPage(
			ctx,
			dbgen.FeedbackActionHistoryPageParams{
				EventID:       eventID,
				ID:            id,
				BeforeVersion: before,
				PageSize:      limit + 1,
			},
		)
		if err != nil {
			return wrap("page support history", err)
		}
		if len(rows) > int(limit) {
			rows = rows[:limit]
		}
		for _, raw := range rows {
			a, err := decodeAction(raw)
			if err != nil {
				return err
			}
			out.Items = append(out.Items, a)
		}
		if len(out.Items) == int(limit) {
			v := out.Items[len(out.Items)-1].Version
			out.NextCursor = &v
		}
		return nil
	})
	return out, wrap("read team support history", err)
}

// ActionReference is operational evidence only, never a formal gap-resolution
// decision or an assessment grant. Consumers must bind and independently review it.
type ActionReference struct {
	ActionID        uuid.UUID
	Version         int64
	FeedbackID      uuid.UUID
	FeedbackVersion int64
	Status          string
	OwnerEligible   bool
}

func (s Service) ResolveActionInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID, id uuid.UUID,
	expected int64,
) (ActionReference, error) {
	var out ActionReference
	q := dbgen.New(tx)
	row, err := lock(ctx, q, eventID)
	if err != nil {
		return out, err
	}
	ah, err := q.FeedbackActionHead(ctx, dbgen.FeedbackActionHeadParams{EventID: eventID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, access.ErrNotFound
	}
	if err != nil {
		return out, wrap("resolve native support action", err)
	}
	h, err := head(ctx, q, eventID, ah.FeedbackID)
	if err != nil {
		return out, err
	}
	if h.TeamID != teamID || !h.PublishedVersion.Valid || ah.CurrentVersion != expected {
		return out, access.ErrConflict
	}
	a, err := actionVersion(ctx, q, eventID, id, expected)
	if err != nil {
		return out, err
	}
	view, err := s.actionView(ctx, q, row, actor, h, a)
	if err != nil {
		return out, err
	}
	return ActionReference{
		ActionID:        id,
		Version:         a.Version,
		FeedbackID:      h.ID,
		FeedbackVersion: h.PublishedVersion.Int64,
		Status:          a.Status,
		OwnerEligible:   view.OwnerAvailable,
	}, nil
}
