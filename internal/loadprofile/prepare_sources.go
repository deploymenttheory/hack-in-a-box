package loadprofile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
	"github.com/deploymenttheory/hack-in-a-box/internal/teams"
)

type PreparedEventSources struct {
	Event         event.View   `json:"current_event"`
	Teams         teams.Page   `json:"native_teams"`
	Mentors       mentors.Page `json:"native_mentor_coverage"`
	Challenge     content.Page `json:"approved_challenge"`
	DeliverableID uuid.UUID    `json:"required_url_deliverable"`
}

// SourcePreparation retains the original admission acknowledgements separately
// from new source commands/current views. NativeAdmission remains the census
// fact; neither it nor this phase establishes live publication or load results.
type SourcePreparation struct {
	AdmissionPreparation
	OriginalAdmissionSHA256 string                 `json:"original_admission_sha256"`
	Sources                 []PreparedEventSources `json:"event_sources"`
}

// PrepareFactorySources continues an acknowledged fresh admission checkpoint.
// Every actor reauthenticates normally. It keeps the registration event open,
// takes the other four events through their native go gate, allocates current
// confirmed people, authors a rubric/challenge, and publishes mentor coverage.
// Release/capture still require the actual running shared worker in later phases.
func PrepareFactorySources(
	ctx context.Context, plan FactoryPlan, original AdmissionPreparation,
	origins map[Surface]string, issuer string, transport http.RoundTripper,
	database NativeDatabase,
) (receipt SourcePreparation, returnedErr error) {
	if err := plan.Validate(); err != nil {
		return receipt, err
	}
	admissions, err := sourceAdmissionPlan(plan, original)
	if err != nil {
		return receipt, err
	}
	initial, err := database.Installation(ctx)
	if err != nil {
		return receipt, err
	}
	receipt.InitialInstallation = &initial
	if initial.Events != 5 || initial.People != 1000 || initial.ActivePeople != 1000 ||
		initial.UnexpiredSessions != 0 {
		return receipt, fmt.Errorf(
			"%w: exact admitted installation with all prior sessions closed required",
			errCohort,
		)
	}
	actors, auth, err := AuthenticateCohort(ctx, plan.Cohort, origins, issuer, transport, 20)
	receipt.Authentication = auth
	defer func() { returnedErr = errors.Join(returnedErr, receipt.closeSessions(ctx, actors, database)) }()
	if err != nil {
		return receipt, err
	}
	preparer := &NativePreparer{}
	defer func() { receipt.Calls = append(preparer.Calls, receipt.Calls...) }()
	for _, planned := range plan.Events {
		admission := admissions[planned.Input.ID]
		var cohort []CohortActor
		var owner *Session
		for _, actor := range actors {
			if actor.Event != planned.Input.ID {
				continue
			}
			cohort = append(cohort, actor)
			if actor.ID == planned.OwnerSubject {
				owner = actor.Session
			}
		}
		prepared, err := preparer.prepareEventSources(
			ctx,
			planned,
			admission,
			cohort,
			owner,
			plan.Run,
		)
		receipt.Sources = append(receipt.Sources, prepared)
		if err != nil {
			return receipt, err
		}
		receipt.Admissions = append(receipt.Admissions, admission)
	}
	events := make([]uuid.UUID, len(plan.Events))
	for i, item := range plan.Events {
		events[i] = item.Input.ID
	}
	receipt.Census, err = database.Admission(ctx, events)
	if err != nil {
		return receipt, err
	}
	for _, fact := range receipt.Census {
		if fact.People != 200 || fact.Staff != 20 || fact.Confirmed != 180 ||
			fact.ClaimedConfirmed != 180 ||
			fact.DuplicateActive != 0 {
			return receipt, fmt.Errorf(
				"%w: source preparation changed native admitted census",
				errCohort,
			)
		}
	}
	receipt.NativeAdmission = true
	return receipt, nil
}

func sourceAdmissionPlan(
	plan FactoryPlan,
	original AdmissionPreparation,
) (map[uuid.UUID]PreparedAdmission, error) {
	if !original.NativeAdmission || original.FullLoad || len(original.Admissions) != 5 {
		return nil, fmt.Errorf("%w: original complete admission setup receipt required", errCohort)
	}
	admissions := make(map[uuid.UUID]PreparedAdmission, len(original.Admissions))
	expected := make(map[uuid.UUID]bool, len(plan.Events))
	for _, planned := range plan.Events {
		expected[planned.Input.ID] = true
	}
	for _, admission := range original.Admissions {
		id := admission.Event.ID
		if !expected[id] || admissions[id].Event.ID != uuid.Nil ||
			admission.Event.Stage != "registration" ||
			len(admission.Registrations) != 199 {
			return nil, fmt.Errorf(
				"%w: original admission events/people do not match this exact factory run",
				errCohort,
			)
		}
		people := make(map[uuid.UUID]bool, 199)
		owner := ""
		for _, planned := range plan.Events {
			if planned.Input.ID == id {
				owner = planned.OwnerSubject
			}
		}
		for _, member := range plan.Cohort.Members {
			if member.Event != id || member.Subject == owner {
				continue
			}
			registration, exists := admission.Registrations[member.Subject]
			kind, state := "attendee", "confirmed"
			if member.Staff {
				kind, state = "facilitator", "selected"
			}
			if !exists || registration.ID == uuid.Nil || registration.PersonID == uuid.Nil ||
				registration.EventID != id ||
				registration.Kind != kind ||
				registration.State != state ||
				people[registration.PersonID] {
				return nil, fmt.Errorf(
					"%w: original admission subject/person/state does not match this factory cohort",
					errCohort,
				)
			}
			people[registration.PersonID] = true
		}
		admissions[id] = admission
	}
	return admissions, nil
}

func (p *NativePreparer) prepareEventSources(
	ctx context.Context, planned FactoryEvent, admission PreparedAdmission, cohort []CohortActor,
	owner *Session, run uuid.UUID,
) (PreparedEventSources, error) {
	var prepared PreparedEventSources
	if owner == nil || len(cohort) != 200 {
		return prepared, fmt.Errorf("%w: exact current owner/cohort required", errCohort)
	}
	current, err := prepareCall[event.View](
		ctx,
		p,
		owner,
		http.MethodGet,
		"/api/v1/events/"+planned.Input.ID.String(),
		nil,
		nil,
	)
	if err != nil {
		return prepared, err
	}
	prepared.Event = current
	if current.ID != planned.Input.ID || current.Stage != "registration" ||
		current.OwnerID == nil ||
		*current.OwnerID != owner.Person() ||
		current.ConfirmedAttendees != 180 ||
		current.Configuration.Capacity != 180 ||
		current.PublishedAt != nil {
		return prepared, fmt.Errorf(
			"%w: source preparation requires unchanged native registration checkpoint",
			errCohort,
		)
	}
	if planned.DesiredStage == "registration" {
		return prepared, nil
	}
	prepared.DeliverableID, err = p.ConfigureURLDeliverable(ctx, owner, current.ID)
	if err != nil {
		return prepared, err
	}
	prepared.Challenge, err = p.ApproveChallenge(ctx, owner, current.ID, run)
	if err != nil {
		return prepared, err
	}
	prepared.Event, err = p.Transition(ctx, owner, current.ID, "preparation")
	if err != nil {
		return prepared, err
	}
	prepared.Teams, err = p.PrepareTeams(
		ctx,
		owner,
		admission,
		cohort,
		planned.DesiredStage != "preparation",
	)
	if err != nil {
		return prepared, err
	}
	prepared.Mentors, err = p.PrepareMentors(ctx, owner, planned, cohort, prepared.Teams)
	if err != nil {
		return prepared, err
	}
	prepared.Event, err = prepareCall[event.View](
		ctx,
		p,
		owner,
		http.MethodGet,
		"/api/v1/events/"+current.ID.String(),
		nil,
		nil,
	)
	return prepared, err
}

// ParseAdmissionCheckpoint preserves the exact typed original setup receipt;
// current native reads/commands still establish all continuation authority.
func ParseAdmissionCheckpoint(raw []byte) (AdmissionPreparation, error) {
	var receipt AdmissionPreparation
	if len(raw) == 0 || len(raw) > 8<<20 {
		return receipt, fmt.Errorf("%w: bounded original admission checkpoint required", errCohort)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&receipt) != nil || decoder.Decode(new(any)) != io.EOF {
		return receipt, fmt.Errorf(
			"%w: one exact original admission JSON object required",
			errCohort,
		)
	}
	return receipt, nil
}
