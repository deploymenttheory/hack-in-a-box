package loadprofile

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
)

func TestFactorySourceContinuationRejectsForeignOrReassignedAdmission(t *testing.T) {
	plan, err := NewReleaseFactoryPlan(
		uuid.New(),
		time.Date(2026, time.October, 26, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	// These are synthetic receipt inputs for the pre-I/O boundary, not native
	// admission proof. Actual TLS/HTTP/census execution has its own dated receipt.
	original := func() AdmissionPreparation {
		receipt := AdmissionPreparation{NativeAdmission: true}
		for _, planned := range plan.Events {
			admission := PreparedAdmission{
				Event:         event.View{ID: planned.Input.ID, Stage: "registration"},
				Registrations: map[string]registration.View{},
			}
			for _, member := range plan.Cohort.Members {
				if member.Event != planned.Input.ID || member.Subject == planned.OwnerSubject {
					continue
				}
				kind, state := "attendee", "confirmed"
				if member.Staff {
					kind, state = "facilitator", "selected"
				}
				admission.Registrations[member.Subject] = registration.View{
					ID:       uuid.New(),
					PersonID: uuid.New(),
					EventID:  planned.Input.ID,
					Kind:     kind,
					State:    state,
				}
			}
			receipt.Admissions = append(receipt.Admissions, admission)
		}
		return receipt
	}
	if _, err := sourceAdmissionPlan(plan, original()); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*AdmissionPreparation){
		"different-run":   func(r *AdmissionPreparation) { r.Admissions[0].Event.ID = uuid.New() },
		"duplicate-event": func(r *AdmissionPreparation) { r.Admissions[1] = r.Admissions[0] },
		"changed-stage":   func(r *AdmissionPreparation) { r.Admissions[0].Event.Stage = "preparation" },
		"foreign-subject": func(r *AdmissionPreparation) {
			for key, value := range r.Admissions[0].Registrations {
				delete(r.Admissions[0].Registrations, key)
				r.Admissions[0].Registrations["foreign-subject"] = value
				break
			}
		},
		"wrong-native-event": func(r *AdmissionPreparation) {
			for key, value := range r.Admissions[0].Registrations {
				value.EventID = uuid.New()
				r.Admissions[0].Registrations[key] = value
				break
			}
		},
		"withdrawn-competitor": func(r *AdmissionPreparation) {
			for key, value := range r.Admissions[0].Registrations {
				if value.Kind == "attendee" {
					value.State = "withdrawn"
					r.Admissions[0].Registrations[key] = value
					break
				}
			}
		},
		"duplicate-native-person": func(r *AdmissionPreparation) {
			var person uuid.UUID
			for key, value := range r.Admissions[0].Registrations {
				if person == uuid.Nil {
					person = value.PersonID
					continue
				}
				value.PersonID = person
				r.Admissions[0].Registrations[key] = value
				break
			}
		},
		"setup-promoted-to-load": func(r *AdmissionPreparation) { r.FullLoad = true },
	} {
		t.Run(name, func(t *testing.T) {
			receipt := original()
			mutate(&receipt)
			if _, err := sourceAdmissionPlan(plan, receipt); err == nil {
				t.Fatal("altered original context accepted before native source commands")
			}
		})
	}
}
