package preparation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
)

type Service struct {
	Pool *pgxpool.Pool
	Now  func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func authority(ctx context.Context, q *dbgen.Queries, actor, eventID uuid.UUID) ([]string, error) {
	p, err := q.Person(ctx, actor)
	if err != nil || !p.Active {
		return nil, access.ErrForbidden
	}
	roles, err := q.ActiveRoles(ctx, dbgen.ActiveRolesParams{EventID: eventID, PersonID: actor})
	if err != nil {
		return nil, fmt.Errorf("read task authority: %w", err)
	}
	claim, err := q.CompetitionState(
		ctx,
		dbgen.CompetitionStateParams{EventID: eventID, PersonID: actor},
	)
	if err != nil {
		return nil, fmt.Errorf("read task attendee authority: %w", err)
	}
	if claim == "confirmed" {
		return append(roles, "attendee"), nil
	}
	for _, role := range roles {
		if slices.Contains(staffRoles, role) {
			return roles, nil
		}
	}
	return nil, access.ErrForbidden
}

func assigned(d Definition, actor uuid.UUID, roles []string) bool {
	if d.OwnerRole != "" && !slices.Contains(roles, d.OwnerRole) {
		return false
	}
	if d.OwnerID != nil {
		return *d.OwnerID == actor
	}
	return d.OwnerRole != "" && slices.Contains(roles, d.OwnerRole)
}

func load(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
) ([]Task, event.Configuration, templates.Definitions, error) {
	c, defs := event.DefaultConfiguration(), templates.EmptyDefinitions()
	if err := json.Unmarshal(row.Configuration, &c); err != nil {
		return nil, c, defs, fmt.Errorf("decode task schedule: %w", err)
	}
	r, err := q.EventDefinitions(ctx, row.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, c, defs, fmt.Errorf("read task seeds: %w", err)
	}
	if err == nil {
		if err := json.Unmarshal(r.Definitions, &defs); err != nil {
			return nil, c, defs, fmt.Errorf("decode task seeds: %w", err)
		}
	}
	rows, err := q.PreparationTasks(ctx, row.ID)
	if err != nil {
		return nil, c, defs, fmt.Errorf("read task state: %w", err)
	}
	items := make([]Task, 0, len(rows)+len(defs.Tasks))
	seen := map[uuid.UUID]bool{}
	for _, r := range rows {
		t := Task{Revision: r.Revision}
		if err := json.Unmarshal(r.Document, &t.Update); err != nil {
			return nil, c, defs, fmt.Errorf("decode task: %w", err)
		}
		items = append(items, t)
		seen[r.ID] = true
	}
	for _, d := range defs.Tasks {
		if !seen[d.ID] {
			items = append(
				items,
				Task{
					Update: Update{
						Definition: Definition{
							ID:         d.ID,
							Title:      d.Title,
							OwnerRole:  d.OwnerRole,
							Status:     "not_started",
							ScheduleID: d.ScheduleID,
							DependsOn:  append([]uuid.UUID{}, d.DependsOn...),
							Checklist:  []ChecklistItem{},
							Evidence:   []Evidence{},
						},
					},
				},
			)
		}
	}
	sort.Slice(
		items,
		func(i, j int) bool { return items[i].Definition.ID.String() < items[j].Definition.ID.String() },
	)
	return items, c, defs, nil
}

func (s Service) List(ctx context.Context, actor, eventID uuid.UUID, filter Filter) (Page, error) {
	var result Page
	if filter.Status != "" && !slices.Contains(statuses, filter.Status) {
		return result, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, eventID)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock task read: %w", err)
		}
		roles, err := authority(ctx, q, actor, eventID)
		if err != nil {
			return err
		}
		result, err = s.page(ctx, q, actor, row, roles, filter)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("list preparation tasks: %w", err)
	}
	return result, nil
}

func (s Service) page(
	ctx context.Context,
	q *dbgen.Queries,
	actor uuid.UUID,
	row dbgen.Event,
	roles []string,
	filter Filter,
) (Page, error) {
	items, c, _, err := load(ctx, q, row)
	if err != nil {
		return Page{}, err
	}
	manage := slices.Contains(roles, "organiser")
	result := Page{
		Revision:  row.Revision,
		Timezone:  c.Timezone,
		CanManage: manage && row.WritesAllowed,
		Items:     []Task{},
	}
	visible := visibleTasks(items, actor, roles)
	for _, task := range items {
		if !visible[task.Definition.ID] {
			continue
		}
		task.DueAt, task.SchedulingIssue = dueTime(task.Definition, c)
		task.Blocked = task.Definition.Status == "blocked" ||
			(!terminal(task.Definition.Status) && (!dependenciesReady(task.Definition, items) || task.SchedulingIssue))
		task.Overdue = !terminal(task.Definition.Status) && task.DueAt != nil &&
			task.DueAt.Before(s.now())
		task.Unowned = task.Definition.OwnerID == nil
		task.CanEdit = row.WritesAllowed && (manage || assigned(task.Definition, actor, roles))
		if !matches(task, filter) {
			continue
		}
		deps := []uuid.UUID{}
		for _, id := range task.Definition.DependsOn {
			if visible[id] {
				deps = append(deps, id)
			} else {
				task.HiddenDependencies++
			}
		}
		task.Definition.DependsOn = deps
		task.Definition.Evidence, task.RestrictedEvidenceCount, err = visibleEvidence(
			ctx,
			q,
			actor,
			row.ID,
			manage,
			task.Definition.Evidence,
		)
		if err != nil {
			return result, err
		}
		result.Items = append(result.Items, task)
	}
	return result, nil
}

func visibleTasks(items []Task, actor uuid.UUID, roles []string) map[uuid.UUID]bool {
	result := map[uuid.UUID]bool{}
	for _, t := range items {
		result[t.Definition.ID] = slices.Contains(roles, "organiser") ||
			assigned(t.Definition, actor, roles)
	}
	return result
}

func matches(task Task, f Filter) bool {
	d := task.Definition
	return (f.Status == "" || f.Status == d.Status) &&
		(f.OwnerRole == "" || f.OwnerRole == d.OwnerRole) &&
		(f.OwnerID == nil || (d.OwnerID != nil && *f.OwnerID == *d.OwnerID)) &&
		(!f.Blocked || task.Blocked) &&
		(!f.Overdue || task.Overdue) &&
		(!f.Unowned || task.Unowned)
}

func visibleEvidence(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID uuid.UUID,
	manage bool,
	items []Evidence,
) ([]Evidence, int, error) {
	result, restricted := make([]Evidence, 0, len(items)), 0
	for _, e := range items {
		if !manage && e.Visibility == "organisers" {
			restricted++
			continue
		}
		if e.FileID != nil {
			err := (access.EventPolicy{Queries: q}).Authorize(
				ctx,
				actor,
				eventID,
				*e.FileID,
				access.FileRead,
			)
			if errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) {
				restricted++
				continue
			}
			if err != nil {
				return nil, 0, fmt.Errorf("check task evidence: %w", err)
			}
		}
		result = append(result, e)
	}
	return result, restricted, nil
}

func (s Service) Save(
	ctx context.Context,
	actor, eventID uuid.UUID,
	revision int64,
	u Update,
) (Page, error) {
	var result Page
	u.Definition.DependsOn = append([]uuid.UUID{}, u.Definition.DependsOn...)
	u.Definition.Checklist = append([]ChecklistItem{}, u.Definition.Checklist...)
	u.Definition.Evidence = append([]Evidence{}, u.Definition.Evidence...)
	if err := validate(u); err != nil {
		return result, err
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, eventID)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock task edit: %w", err)
		}
		roles, err := authority(ctx, q, actor, eventID)
		if err != nil {
			return err
		}
		if !row.WritesAllowed {
			return access.ErrForbidden
		}
		if row.Revision != revision {
			return access.ErrConflict
		}
		items, c, defs, err := load(ctx, q, row)
		if err != nil {
			return err
		}
		index := slices.IndexFunc(
			items,
			func(t Task) bool { return t.Definition.ID == u.Definition.ID },
		)
		if !slices.Contains(roles, "organiser") {
			if index < 0 {
				return access.ErrForbidden
			}
			u, err = staffUpdate(ctx, q, actor, eventID, roles, items, index, u)
			if err != nil {
				return err
			}
			if err := validate(u); err != nil {
				return err
			}
		}
		if err := validateOwner(ctx, q, eventID, u.Definition); err != nil {
			return err
		}
		if _, missing := dueTime(u.Definition, c); missing {
			return invalid(
				"schedule_id",
				"schedule_missing",
				"Choose an existing event schedule item.",
			)
		}
		if err := checkEvidence(
			ctx,
			q,
			actor,
			eventID,
			u.Definition.Evidence,
			items,
			index,
		); err != nil {
			return err
		}
		candidate := Task{Update: u, Revision: revision + 1}
		if index < 0 {
			items = append(items, candidate)
		} else {
			items[index] = candidate
		}
		if len(items) > 200 {
			return access.ErrInvalid
		}
		if err := validateGraph(items); err != nil {
			return err
		}
		if err := validateCompletion(u, items); err != nil {
			return err
		}
		if err := persist(ctx, q, actor, row, c, defs, u); err != nil {
			return err
		}
		row.Revision++
		result, err = s.page(ctx, q, actor, row, roles, Filter{})
		return err
	})
	if err != nil {
		return result, fmt.Errorf("save preparation task: %w", err)
	}
	return result, nil
}

func validateCompletion(u Update, items []Task) error {
	if u.Definition.Status != "complete" || u.OverrideDependencies {
		return nil
	}
	if !dependenciesReady(u.Definition, items) {
		return invalid(
			"depends_on",
			"unfinished_dependencies",
			"Finish the dependencies or ask an organiser for a reasoned override.",
		)
	}
	for _, item := range u.Definition.Checklist {
		if !item.Complete {
			return invalid(
				"checklist",
				"unfinished_checklist",
				"Complete every checklist item or record an organiser override.",
			)
		}
	}
	return nil
}

func validateOwner(ctx context.Context, q *dbgen.Queries, eventID uuid.UUID, d Definition) error {
	if d.OwnerID == nil {
		return nil
	}
	roles, err := authority(ctx, q, *d.OwnerID, eventID)
	if err != nil {
		return invalid(
			"owner_id",
			"invalid_owner",
			"Choose an active event staff member or confirmed attendee.",
		)
	}
	if d.OwnerRole != "" && !slices.Contains(roles, d.OwnerRole) {
		return invalid(
			"owner_role",
			"owner_role_mismatch",
			"The owner must currently hold the assigned event role.",
		)
	}
	return nil
}

func checkEvidence(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID uuid.UUID,
	items []Evidence,
	tasks []Task,
	index int,
) error {
	for _, e := range items {
		if e.FileID == nil {
			continue
		}
		if index >= 0 &&
			slices.ContainsFunc(
				tasks[index].Definition.Evidence,
				func(old Evidence) bool { return reflect.DeepEqual(e, old) },
			) {
			continue
		}
		if err := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			*e.FileID,
			access.FileRead,
		); err != nil {
			return fmt.Errorf("authorize evidence attachment: %w", err)
		}
	}
	return nil
}

func staffUpdate(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID uuid.UUID,
	roles []string,
	items []Task,
	index int,
	next Update,
) (Update, error) {
	old := items[index].Update
	if !assigned(old.Definition, actor, roles) || next.Definition.Status == "waived" ||
		next.OverrideDependencies {
		return next, access.ErrForbidden
	}
	visible, _, err := visibleEvidence(ctx, q, actor, eventID, false, old.Definition.Evidence)
	if err != nil {
		return next, err
	}
	for _, e := range next.Definition.Evidence {
		if e.Visibility != "assignees" {
			return next, access.ErrForbidden
		}
	}
	visibleIDs := visibleTasks(items, actor, roles)
	deps := []uuid.UUID{}
	for _, id := range old.Definition.DependsOn {
		if visibleIDs[id] {
			deps = append(deps, id)
		}
	}
	if !slices.Equal(next.Definition.DependsOn, deps) {
		return next, access.ErrForbidden
	}
	next.Definition.DependsOn = old.Definition.DependsOn
	before, after := old.Definition, next.Definition
	before.Status, after.Status = "", ""
	before.Evidence, after.Evidence = nil, nil
	before.Checklist, after.Checklist = slices.Clone(
		before.Checklist,
	), slices.Clone(
		after.Checklist,
	)
	for i := range before.Checklist {
		before.Checklist[i].Complete = false
	}
	for i := range after.Checklist {
		after.Checklist[i].Complete = false
	}
	if !reflect.DeepEqual(before, after) {
		return next, access.ErrForbidden
	}
	for _, e := range old.Definition.Evidence {
		if !slices.Contains(visible, e) {
			next.Definition.Evidence = append(next.Definition.Evidence, e)
		}
	}
	return next, nil
}

func persist(
	ctx context.Context,
	q *dbgen.Queries,
	actor uuid.UUID,
	row dbgen.Event,
	c event.Configuration,
	defs templates.Definitions,
	u Update,
) error {
	d := u.Definition
	seed := templates.Task{
		ID:         d.ID,
		Title:      d.Title,
		OwnerRole:  d.OwnerRole,
		DependsOn:  d.DependsOn,
		ScheduleID: d.ScheduleID,
	}
	index := slices.IndexFunc(defs.Tasks, func(t templates.Task) bool { return t.ID == d.ID })
	if index < 0 {
		defs.Tasks = append(defs.Tasks, seed)
	} else {
		defs.Tasks[index] = seed
	}
	if err := templates.Validate(templates.Bundle{Event: c, Definitions: defs}); err != nil {
		return fmt.Errorf("validate reusable task definitions: %w", err)
	}
	data, err := json.Marshal(defs)
	if err != nil {
		return fmt.Errorf("encode reusable tasks: %w", err)
	}
	if err := q.SaveEventDefinitions(
		ctx,
		dbgen.SaveEventDefinitionsParams{EventID: row.ID, Definitions: data},
	); err != nil {
		return fmt.Errorf("save task definitions: %w", err)
	}
	document, err := json.Marshal(u)
	if err != nil {
		return fmt.Errorf("encode task change: %w", err)
	}
	revision := row.Revision + 1
	if err := q.SavePreparationTask(
		ctx,
		dbgen.SavePreparationTaskParams{
			EventID:  row.ID,
			ID:       d.ID,
			Revision: revision,
			Document: document,
		},
	); err != nil {
		return fmt.Errorf("persist task: %w", err)
	}
	if err := q.RecordPreparationChange(
		ctx,
		dbgen.RecordPreparationChangeParams{
			EventID:  row.ID,
			TaskID:   d.ID,
			Revision: revision,
			ActorID:  actor,
			Document: document,
		},
	); err != nil {
		return fmt.Errorf("record task change: %w", err)
	}
	if _, err := q.BumpEventRevision(ctx, row.ID); err != nil {
		return fmt.Errorf("advance task revision: %w", err)
	}
	if err := audit.Append(
		ctx,
		q,
		audit.Record{
			ActorID:  &actor,
			EventID:  &row.ID,
			TargetID: d.ID,
			Action:   audit.PreparationChanged,
			Reason:   audit.UserRequest,
			Metadata: audit.Metadata{PreviousRevision: &row.Revision, Revision: &revision},
		},
	); err != nil {
		return fmt.Errorf("audit task change: %w", err)
	}
	return nil
}

func (s Service) History(ctx context.Context, actor, eventID, taskID uuid.UUID) ([]Change, error) {
	result := []Change{}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(ctx, eventID); err != nil {
			return access.ErrNotFound
		}
		if err := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.StaffRead,
		); err != nil {
			return fmt.Errorf("authorize task history: %w", err)
		}
		rows, err := q.PreparationHistory(
			ctx,
			dbgen.PreparationHistoryParams{EventID: eventID, TaskID: taskID},
		)
		if err != nil {
			return fmt.Errorf("read task history: %w", err)
		}
		for _, row := range rows {
			change := Change{Revision: row.Revision, ActorID: row.ActorID, CreatedAt: row.CreatedAt}
			if err := json.Unmarshal(row.Document, &change.Update); err != nil {
				return fmt.Errorf("decode task history: %w", err)
			}
			change.Definition.Evidence, _, err = visibleEvidence(
				ctx,
				q,
				actor,
				eventID,
				true,
				change.Definition.Evidence,
			)
			if err != nil {
				return err
			}
			result = append(result, change)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read preparation history: %w", err)
	}
	return result, nil
}
