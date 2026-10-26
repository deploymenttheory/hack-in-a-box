package loadprofile

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

// FactoryPlan declares REQUEST identities and explicit fixture actors. It is
// not native state, a population census, permission or an authentication seed.
// The preparer must obtain every resulting native fact through owner HTTP.
type FactoryPlan struct {
	Run              uuid.UUID      `json:"run"`
	RuntimeAnchor    time.Time      `json:"runtime_anchor"`
	SetupStart       time.Time      `json:"historical_setup_start"`
	BootstrapSubject string         `json:"bootstrap_subject"`
	Cohort           CohortPlan     `json:"cohort"`
	Events           []FactoryEvent `json:"events"`
}

type FactoryEvent struct {
	Input        event.Input    `json:"native_create_input"`
	DesiredStage string         `json:"desired_runtime_stage"`
	OwnerSubject string         `json:"owner_subject"`
	Staff        []FactoryStaff `json:"staff"`
}

type FactoryStaff struct {
	Subject string `json:"subject"`
	Role    string `json:"role"`
}

// NewReleaseFactoryPlan fixes the full five-event, 1000-person census. UTC
// avoids incidental DST ambiguity in synthetic schedules; normal production
// scheduling validation still runs on every request. The old archive is setup
// history only, and future due follow-ups are scheduled separately after the
// production runtime's actual warmup boundary.
func NewReleaseFactoryPlan(run uuid.UUID, anchor time.Time) (FactoryPlan, error) {
	plan := FactoryPlan{
		Run: run, RuntimeAnchor: anchor.UTC(), SetupStart: anchor.UTC().AddDate(0, 0, -45),
		BootstrapSubject: "load-e01-p181", Cohort: CohortPlan{Profile: "release"},
	}
	if run == uuid.Nil || anchor.IsZero() {
		return plan, fmt.Errorf(
			"%w: explicit fresh run and runtime wall anchor required",
			errCohort,
		)
	}
	stages := []string{"registration", "preparation", "live", "live", "archived"}
	for index, stage := range stages {
		eventID := uuid.NewSHA1(run, fmt.Appendf(nil, "native-event-%d", index+1))
		configuration := event.DefaultConfiguration()
		configuration.Timezone = "UTC"
		configuration.Capacity = 180
		configuration.MinimumAttendance = 30
		configuration.Description = "Synthetic release capacity fixture; not a public hackathon"
		configuration.Venue = "Synthetic load fixture"
		switch stage {
		case "registration":
			configuration.LiveStart = anchor.UTC().AddDate(0, 0, 14)
		case "preparation":
			configuration.LiveStart = anchor.UTC().AddDate(0, 0, 9)
		case "live":
			configuration.LiveStart = anchor.UTC().Add(-time.Hour)
		case "archived":
			configuration.LiveStart = anchor.UTC().AddDate(0, 0, -31)
		}
		configuration.LiveEnd = configuration.LiveStart.Add(4 * time.Hour)
		configuration.SubmissionCutoff = configuration.LiveEnd
		publication := configuration.LiveStart.AddDate(0, 0, -7)
		configuration.TeamFreeze = publication.Add(-time.Hour)
		configuration.AdmissionCutoff = configuration.TeamFreeze.Add(-time.Hour)
		item := FactoryEvent{
			DesiredStage: stage, OwnerSubject: factorySubject(index+1, 181),
			Input: event.Input{
				ID: eventID, Name: fmt.Sprintf("Load event %d %s", index+1, stage),
				Slug: fmt.Sprintf("load-%s-e%d", run, index+1), Configuration: configuration,
				AcknowledgeCompetitionRestriction: true,
			},
		}
		for person := 1; person <= 200; person++ {
			staff := person > 180
			surface := PublicSurface
			if staff {
				surface = AdminSurface
				item.Staff = append(
					item.Staff,
					FactoryStaff{
						Subject: factorySubject(index+1, person),
						Role:    factoryRole(person),
					},
				)
			}
			plan.Cohort.Members = append(plan.Cohort.Members, CohortMember{
				Subject: factorySubject(
					index+1,
					person,
				),
				Event:   eventID,
				Staff:   staff,
				Surface: surface,
			})
		}
		plan.Events = append(plan.Events, item)
	}
	return plan, plan.Cohort.Validate()
}

func factorySubject(eventIndex, person int) string {
	return fmt.Sprintf("load-e%02d-p%03d", eventIndex, person)
}

func factoryRole(person int) string {
	switch {
	case person <= 182:
		return "organiser"
	case person <= 190:
		return "mentor"
	case person <= 195:
		return "judge"
	case person <= 197:
		return "mc"
	default:
		return "logistics"
	}
}
