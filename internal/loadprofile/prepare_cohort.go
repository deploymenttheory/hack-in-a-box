package loadprofile

import (
	"context"
	"fmt"
	"net/http"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
)

type PreparedAdmission struct {
	Event         event.View                   `json:"event"`
	AttendeeForm  registration.FormView        `json:"attendee_form"`
	Registrations map[string]registration.View `json:"registrations"`
}

// PrepareAdmission uses the explicit already-authenticated cohort. All 180
// competitor applications and 19 additional facilitator selections use normal
// native forms/review commands. The declared owner is the twentieth staff
// member; a former bootstrap owner is actively removed before the census.
func (p *NativePreparer) PrepareAdmission(
	ctx context.Context,
	bootstrap *Session,
	planned FactoryEvent,
	actors []CohortActor,
) (PreparedAdmission, error) {
	bundle := PreparedAdmission{Registrations: make(map[string]registration.View)}
	bySubject := make(map[string]CohortActor, len(actors))
	for _, actor := range actors {
		if actor.Session == nil || actor.Event != planned.Input.ID {
			return bundle, fmt.Errorf("%w: exact event's authenticated cohort required", errCohort)
		}
		if _, duplicate := bySubject[actor.ID]; duplicate {
			return bundle, fmt.Errorf("%w: duplicate setup subject", errCohort)
		}
		bySubject[actor.ID] = actor
	}
	owner, exists := bySubject[planned.OwnerSubject]
	if !exists || !owner.Staff || len(actors) != 200 || len(planned.Staff) != 20 {
		return bundle, fmt.Errorf(
			"%w: 200 explicit people including declared owner and twenty staff required",
			errCohort,
		)
	}
	created, err := p.CreateEvent(ctx, bootstrap, planned.Input)
	if err != nil {
		return bundle, err
	}
	if err := p.assignOwner(ctx, bootstrap, owner.Session, created); err != nil {
		return bundle, err
	}
	form, err := p.PublishForm(ctx, owner.Session, created.ID, "attendee")
	if err != nil {
		return bundle, err
	}
	bundle.AttendeeForm = form
	bundle.Event, err = p.Transition(ctx, owner.Session, created.ID, "registration")
	if err != nil {
		return bundle, err
	}
	for _, actor := range actors {
		if actor.Staff {
			continue
		}
		applied, err := p.ApplyForm(ctx, actor.Session, created.ID, form)
		if err != nil {
			return bundle, err
		}
		if applied.State != "confirmed" || applied.Kind != "attendee" {
			return bundle, fmt.Errorf(
				"%w: native capacity did not confirm intended competitor",
				errCohort,
			)
		}
		bundle.Registrations[actor.ID] = applied
	}
	staffForms := make(map[string]registration.FormView)
	for _, staff := range planned.Staff {
		actor, exists := bySubject[staff.Subject]
		if !exists || !actor.Staff {
			return bundle, fmt.Errorf("%w: selected staff absent from declared cohort", errCohort)
		}
		if staff.Subject == planned.OwnerSubject {
			continue
		}
		form, exists := staffForms[staff.Role]
		if !exists {
			var err error
			form, err = p.PublishForm(ctx, owner.Session, created.ID, staff.Role)
			if err != nil {
				return bundle, err
			}
			staffForms[staff.Role] = form
		}
		selected, err := p.SelectStaff(ctx, owner.Session, actor.Session, created.ID, form)
		if err != nil {
			return bundle, err
		}
		bundle.Registrations[staff.Subject] = selected
	}
	path := "/api/v1/events/" + created.ID.String()
	bundle.Event, err = prepareCall[event.View](
		ctx,
		p,
		owner.Session,
		http.MethodGet,
		path,
		nil,
		nil,
	)
	if err == nil && bundle.Event.ConfirmedAttendees != 180 {
		err = fmt.Errorf("%w: native event count includes staff or lost competitors", errCohort)
	}
	return bundle, err
}

func (p *NativePreparer) assignOwner(
	ctx context.Context,
	bootstrap, owner *Session,
	created event.View,
) error {
	if bootstrap.Person() == owner.Person() {
		return nil
	}
	path := "/api/v1/events/" + created.ID.String()
	type receipt struct {
		Revision int64 `json:"revision"`
	}
	transferred, err := prepareCall[receipt](
		ctx,
		p,
		bootstrap,
		http.MethodPut,
		path+"/owner",
		&created.Revision,
		map[string]any{"person_id": owner.Person(), "acknowledge_competition_restriction": true},
	)
	if err != nil {
		return err
	}
	if transferred.Revision <= created.Revision {
		return fmt.Errorf("%w: native owner transfer revision missing", errCohort)
	}
	current, err := prepareCall[event.View](ctx, p, owner, http.MethodGet, path, nil, nil)
	if err != nil {
		return err
	}
	if current.OwnerID == nil || *current.OwnerID != owner.Person() {
		return fmt.Errorf("%w: current native owner differs from transfer ACK", errCohort)
	}
	removed, err := prepareCall[receipt](
		ctx,
		p,
		owner,
		http.MethodPut,
		path+"/staff",
		&current.Revision,
		map[string]any{
			"person_id":                           bootstrap.Person(),
			"role":                                "organiser",
			"active":                              false,
			"acknowledge_competition_restriction": true,
		},
	)
	if err != nil {
		return err
	}
	if removed.Revision <= current.Revision {
		return fmt.Errorf("%w: native former-owner removal revision missing", errCohort)
	}
	staff, err := prepareCall[contract.StaffPage](
		ctx,
		p,
		owner,
		http.MethodGet,
		path+"/staff",
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	for _, member := range staff.Items {
		if member.PersonId == bootstrap.Person() && member.Active {
			return fmt.Errorf(
				"%w: bootstrap still adds an undeclared event staff member",
				errCohort,
			)
		}
	}
	return nil
}
