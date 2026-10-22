package event

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/permissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
)

type Service struct {
	Pool              *pgxpool.Pool
	Clock             func() time.Time
	SettleSubmissions func(context.Context, pgx.Tx, dbgen.Event, time.Time) error
}

func (s Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
}

func recordAudit(
	ctx context.Context,
	q *dbgen.Queries,
	actor, event uuid.UUID,
	action string,
	target uuid.UUID,
) error {
	if err := audit.Append(
		ctx,
		q,
		audit.Record{
			ActorID:  &actor,
			EventID:  &event,
			TargetID: target,
			Action:   audit.Action(action),
			Reason:   audit.OrganiserDecision,
		},
	); err != nil {
		return fmt.Errorf("record event decision: %w", err)
	}
	return nil
}

func (s Service) Create(ctx context.Context, actor uuid.UUID, input Input) (View, error) {
	var result View
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		var err error
		result, err = s.CreateIn(ctx, tx, actor, input)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("create event: %w", err)
	}
	return result, nil
}

// CreateIn lets a template snapshot and the new event commit atomically.
func (s Service) CreateIn(
	ctx context.Context,
	tx pgx.Tx,
	actor uuid.UUID,
	input Input,
) (View, error) {
	var result View
	if issues := Validate(input, false); len(issues) > 0 {
		return result, &Blocked{Issues: issues}
	}
	if input.ID == uuid.Nil || !input.AcknowledgeCompetitionRestriction {
		return result, access.ErrInvalid
	}
	q := dbgen.New(tx)
	if err := q.LockCreationID(ctx, input.ID.String()); err != nil {
		return result, fmt.Errorf("lock creation identity: %w", err)
	}
	allowed, err := q.CanCreateEvent(ctx, actor)
	if err != nil {
		return result, fmt.Errorf("check event creation authority: %w", err)
	}
	if !allowed {
		return result, access.ErrForbidden
	}
	data, err := json.Marshal(input.Configuration)
	if err != nil {
		return result, fmt.Errorf("encode event: %w", err)
	}
	existing, err := q.Event(ctx, input.ID)
	if err == nil {
		old, decodeErr := decode(existing)
		if decodeErr != nil {
			return result, fmt.Errorf("decode existing event: %w", decodeErr)
		}
		oldData, _ := json.Marshal(old)
		if existing.OwnerID == nil || *existing.OwnerID != actor ||
			existing.Name != input.Name ||
			existing.Slug != input.Slug ||
			string(oldData) != string(data) {
			return result, access.ErrConflict
		}
		result, err = s.view(ctx, q, actor, existing)
		return result, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, fmt.Errorf("check event creation: %w", err)
	}
	row, err := q.CreateEvent(
		ctx,
		dbgen.CreateEventParams{
			ID:            input.ID,
			Name:          input.Name,
			Slug:          input.Slug,
			Configuration: data,
		},
	)
	if err != nil {
		return result, conflict(err)
	}
	if err := q.InitializeEventFacts(ctx, row.ID); err != nil {
		return result, fmt.Errorf("initialize gates: %w", err)
	}
	if err := permissions.AssignOwner(ctx, q, row.ID, actor); err != nil {
		return result, fmt.Errorf("assign initial owner: %w", err)
	}
	if err := replan(ctx, q, row.ID, input.Configuration); err != nil {
		return result, err
	}
	if err := recordAudit(ctx, q, actor, row.ID, "event.created", row.ID); err != nil {
		return result, err
	}
	row.OwnerID = &actor
	result, err = s.view(ctx, q, actor, row)
	return result, err
}

func conflict(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return access.ErrConflict
	}
	return fmt.Errorf("persist event: %w", err)
}

func (s Service) Read(ctx context.Context, actor, id uuid.UUID) (View, error) {
	q := dbgen.New(s.Pool)
	if err := (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		id,
		id,
		access.EventRead,
	); err != nil {
		return View{}, fmt.Errorf("authorize event: %w", err)
	}
	row, err := q.Event(ctx, id)
	if err != nil {
		return View{}, fmt.Errorf("load event: %w", err)
	}
	return s.view(ctx, q, actor, row)
}

func (s Service) Decisions(
	ctx context.Context,
	actor, id uuid.UUID,
) ([]dbgen.EventDecision, error) {
	q := dbgen.New(s.Pool)
	if err := (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		id,
		id,
		access.StaffRead,
	); err != nil {
		return nil, fmt.Errorf("authorize decision reasons: %w", err)
	}
	rows, err := q.ListEventDecisions(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load decisions: %w", err)
	}
	return rows, nil
}

func (s Service) List(ctx context.Context, actor, cursor uuid.UUID, limit int32) ([]View, error) {
	if limit < 1 || limit > 100 {
		return nil, access.ErrInvalid
	}
	q := dbgen.New(s.Pool)
	person, err := q.Person(ctx, actor)
	if err != nil || !person.Active {
		return nil, access.ErrForbidden
	}
	rows, err := q.ListVisibleEvents(
		ctx,
		dbgen.ListVisibleEventsParams{Actor: actor, Cursor: cursor, PageSize: limit},
	)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	result := make([]View, 0, len(rows))
	for _, row := range rows {
		view, err := s.view(ctx, q, actor, row)
		if err != nil {
			return nil, err
		}
		result = append(result, view)
	}
	return result, nil
}

func (s Service) view(
	ctx context.Context,
	q *dbgen.Queries,
	actor uuid.UUID,
	row dbgen.Event,
) (View, error) {
	c, err := decode(row)
	if err != nil {
		return View{}, fmt.Errorf("decode event configuration: %w", err)
	}
	facts, err := q.EventFacts(ctx, row.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return View{}, fmt.Errorf("load lifecycle facts: %w", err)
	}
	count, err := q.ConfirmedAttendees(ctx, row.ID)
	if err != nil {
		return View{}, fmt.Errorf("count confirmed attendees: %w", err)
	}
	next := map[string]string{"draft": "registration", "registration": "preparation", "preparation": "live", "live": "assessment"}[row.Stage]
	blockers := gateBlockers(row, c, facts, count, next, s.now(), false, "")
	if row.Stage == "preparation" && !row.PublishedAt.Valid {
		publication, err := schedule.ChallengePublication(c.LiveStart, c.Timezone, c.DSTChoice)
		if err == nil && s.now().After(publication.Add(schedule.ChallengeDispatchTolerance)) {
			blockers = append(
				blockers,
				Issue{
					Field:   "challenge_publication",
					Code:    "publication_missed",
					Message: "Publication is overdue. Postpone the live start to preserve a full preparation week, or cancel.",
				},
			)
		}
	}
	policy := access.EventPolicy{Queries: q}
	view := View{
		ID:                 row.ID,
		Name:               row.Name,
		Slug:               row.Slug,
		OwnerID:            row.OwnerID,
		Revision:           row.Revision,
		Stage:              row.Stage,
		Configuration:      c,
		ConfirmedAttendees: count,
		Blockers:           blockers,
		CanEdit:            policy.Authorize(ctx, actor, row.ID, row.ID, access.EventEdit) == nil,
		CanManageOwner: policy.Authorize(
			ctx,
			actor,
			row.ID,
			row.ID,
			access.OwnershipManage,
		) == nil,
	}
	if !c.LiveStart.IsZero() {
		if at, err := schedule.ChallengePublication(
			c.LiveStart,
			c.Timezone,
			c.DSTChoice,
		); err == nil {
			view.ChallengePublication = &at
		}
	}
	view.CanPresent = policy.Authorize(
		ctx,
		actor,
		row.ID,
		row.ID,
		access.PresentationControl,
	) == nil
	view.CanReadStaff = policy.Authorize(ctx, actor, row.ID, row.ID, access.StaffRead) == nil
	view.CanReadAttendance = policy.Authorize(ctx, actor, row.ID, row.ID, access.RosterRead) == nil
	view.CanReadLogistics = policy.Authorize(
		ctx,
		actor,
		row.ID,
		row.ID,
		access.ProfileExport,
	) == nil
	view.CanTransition = view.CanReadStaff && row.Stage != "archived"
	if row.PublishedAt.Valid {
		at := row.PublishedAt.Time
		view.PublishedAt = &at
		view.ChallengePublication = &at
	}
	// Event details are shared; unreleased role-specific agenda text is not.
	if policy.Authorize(ctx, actor, row.ID, row.ID, access.StaffRead) != nil {
		view.Configuration.Schedule = []AgendaItem{}
	}
	return view, nil
}

func (s Service) Update(
	ctx context.Context,
	actor, id uuid.UUID,
	revision int64,
	input Input,
) (View, error) {
	var result View
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, id)
		if err != nil {
			return fmt.Errorf("lock event configuration: %w", err)
		}
		if err := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			id,
			id,
			access.EventEdit,
		); err != nil {
			return fmt.Errorf("authorize event edit: %w", err)
		}
		if row.Revision != revision {
			return access.ErrConflict
		}
		if issues := Validate(input, row.Stage != "draft"); len(issues) > 0 {
			return &Blocked{Issues: issues}
		}
		confirmed, err := q.ConfirmedAttendees(ctx, id)
		if err != nil {
			return fmt.Errorf("read confirmed capacity: %w", err)
		}
		reserved, err := q.ReservedAttendees(ctx, id)
		if err != nil {
			return fmt.Errorf("read reserved capacity: %w", err)
		}
		if int64(input.Configuration.Capacity) < confirmed+reserved {
			return &Blocked{
				Issues: []Issue{
					{
						Field:   "capacity",
						Code:    "occupied_places",
						Message: "Capacity cannot be reduced below confirmed places and reserved offers. Resolve withdrawals or offers before reducing it.",
					},
				},
			}
		}
		previous, err := decode(row)
		if err != nil {
			return fmt.Errorf("decode previous configuration: %w", err)
		}
		if err := s.rescheduleRules(row, previous, input); err != nil {
			return err
		}
		deadlineChanged, err := submissionDeadlineChange(
			ctx,
			q,
			row,
			previous,
			input.Configuration,
			input.Reason,
		)
		if err != nil {
			return err
		}
		if s.SettleSubmissions != nil {
			if err := s.SettleSubmissions(ctx, tx, row, s.now()); err != nil {
				return fmt.Errorf("settle before rescheduling: %w", err)
			}
		}
		data, err := json.Marshal(input.Configuration)
		if err != nil {
			return fmt.Errorf("encode configuration: %w", err)
		}
		changed, err := q.SaveEventConfiguration(
			ctx,
			dbgen.SaveEventConfigurationParams{
				ID:            id,
				Name:          input.Name,
				Slug:          input.Slug,
				Configuration: data,
			},
		)
		if err != nil {
			return conflict(err)
		}
		if err := invalidateChallengeApproval(
			ctx,
			q,
			row,
			actor,
			input.Configuration,
			s.now(),
		); err != nil {
			return err
		}
		if err := replan(ctx, q, id, input.Configuration); err != nil {
			return err
		}
		if row.PublishedAt.Valid && !previous.LiveStart.Equal(input.Configuration.LiveStart) {
			if err := s.decision(
				ctx,
				q,
				actor,
				id,
				"postponement",
				input.Reason,
				changed.Revision,
			); err != nil {
				return err
			}
			if err := notice(ctx, tx, id, changed.Revision, input.Configuration); err != nil {
				return err
			}
		}
		if deadlineChanged {
			if err := s.decision(
				ctx,
				q,
				actor,
				id,
				"deliverable_deadline_change",
				input.Reason,
				changed.Revision,
			); err != nil {
				return err
			}
		}
		if err := recordAudit(ctx, q, actor, id, "event.updated", id); err != nil {
			return err
		}
		result, err = s.view(ctx, q, actor, changed)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("update event: %w", err)
	}
	return result, nil
}

func (s Service) rescheduleRules(row dbgen.Event, old Configuration, input Input) error {
	c := input.Configuration
	blocked := func(field, code, message string) error {
		return &Blocked{Issues: []Issue{{Field: field, Code: code, Message: message}}}
	}
	if (row.TeamFrozenAt.Valid || row.PublishedAt.Valid) &&
		(!old.TeamFreeze.Equal(c.TeamFreeze) || !old.AdmissionCutoff.Equal(c.AdmissionCutoff)) {
		return blocked(
			"team_freeze",
			"already_frozen",
			"A completed team freeze cannot be reopened by changing admission or freeze deadlines.",
		)
	}
	startChanged := !old.LiveStart.Equal(c.LiveStart)
	timingChanged := startChanged || !old.LiveEnd.Equal(c.LiveEnd) ||
		!old.SubmissionCutoff.Equal(c.SubmissionCutoff) ||
		old.Timezone != c.Timezone
	if row.PublishedAt.Valid && timingChanged {
		if !c.LiveStart.After(old.LiveStart) || !input.ApprovePostponement ||
			strings.TrimSpace(input.Reason) == "" ||
			len(input.Reason) > 2000 {
			return blocked(
				"live_start",
				"postponement_approval",
				"After publication, postponement requires a later live start, explicit approval and a recorded reason. Teams may continue working.",
			)
		}
	} else if startChanged && !c.LiveStart.IsZero() {
		publication, err := schedule.ChallengePublication(c.LiveStart, c.Timezone, c.DSTChoice)
		if err == nil && !publication.After(s.now()) {
			return blocked(
				"live_start",
				"preserve_preparation_week",
				"Choose a live start whose challenge publication is still in the future, preserving the full preparation week.",
			)
		}
	}
	return nil
}

func (s Service) decision(
	ctx context.Context,
	q *dbgen.Queries,
	actor, event uuid.UUID,
	kind, reason string,
	revision int64,
) error {
	if strings.TrimSpace(reason) == "" || len(reason) > 2000 {
		return access.ErrInvalid
	}
	row, err := q.RecordEventDecision(
		ctx,
		dbgen.RecordEventDecisionParams{
			EventID:  event,
			ActorID:  actor,
			Kind:     kind,
			Reason:   reason,
			Revision: revision,
		},
	)
	if err != nil {
		return fmt.Errorf("record reason: %w", err)
	}
	return recordAudit(ctx, q, actor, event, "event.decision", row.ID)
}

func (s Service) Transition(
	ctx context.Context,
	actor, id uuid.UUID,
	revision int64,
	to, reason string,
	attendanceException bool,
) (View, error) {
	var result View
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, id)
		if err != nil {
			return fmt.Errorf("lock lifecycle: %w", err)
		}
		if err := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			id,
			id,
			access.StaffRead,
		); err != nil {
			return fmt.Errorf("authorize stage change: %w", err)
		}
		if row.Revision != revision {
			return access.ErrConflict
		}
		if !allowedTransition(row.Stage, to) {
			return &Blocked{
				Issues: []Issue{
					{
						Field:   "stage",
						Code:    "invalid_transition",
						Message: "This lifecycle transition is not available.",
					},
				},
			}
		}
		c, err := decode(row)
		if err != nil {
			return fmt.Errorf("decode lifecycle configuration: %w", err)
		}
		if to == "assessment" && s.SettleSubmissions != nil {
			if err := s.SettleSubmissions(ctx, tx, row, s.now()); err != nil {
				return fmt.Errorf("settle assessment submissions: %w", err)
			}
		}
		f, err := q.EventFacts(ctx, id)
		if err != nil {
			return fmt.Errorf("load gates: %w", err)
		}
		count, err := q.ConfirmedAttendees(ctx, id)
		if err != nil {
			return fmt.Errorf("count competitors: %w", err)
		}
		if issues := gateBlockers(
			row,
			c,
			f,
			count,
			to,
			s.now(),
			attendanceException,
			strings.TrimSpace(reason),
		); len(
			issues,
		) > 0 {
			return &Blocked{Issues: issues}
		}
		if to == "cancelled" || row.Stage == "completed" || attendanceException {
			if err := s.decision(
				ctx,
				q,
				actor,
				id,
				"transition_to_"+to,
				reason,
				row.Revision+1,
			); err != nil {
				return err
			}
		}
		changed, err := q.SetEventStage(
			ctx,
			dbgen.SetEventStageParams{
				ID:            id,
				Stage:         to,
				WritesAllowed: to != "archived" && to != "cancelled",
			},
		)
		if err != nil {
			return fmt.Errorf("change stage: %w", err)
		}
		if !changed.WritesAllowed {
			if err := q.CancelEventOperations(ctx, id); err != nil {
				return fmt.Errorf("cancel event operations: %w", err)
			}
			if err := q.CancelEventSchedule(ctx, id); err != nil {
				return fmt.Errorf("cancel event schedule: %w", err)
			}
		}
		if err := recordAudit(ctx, q, actor, id, "event.stage_changed", id); err != nil {
			return err
		}
		result, err = s.view(ctx, q, actor, changed)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("transition event: %w", err)
	}
	return result, nil
}

func allowedTransition(from, to string) bool {
	if to == "cancelled" {
		return from != "cancelled" && from != "archived"
	}
	switch from {
	case "draft":
		return to == "registration"
	case "registration":
		return to == "preparation"
	case "preparation":
		return to == "live"
	case "live":
		return to == "assessment"
	case "assessment":
		return to == "completed"
	case "completed":
		return to == "archived" || to == "assessment"
	case "cancelled":
		return to == "archived"
	}
	return false
}

// notice persists the exact revised dates as an outbox intent for the future
// communication owner (#82). It must not claim an announcement was delivered.
func notice(
	ctx context.Context,
	tx pgx.Tx,
	event uuid.UUID,
	revision int64,
	c Configuration,
) error {
	id := uuid.New()
	if _, err := tx.Exec(
		ctx,
		"INSERT INTO event_notices(id,event_id,revision,live_start,live_end,submission_cutoff) VALUES($1,$2,$3,$4,$5,$6)",
		id,
		event,
		revision,
		c.LiveStart,
		c.LiveEnd,
		c.SubmissionCutoff,
	); err != nil {
		return fmt.Errorf("persist revised-date notice: %w", err)
	}
	if _, err := tx.Exec(
		ctx,
		"INSERT INTO operations(event_id,operation_key,kind,target_id,due_at,approved) VALUES($1,$2,'event.rescheduled',$3,now(),true)",
		event,
		fmt.Sprintf("event-rescheduled:%d", revision),
		id,
	); err != nil {
		return fmt.Errorf("queue revised-date notice: %w", err)
	}
	return nil
}
