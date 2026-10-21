package registration

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/permissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestFacilitatorSelectionUsesAtomicEligibilityAndNoSeats(t *testing.T) {
	f := newRegistrationFixture(t, 2)
	ctx := t.Context()
	person := testdb.Person(t, f.pool)
	outsider := testdb.Person(t, f.pool)
	attendee, err := f.service.Apply(ctx, person.ID, f.event.ID, f.application())
	if err != nil {
		t.Fatal(err)
	}
	d := forms.Definition{
		ID:       uuid.New(),
		Name:     "Mentor application",
		Audience: "mentor",
		Fields:   []forms.Field{},
	}
	saved, err := f.service.SaveForm(ctx, f.owner.ID, f.event.ID, 0, d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.PublishForm(
		ctx,
		f.owner.ID,
		f.event.ID,
		d.ID,
		saved.Revision,
		saved.Version,
		true,
	); err != nil {
		t.Fatal(err)
	}
	application, err := f.service.Apply(
		ctx,
		person.ID,
		f.event.ID,
		Application{
			ID:                 uuid.New(),
			FormID:             d.ID,
			FormVersion:        1,
			TeamPreference:     "solo",
			Answers:            forms.Answers{},
			FallbackToAttendee: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	input := ReviewInput{Action: "selected", Reason: "Relevant mentoring experience"}
	if _, err := f.service.Review(
		ctx,
		outsider.ID,
		f.event.ID,
		application.ID,
		application.Revision,
		input,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatalf("outsider reviewed: %v", err)
	}
	if _, err := f.service.Review(
		ctx,
		f.owner.ID,
		f.event.ID,
		application.ID,
		application.Revision,
		input,
	); !errors.Is(
		err,
		permissions.ErrAssessment,
	) {
		t.Fatalf("selected competing mentor: %v", err)
	}
	unchanged, err := f.service.Own(ctx, person.ID, f.event.ID)
	if err != nil || unchanged[0].State != "received" {
		t.Fatalf("failed selection was not atomic: %+v %v", unchanged, err)
	}
	if _, err := f.service.Respond(
		ctx,
		person.ID,
		f.event.ID,
		attendee.ID,
		attendee.Revision,
		"withdraw",
	); err != nil {
		t.Fatal(err)
	}
	selected, err := f.service.Review(
		ctx,
		f.owner.ID,
		f.event.ID,
		application.ID,
		application.Revision,
		input,
	)
	if err != nil || selected.State != "selected" {
		t.Fatalf("select mentor: %+v %v", selected, err)
	}
	q := dbgen.New(f.pool)
	roles, err := q.ActiveRoles(
		ctx,
		dbgen.ActiveRolesParams{EventID: f.event.ID, PersonID: person.ID},
	)
	if err != nil || len(roles) != 1 || roles[0] != "mentor" {
		t.Fatalf("missing selected role: %v %v", roles, err)
	}
	count, err := q.ConfirmedAttendees(ctx, f.event.ID)
	if err != nil || count != 0 {
		t.Fatalf("facilitator consumed a place: %d %v", count, err)
	}
	declined, err := f.service.Review(
		ctx,
		f.owner.ID,
		f.event.ID,
		application.ID,
		selected.Revision,
		ReviewInput{Action: "declined", Reason: "Applicant preference"},
	)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := f.service.Applicant(ctx, f.owner.ID, f.event.ID, application.ID)
	if err != nil || !detail.FallbackAvailable {
		t.Fatalf("fallback unavailable: %+v %v", detail, err)
	}
	if _, err := f.service.Applicant(
		ctx,
		outsider.ID,
		f.event.ID,
		application.ID,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatalf("private applicant leaked: %v", err)
	}
	invite := ReviewInput{Action: "invite", Reason: "Invite to apply individually"}
	for range 2 {
		if _, err := f.service.Review(
			ctx,
			f.owner.ID,
			f.event.ID,
			application.ID,
			declined.Revision,
			invite,
		); err != nil {
			t.Fatal(err)
		}
	}
	count, err = q.ConfirmedAttendees(ctx, f.event.ID)
	if err != nil || count != 0 {
		t.Fatal("fallback silently enrolled person")
	}
	var notices int
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM operations WHERE event_id=$1 AND kind='registration.fallback'", f.event.ID).
		Scan(&notices); err != nil ||
		notices != 1 {
		t.Fatalf("duplicate fallback notices: %d %v", notices, err)
	}
}
