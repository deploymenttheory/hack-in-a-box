package templates

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func fixture() Bundle {
	c := event.DefaultConfiguration()
	c.LiveStart = time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	c.SubmissionCutoff = c.LiveEnd
	c.TeamFreeze = c.LiveStart.AddDate(0, 0, -8)
	c.AdmissionCutoff = c.TeamFreeze
	agenda := uuid.New()
	c.Schedule = []event.AgendaItem{
		{ID: agenda, Title: "Welcome", Audience: []string{"all"}, Anchor: "live_start"},
	}
	d := EmptyDefinitions()
	f := uuid.New()
	task := uuid.New()
	d.Forms = []Form{
		{
			ID:       uuid.New(),
			Name:     "Registration",
			Audience: "attendee",
			Fields: []Field{
				{ID: f, Label: "Choice", Type: "single", Choices: []string{"yes", "no"}},
				{
					ID:          uuid.New(),
					Label:       "Details",
					Type:        "text",
					Choices:     []string{},
					VisibleWhen: &Condition{FieldID: f, Equals: "yes"},
				},
			},
		},
	}
	d.Tasks = []Task{
		{
			ID:         task,
			Title:      "Prepare",
			OwnerRole:  "organiser",
			DependsOn:  []uuid.UUID{},
			ScheduleID: &agenda,
		},
		{ID: uuid.New(), Title: "Check", OwnerRole: "organiser", DependsOn: []uuid.UUID{task}},
	}
	d.Deliverables = []Deliverable{{ID: uuid.New(), Title: "Demo", ScheduleID: &agenda}}
	d.RoleBriefs = []RoleBrief{{ID: uuid.New(), Role: "mentor", Body: "Guide teams"}}
	d.Rubrics = []Rubric{
		{
			ID:       uuid.New(),
			Name:     "Final",
			Criteria: []Criterion{{ID: uuid.New(), Label: "Demo", MaxScore: 10, Weight: 1}},
		},
	}
	d.Messages = []Message{
		{
			ID:         uuid.New(),
			Name:       "Welcome",
			Subject:    "Hello",
			Body:       "Welcome",
			Audience:   []string{"attendee"},
			ScheduleID: &agenda,
		},
	}
	var asset bytes.Buffer
	_ = png.Encode(&asset, image.NewRGBA(image.Rect(0, 0, 1, 1)))
	d.Branding.Assets = []Asset{
		{ID: uuid.New(), Kind: "logo", MediaType: "image/png", Content: asset.Bytes()},
	}
	return Bundle{Event: c, Definitions: d}
}

func cloneInput() CloneInput {
	return CloneInput{
		ID:                                uuid.New(),
		Name:                              "Cloned event",
		Slug:                              "clone-" + uuid.NewString(),
		LiveStart:                         time.Date(2027, 3, 29, 9, 0, 0, 0, time.UTC),
		Timezone:                          "Europe/London",
		Version:                           1,
		AcknowledgeCompetitionRestriction: true,
	}
}

func TestRemapReferencesDatesAndInvalidDefinitions(t *testing.T) {
	source := fixture()
	input := cloneInput()
	result, err := Remap(source, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Definitions.Forms[0].Fields[1].VisibleWhen.FieldID != result.Definitions.Forms[0].Fields[0].ID ||
		result.Definitions.Tasks[1].DependsOn[0] != result.Definitions.Tasks[0].ID ||
		*result.Definitions.Messages[0].ScheduleID != result.Event.Schedule[0].ID ||
		*result.Definitions.Deliverables[0].ScheduleID != result.Event.Schedule[0].ID {
		t.Fatal("internal references not remapped")
	}
	originalIDs := map[uuid.UUID]bool{}
	visitIDs(&source, func(id *uuid.UUID) { originalIDs[*id] = true })
	visitIDs(&result, func(id *uuid.UUID) {
		if originalIDs[*id] {
			t.Fatal("reused source identity")
		}
	})
	result.Definitions.Branding.Assets[0].Content[0] = 0
	if source.Definitions.Branding.Assets[0].Content[0] == 0 {
		t.Fatal("branding bytes aliased")
	}
	retry, err := Remap(source, input)
	if err != nil || retry.Definitions.Tasks[0].ID != result.Definitions.Tasks[0].ID {
		t.Fatal("retry remapping unstable")
	}
	for _, mutate := range []func(*Bundle){func(b *Bundle) { b.Definitions.Tasks[0].DependsOn = []uuid.UUID{b.Definitions.Tasks[1].ID} }, func(b *Bundle) { b.Definitions.Messages[0].ScheduleID = new(uuid.New()) }, func(b *Bundle) { b.Definitions.Branding.Assets[0].MediaType = "image/svg+xml" }, func(b *Bundle) { b.Definitions.Forms[0].Fields[1].VisibleWhen.FieldID = uuid.New() }} {
		b := fixture()
		mutate(&b)
		if Validate(b) == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
}

func TestImmutableTemplateSnapshotsAndCloneIsolation(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	owner := testdb.Person(t, pool)
	testdb.Event(t, pool, owner.ID)
	s := Service{Pool: pool}
	input := Input{ID: uuid.New(), Name: "Reusable", Configuration: fixture()}
	template, err := s.Create(ctx, owner.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	outsider := testdb.Person(t, pool)
	if _, err := s.Read(ctx, outsider.ID, template.ID, 1); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("template leaked to outsider")
	}
	clone := cloneInput()
	if _, err := s.CloneTemplate(
		ctx,
		owner.ID,
		template.ID,
		clone,
	); !errors.Is(
		err,
		access.ErrInvalid,
	) {
		t.Fatal("unpublished template cloned")
	}
	template, err = s.Publish(ctx, owner.ID, template.ID, template.Revision, 1)
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.CloneTemplate(ctx, owner.ID, template.ID, clone)
	if err != nil {
		t.Fatal(err)
	}
	if created.Stage != "draft" || created.PublishedAt != nil || created.ConfirmedAttendees != 0 {
		t.Fatal("clone inherited live state")
	}
	if created.ChallengePublication == nil ||
		!created.ChallengePublication.Equal(time.Date(2027, 3, 22, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf(
			"challenge publication lost event-local calendar semantics: %v",
			created.ChallengePublication,
		)
	}
	schedule, err := q.ScheduleItems(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range schedule {
		if item.Approved || item.ExecutedAt.Valid || item.OperationID != nil {
			t.Fatal("schedule inherited delivery/approval")
		}
	}
	snap, err := s.Snapshot(ctx, owner.ID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(snap)
	input.Configuration.Definitions.Forms[0].Name = "Edited source"
	edited, err := s.Edit(ctx, owner.ID, template.ID, template.Revision, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(ctx, owner.ID, template.ID, edited.Revision, 2); err != nil {
		t.Fatal(err)
	}
	old, err := s.Read(ctx, owner.ID, template.ID, 1)
	if err != nil || old.Configuration.Definitions.Forms[0].Name != "Registration" {
		t.Fatal("old version mutated")
	}
	snap, _ = s.Snapshot(ctx, owner.ID, created.ID)
	after, _ := json.Marshal(snap)
	if !bytes.Equal(before, after) {
		t.Fatal("source publication mutated destination")
	}
	snap.Definitions.Forms[0].Name = "Destination edit"
	if _, err := s.SaveDefinitions(
		ctx,
		owner.ID,
		created.ID,
		created.Revision,
		snap.Definitions,
	); err != nil {
		t.Fatal(err)
	}
	again, err := s.CloneTemplate(ctx, owner.ID, template.ID, clone)
	if err != nil || again.ID != created.ID {
		t.Fatalf("clone retry: %v", err)
	}
	snap, _ = s.Snapshot(ctx, owner.ID, created.ID)
	if snap.Definitions.Forms[0].Name != "Destination edit" {
		t.Fatal("retry overwrote destination")
	}
	for _, table := range []string{"template_versions", "template_publications"} {
		if _, err := pool.Exec(
			ctx,
			"DELETE FROM "+table+" WHERE template_id=$1",
			template.ID,
		); err == nil {
			t.Fatal("immutable history deleted")
		}
	}
	clone.ID = uuid.New()
	clone.Slug = "rejected-" + uuid.NewString()
	clone.AcknowledgeCompetitionRestriction = false
	if _, err := s.CloneTemplate(ctx, owner.ID, template.ID, clone); err == nil {
		t.Fatal("missing eligibility acknowledgment")
	}
	if _, err := q.Event(ctx, clone.ID); err == nil {
		t.Fatal("failed clone left event")
	}
}

func TestEventCloneExcludesOperationalRowsAndRequiresCurrentAccess(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	owner := testdb.Person(t, pool)
	testdb.Event(t, pool, owner.ID)
	bundle := fixture()
	events := event.Service{Pool: pool}
	s := Service{Pool: pool}
	source, err := events.Create(
		ctx,
		owner.ID,
		event.Input{
			ID:                                uuid.New(),
			Name:                              "Source",
			Slug:                              "source-" + uuid.NewString(),
			Configuration:                     bundle.Event,
			AcknowledgeCompetitionRestriction: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveDefinitions(
		ctx,
		owner.ID,
		source.ID,
		source.Revision,
		bundle.Definitions,
	); err != nil {
		t.Fatal(err)
	}
	person := testdb.Person(t, pool)
	if err := q.SetCompetitionState(
		ctx,
		dbgen.SetCompetitionStateParams{
			EventID:  source.ID,
			PersonID: person.ID,
			State:    "confirmed",
		},
	); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		"INSERT INTO operational_profiles(event_id,person_id,dietary) VALUES($1,$2,'private')",
		"INSERT INTO policy_resources(event_id,id,kind) VALUES($1,$2,'assessment')",
		"UPDATE events SET published_at=now(),team_frozen_at=now(),stage='live' WHERE id=$1 AND owner_id=$2",
	} {
		id := person.ID
		if sql[0] == 'U' {
			id = owner.ID
		}
		if _, err := pool.Exec(ctx, sql, source.ID, id); err != nil {
			t.Fatal(err)
		}
	}
	cloned, err := s.CloneEvent(ctx, owner.ID, source.ID, cloneInput())
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"competition_claims", "operational_profiles", "policy_resources", "resource_assignments", "event_decisions", "event_notices", "operations"} {
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE event_id=$1", cloned.ID).
			Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("clone copied %s", table)
		}
	}
	row, _ := q.Event(ctx, cloned.ID)
	if row.PublishedAt.Valid || row.TeamFrozenAt.Valid || row.Stage != "draft" {
		t.Fatal("realised gate state copied")
	}
	if _, err := s.CloneEvent(
		ctx,
		person.ID,
		source.ID,
		cloneInput(),
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("attendee cloned privileged configuration")
	}
	if err := q.SetMembership(
		ctx,
		dbgen.SetMembershipParams{
			EventID:  source.ID,
			PersonID: owner.ID,
			Role:     "organiser",
			Active:   false,
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CloneEvent(
		ctx,
		owner.ID,
		source.ID,
		cloneInput(),
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("revoked organiser cloned source")
	}
}
