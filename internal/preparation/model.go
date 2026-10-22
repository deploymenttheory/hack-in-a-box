// Package preparation owns operational tasks, separate from lifecycle gates and team submissions.
package preparation

import (
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

type ChecklistItem struct {
	ID       uuid.UUID `json:"id"`
	Title    string    `json:"title"`
	Complete bool      `json:"complete"`
}
type Evidence struct {
	ID         uuid.UUID  `json:"id"`
	Kind       string     `json:"kind"`
	Label      string     `json:"label"`
	URL        string     `json:"url"`
	FileID     *uuid.UUID `json:"file_id"`
	Visibility string     `json:"visibility"`
}
type Definition struct {
	ID          uuid.UUID       `json:"id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	OwnerID     *uuid.UUID      `json:"owner_id"`
	OwnerRole   string          `json:"owner_role"`
	Status      string          `json:"status"`
	ScheduleID  *uuid.UUID      `json:"schedule_id"`
	FixedDueAt  *time.Time      `json:"fixed_due_at"`
	DependsOn   []uuid.UUID     `json:"depends_on"`
	Checklist   []ChecklistItem `json:"checklist"`
	Evidence    []Evidence      `json:"evidence"`
}
type Update struct {
	Definition           Definition `json:"definition"`
	Reason               string     `json:"reason"`
	OverrideDependencies bool       `json:"override_dependencies"`
}
type Task struct {
	Update
	Revision                int64      `json:"revision"`
	DueAt                   *time.Time `json:"due_at"`
	SchedulingIssue         bool       `json:"scheduling_issue"`
	Blocked                 bool       `json:"blocked"`
	Overdue                 bool       `json:"overdue"`
	Unowned                 bool       `json:"unowned"`
	CanEdit                 bool       `json:"can_edit"`
	RestrictedEvidenceCount int        `json:"restricted_evidence_count"`
	HiddenDependencies      int        `json:"hidden_dependencies"`
}
type Page struct {
	Revision  int64  `json:"revision"`
	Timezone  string `json:"timezone"`
	CanManage bool   `json:"can_manage"`
	Items     []Task `json:"items"`
}
type Change struct {
	Revision  int64     `json:"revision"`
	ActorID   uuid.UUID `json:"actor_id"`
	CreatedAt time.Time `json:"created_at"`
	Update
}
type Filter struct {
	Status    string
	OwnerID   *uuid.UUID
	OwnerRole string
	Overdue   bool
	Blocked   bool
	Unowned   bool
}

var (
	staffRoles = []string{"organiser", "mentor", "judge", "mc", "logistics", "photographer"}
	statuses   = []string{"not_started", "in_progress", "blocked", "complete", "waived"}
)

func invalid(field, code, message string) error {
	return &event.Blocked{Issues: []event.Issue{{Field: field, Code: code, Message: message}}}
}

func validate(u Update) error {
	d := u.Definition
	if d.ID == uuid.Nil || strings.TrimSpace(d.Title) == "" || len(d.Title) > 160 ||
		len(d.Description) > 4000 ||
		!slices.Contains(statuses, d.Status) ||
		len(u.Reason) > 2000 ||
		(d.OwnerRole != "" && d.OwnerRole != "attendee" && !slices.Contains(staffRoles, d.OwnerRole)) ||
		(d.OwnerID != nil && *d.OwnerID == uuid.Nil) ||
		len(d.DependsOn) > 200 ||
		len(d.Checklist) > 100 ||
		len(d.Evidence) > 30 {
		return access.ErrInvalid
	}
	if (d.Status == "complete" || d.Status == "waived" || d.Status == "blocked" || u.OverrideDependencies) &&
		strings.TrimSpace(u.Reason) == "" {
		return invalid(
			"reason",
			"reason_required",
			"Record a reason for completion, blocking, a waiver or a dependency override.",
		)
	}
	if u.OverrideDependencies && d.Status != "complete" {
		return access.ErrInvalid
	}
	if err := validateDueTime(d); err != nil {
		return err
	}
	return validateItems(d)
}

func validateDueTime(d Definition) error {
	if d.ScheduleID != nil && (d.FixedDueAt != nil || *d.ScheduleID == uuid.Nil) {
		return invalid(
			"schedule_id",
			"due_time",
			"Choose a schedule item or a fixed due time, not both.",
		)
	}
	if d.FixedDueAt != nil &&
		(d.FixedDueAt.IsZero() || d.FixedDueAt.Year() < 2000 || d.FixedDueAt.Year() > 2200) {
		return access.ErrInvalid
	}
	return nil
}

func validateItems(d Definition) error {
	seen := map[uuid.UUID]bool{}
	for _, item := range d.Checklist {
		if item.ID == uuid.Nil || seen[item.ID] || strings.TrimSpace(item.Title) == "" ||
			len(item.Title) > 300 {
			return access.ErrInvalid
		}
		seen[item.ID] = true
	}
	for _, e := range d.Evidence {
		if e.ID == uuid.Nil || seen[e.ID] || strings.TrimSpace(e.Label) == "" ||
			len(e.Label) > 300 ||
			!slices.Contains([]string{"organisers", "assignees"}, e.Visibility) {
			return access.ErrInvalid
		}
		seen[e.ID] = true
		if err := validateEvidence(e); err != nil {
			return err
		}
	}
	return nil
}

func validateEvidence(e Evidence) error {
	if e.Kind == "file" {
		if e.FileID == nil || *e.FileID == uuid.Nil || e.URL != "" {
			return access.ErrInvalid
		}
		return nil
	}
	if !slices.Contains([]string{"url", "repository"}, e.Kind) || e.FileID != nil ||
		len(e.URL) > 2048 {
		return access.ErrInvalid
	}
	u, err := url.Parse(e.URL)
	if err != nil || u.Hostname() == "" || u.User != nil ||
		(u.Scheme != "https" && u.Scheme != "http") {
		return invalid(
			"evidence",
			"unsafe_url",
			"Use an HTTP or HTTPS reference without embedded credentials. References are never fetched by the server.",
		)
	}
	return nil
}

func validateGraph(tasks []Task) error {
	graph := make(map[uuid.UUID][]uuid.UUID, len(tasks))
	for _, task := range tasks {
		graph[task.Definition.ID] = task.Definition.DependsOn
	}
	visited := map[uuid.UUID]int{}
	var visit func(uuid.UUID) bool
	visit = func(id uuid.UUID) bool {
		if visited[id] == 1 {
			return false
		}
		if visited[id] == 2 {
			return true
		}
		deps, ok := graph[id]
		if !ok {
			return false
		}
		visited[id] = 1
		seen := map[uuid.UUID]bool{}
		for _, dep := range deps {
			if seen[dep] || !visit(dep) {
				return false
			}
			seen[dep] = true
		}
		visited[id] = 2
		return true
	}
	for id := range graph {
		if !visit(id) {
			return invalid(
				"depends_on",
				"invalid_dependencies",
				"Dependencies must be distinct tasks in this event and must not form a cycle.",
			)
		}
	}
	return nil
}

func terminal(status string) bool { return status == "complete" || status == "waived" }

func dependenciesReady(d Definition, tasks []Task) bool {
	for _, id := range d.DependsOn {
		found := false
		for _, task := range tasks {
			if task.Definition.ID == id {
				found = terminal(task.Definition.Status)
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func dueTime(d Definition, c event.Configuration) (*time.Time, bool) {
	if d.ScheduleID == nil {
		return d.FixedDueAt, false
	}
	for _, item := range c.Schedule {
		if item.ID == *d.ScheduleID {
			due, err := event.Plan(c, item)
			if err == nil {
				return &due, false
			}
			break
		}
	}
	return nil, true
}
