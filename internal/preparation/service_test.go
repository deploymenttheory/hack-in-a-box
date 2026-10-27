package preparation

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func taskInput(title, role string) Update {
	return Update{
		Definition: Definition{
			ID:        uuid.New(),
			Title:     title,
			OwnerRole: role,
			Status:    "not_started",
			DependsOn: []uuid.UUID{},
			Checklist: []ChecklistItem{},
			Evidence:  []Evidence{},
		},
	}
}

func findTask(t *testing.T, page Page, id uuid.UUID) Task {
	t.Helper()
	for _, item := range page.Items {
		if item.Definition.ID == id {
			return item
		}
	}
	t.Fatal("task missing")
	return Task{}
}

func TestPreparationDependenciesWaiversAndConcurrentEdits(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	owner := testdb.Person(t, pool)
	outsider := testdb.Person(t, pool)
	id := testdb.Event(t, pool, owner.ID)
	s := Service{
		Pool: pool,
		Now:  func() time.Time { return time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC) },
	}
	page, err := s.List(ctx, owner.ID, id, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	a, b := taskInput("Approve venue", "organiser"), taskInput("Set up AV", "logistics")
	page, err = s.Save(ctx, owner.ID, id, page.Revision, a)
	if err != nil {
		t.Fatal(err)
	}
	b.Definition.DependsOn = []uuid.UUID{a.Definition.ID}
	b.Definition.FixedDueAt = new(s.now().Add(-time.Hour))
	page, err = s.Save(ctx, owner.ID, id, page.Revision, b)
	if err != nil {
		t.Fatal(err)
	}
	filtered, err := s.List(ctx, owner.ID, id, Filter{Blocked: true, Overdue: true, Unowned: true})
	if err != nil || len(filtered.Items) != 1 {
		t.Fatalf("filters: %+v %v", filtered, err)
	}
	a.Definition.DependsOn = []uuid.UUID{b.Definition.ID}
	if _, err := s.Save(ctx, owner.ID, id, page.Revision, a); !errors.Is(err, event.ErrBlocked) {
		t.Fatalf("cycle accepted: %v", err)
	}
	otherEvent := testdb.Event(t, pool, outsider.ID)
	otherPage, err := s.List(ctx, outsider.ID, otherEvent, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	foreignTask := taskInput("Other event task", "organiser")
	if _, err := s.Save(ctx, outsider.ID, otherEvent, otherPage.Revision, foreignTask); err != nil {
		t.Fatal(err)
	}
	a.Definition.DependsOn = []uuid.UUID{foreignTask.Definition.ID}
	if _, err := s.Save(ctx, owner.ID, id, page.Revision, a); !errors.Is(err, event.ErrBlocked) {
		t.Fatalf("foreign dependency accepted: %v", err)
	}
	a.Definition.DependsOn = []uuid.UUID{}
	b.Definition.Status, b.Reason = "complete", "AV tested"
	if _, err := s.Save(ctx, owner.ID, id, page.Revision, b); !errors.Is(err, event.ErrBlocked) {
		t.Fatalf("unfinished dependency accepted: %v", err)
	}
	b.OverrideDependencies = true
	b.Reason = ""
	if _, err := s.Save(ctx, owner.ID, id, page.Revision, b); !errors.Is(err, event.ErrBlocked) {
		t.Fatal("unreasoned override accepted")
	}
	b.Reason = "AV can be verified while final venue approval is pending"
	page, err = s.Save(ctx, owner.ID, id, page.Revision, b)
	if err != nil {
		t.Fatal(err)
	}
	a.Definition.Status = "complete"
	a.Reason = "Venue checked"
	a.Definition.Checklist = []ChecklistItem{{ID: uuid.New(), Title: "Confirm capacity"}}
	if _, err := s.Save(ctx, owner.ID, id, page.Revision, a); !errors.Is(err, event.ErrBlocked) {
		t.Fatal("incomplete checklist accepted")
	}
	a.Definition.Status = "waived"
	a.Reason = ""
	if _, err := s.Save(ctx, owner.ID, id, page.Revision, a); !errors.Is(err, event.ErrBlocked) {
		t.Fatal("unreasoned waiver accepted")
	}
	a.Reason = "Venue already approved under a separate contract"
	page, err = s.Save(ctx, owner.ID, id, page.Revision, a)
	if err != nil {
		t.Fatal(err)
	}
	history, err := s.History(ctx, owner.ID, id, a.Definition.ID)
	if err != nil || len(history) != 2 || history[0].Reason != a.Reason ||
		history[1].Definition.Status != "not_started" {
		t.Fatalf("history: %+v %v", history, err)
	}
	if _, err := pool.Exec(
		ctx,
		"UPDATE preparation_task_changes SET document='{}' WHERE event_id=$1",
		id,
	); err == nil {
		t.Fatal("history can be rewritten")
	}
	if _, err := s.List(ctx, outsider.ID, id, Filter{}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("tasks leaked")
	}
	if _, err := pool.Exec(
		ctx,
		"UPDATE people SET platform_admin=true WHERE id=$1",
		outsider.ID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(ctx, outsider.ID, id, Filter{}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("admin implicitly sees event tasks")
	}
	start, results := make(chan struct{}), make(chan error, 2)
	for _, title := range []string{"First edit", "Second edit"} {
		u := a
		u.Definition.Title = title
		go func() { <-start; _, err := s.Save(ctx, owner.ID, id, page.Revision, u); results <- err }()
	}
	close(start)
	wins, conflicts := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			wins++
		} else if errors.Is(err, access.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("race: wins=%d conflicts=%d", wins, conflicts)
	}
	// Task state is not a domain approval or a source of confirmed attendance.
	if err := q.InitializeEventFacts(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(
		ctx,
		"UPDATE events SET stage='registration' WHERE id=$1",
		id,
	); err != nil {
		t.Fatal(err)
	}
	row, err := q.Event(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (event.Service{Pool: pool}).Transition(
		ctx,
		owner.ID,
		id,
		row.Revision,
		"preparation",
		"",
		false,
	); !errors.Is(
		err,
		event.ErrBlocked,
	) {
		t.Fatalf("waiver bypassed minimum: %v", err)
	}
	var leaked int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE event_id=$1 AND metadata::text LIKE '%Venue already%'", id).
		Scan(&leaked); err != nil ||
		leaked != 0 {
		t.Fatal("reason leaked into generic audit")
	}
}

func TestPreparationAssignedStaffEvidenceAndOwnership(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	owner, staff, other := testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool)
	id := testdb.Event(t, pool, owner.ID)
	foreign := testdb.Event(t, pool, other.ID)
	for _, p := range []uuid.UUID{staff.ID, other.ID} {
		if err := q.SetMembership(
			ctx,
			dbgen.SetMembershipParams{EventID: id, PersonID: p, Role: "logistics", Active: true},
		); err != nil {
			t.Fatal(err)
		}
	}
	s := Service{Pool: pool}
	page, err := s.List(ctx, owner.ID, id, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	private := taskInput("Private contract", "organiser")
	page, err = s.Save(ctx, owner.ID, id, page.Revision, private)
	if err != nil {
		t.Fatal(err)
	}
	u := taskInput("Venue readiness", "logistics")
	u.Definition.OwnerID = &staff.ID
	u.Definition.DependsOn = []uuid.UUID{private.Definition.ID}
	u.Definition.Checklist = []ChecklistItem{{ID: uuid.New(), Title: "Microphone test"}}
	fileID := uuid.New()
	_, err = q.InsertFile(
		ctx,
		dbgen.InsertFileParams{
			ID:             fileID,
			EventID:        id,
			OwnerID:        owner.ID,
			IdempotencyKey: uuid.New(),
			ObjectKey:      strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", ""),
			Filename:       "private.txt",
			MediaType:      "text/plain",
			Checksum:       strings.Repeat("a", 64),
			SizeBytes:      1,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	u.Definition.Evidence = []Evidence{
		{
			ID:         uuid.New(),
			Kind:       "url",
			Label:      "private contract",
			URL:        "https://example.test/private-contract",
			Visibility: "organisers",
		},
		{
			ID:         uuid.New(),
			Kind:       "file",
			Label:      "restricted file",
			FileID:     &fileID,
			Visibility: "assignees",
		},
		{
			ID:         uuid.New(),
			Kind:       "repository",
			Label:      "Shared checklist",
			URL:        "https://example.test/checklist",
			Visibility: "assignees",
		},
	}
	page, err = s.Save(ctx, owner.ID, id, page.Revision, u)
	if err != nil {
		t.Fatal(err)
	}
	staffPage, err := s.List(ctx, staff.ID, id, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(staffPage.Items) != 1 || staffPage.Items[0].RestrictedEvidenceCount != 2 ||
		staffPage.Items[0].HiddenDependencies != 1 ||
		len(staffPage.Items[0].Definition.DependsOn) != 0 {
		t.Fatalf("redaction: %+v", staffPage)
	}
	data, _ := json.Marshal(staffPage)
	if strings.Contains(string(data), "private-contract") ||
		strings.Contains(string(data), fileID.String()) ||
		strings.Contains(string(data), private.Definition.ID.String()) {
		t.Fatal("restricted evidence/dependency leaked")
	}
	change := staffPage.Items[0].Update
	change.Definition.Status = "in_progress"
	change.Definition.Checklist[0].Complete = true
	staffPage, err = s.Save(ctx, staff.ID, id, staffPage.Revision, change)
	if err != nil {
		t.Fatal(err)
	}
	page, err = s.List(ctx, owner.ID, id, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	stored := findTask(t, page, u.Definition.ID)
	if len(stored.Definition.Evidence) != 3 || len(stored.Definition.DependsOn) != 1 ||
		!stored.Definition.Checklist[0].Complete {
		t.Fatal("staff save discarded hidden evidence, dependencies, or checked item")
	}
	change = staffPage.Items[0].Update
	change.Definition.Status = "waived"
	change.Reason = "Trying to waive"
	if _, err := s.Save(
		ctx,
		staff.ID,
		id,
		staffPage.Revision,
		change,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("staff can waive")
	}
	change = staffPage.Items[0].Update
	change.Definition.OwnerID = &other.ID
	if _, err := s.Save(
		ctx,
		staff.ID,
		id,
		staffPage.Revision,
		change,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("staff can reassign")
	}
	if _, err := s.History(
		ctx,
		staff.ID,
		id,
		u.Definition.ID,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("staff can see historical private changes")
	}
	foreignFile := uuid.New()
	_, err = q.InsertFile(
		ctx,
		dbgen.InsertFileParams{
			ID:             foreignFile,
			EventID:        foreign,
			OwnerID:        other.ID,
			IdempotencyKey: uuid.New(),
			ObjectKey:      strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", ""),
			Filename:       "foreign.txt",
			MediaType:      "text/plain",
			Checksum:       strings.Repeat("a", 64),
			SizeBytes:      1,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	bad := stored.Update
	bad.Definition.Evidence = []Evidence{
		{
			ID:         uuid.New(),
			Kind:       "file",
			Label:      "foreign",
			FileID:     &foreignFile,
			Visibility: "organisers",
		},
	}
	if _, err := s.Save(
		ctx,
		owner.ID,
		id,
		page.Revision,
		bad,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatalf("cross-event file accepted: %v", err)
	}
	change = stored.Update
	change.Definition.OwnerID = &other.ID
	page, err = s.Save(ctx, owner.ID, id, page.Revision, change)
	if err != nil {
		t.Fatal(err)
	}
	staffPage, err = s.List(ctx, staff.ID, id, Filter{})
	if err != nil || len(staffPage.Items) != 0 {
		t.Fatal("previous owner retained access")
	}
	if _, err := pool.Exec(
		ctx,
		"UPDATE events SET writes_allowed=false WHERE id=$1",
		id,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(
		ctx,
		owner.ID,
		id,
		page.Revision,
		change,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("read-only event editable")
	}
}

func TestPreparationTemplateDatesFollowScheduleChanges(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	owner := testdb.Person(t, pool)
	id := testdb.Event(t, pool, owner.ID)
	c := event.DefaultConfiguration()
	c.LiveStart = time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	agenda := event.AgendaItem{
		ID:       uuid.New(),
		Title:    "Venue ready",
		Audience: []string{"organiser"},
		Anchor:   "live_start",
		Days:     -3,
	}
	c.Schedule = []event.AgendaItem{agenda}
	data, _ := json.Marshal(c)
	if _, err := pool.Exec(
		ctx,
		"UPDATE events SET configuration=$2 WHERE id=$1",
		id,
		data,
	); err != nil {
		t.Fatal(err)
	}
	defs := templates.EmptyDefinitions()
	seed := templates.Task{
		ID:         uuid.New(),
		Title:      "Check venue",
		OwnerRole:  "organiser",
		DependsOn:  []uuid.UUID{},
		ScheduleID: &agenda.ID,
	}
	defs.Tasks = []templates.Task{seed}
	data, _ = json.Marshal(defs)
	if err := q.SaveEventDefinitions(
		ctx,
		dbgen.SaveEventDefinitionsParams{EventID: id, Definitions: data},
	); err != nil {
		t.Fatal(err)
	}
	// Both schedules must remain ahead of the native T−7 gate regardless
	// of the wall date on which this calendar/DST fixture executes.
	clock := func() time.Time { return time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC) }
	s := Service{Pool: pool, Now: clock}
	page, err := s.List(ctx, owner.ID, id, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	first := page.Items[0]
	expected := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	if first.Revision != 0 || first.DueAt == nil || !first.DueAt.Equal(expected) {
		t.Fatalf("template DST seed: %+v", first)
	}
	u := first.Update
	u.Definition.Status = "in_progress"
	page, err = s.Save(ctx, owner.ID, id, page.Revision, u)
	if err != nil {
		t.Fatal(err)
	}
	c.LiveStart = c.LiveStart.AddDate(0, 0, 7)
	_, err = (event.Service{Pool: pool, Clock: clock}).Update(
		ctx,
		owner.ID,
		id,
		page.Revision,
		event.Input{Name: "Updated event", Slug: "updated-event", Configuration: c},
	)
	if err != nil {
		t.Fatal(err)
	}
	page, err = s.List(ctx, owner.ID, id, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	expected = time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)
	if page.Items[0].DueAt == nil || !page.Items[0].DueAt.Equal(expected) ||
		page.Items[0].Definition.Status != "in_progress" {
		t.Fatalf("recomputation lost progress: %+v", page)
	}
}

func TestPreparationRejectsUnsafeEvidence(t *testing.T) {
	for _, value := range []string{"javascript:alert(1)", "file:///etc/passwd", "https://user:password@example.test/a", "https://"} {
		u := taskInput("Safe evidence", "")
		u.Definition.Evidence = []Evidence{
			{ID: uuid.New(), Kind: "url", Label: "reference", URL: value, Visibility: "organisers"},
		}
		if err := validate(u); err == nil {
			t.Fatalf("accepted unsafe reference %q", value)
		}
	}
}
