package loadprofile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
)

func ParseFactoryPlan(raw []byte) (FactoryPlan, error) {
	var plan FactoryPlan
	if len(raw) == 0 || len(raw) > MaximumPreparedBytes {
		return plan, fmt.Errorf("%w: bounded exact factory request plan required", errCohort)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&plan) != nil || decoder.Decode(new(any)) != io.EOF {
		return plan, fmt.Errorf("%w: factory plan must be one exact JSON object", errCohort)
	}
	return plan, plan.Validate()
}

func (p FactoryPlan) Validate() error {
	canonical, err := NewReleaseFactoryPlan(p.Run, p.RuntimeAnchor)
	if err != nil {
		return err
	}
	// This is a fixed release factory rather than an arbitrary remote-command
	// runner. Only a fresh run UUID and wall anchor vary; no input may add an
	// actor, alter the role/capacity census, retarget an event or rewrite history.
	actual, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("encode private factory plan: %w", err)
	}
	expected, err := json.Marshal(canonical)
	if err != nil {
		return fmt.Errorf("encode canonical factory plan: %w", err)
	}
	if !bytes.Equal(actual, expected) {
		return fmt.Errorf(
			"%w: factory differs from fixed five-event explicit release profile",
			errCohort,
		)
	}
	return nil
}

type AdmissionPreparation struct {
	Authentication      AuthenticationReceipt `json:"authentication"`
	Calls               []PreparationCall     `json:"calls"`
	Admissions          []PreparedAdmission   `json:"admissions"`
	Census              []AdmissionFact       `json:"census"`
	NativeAdmission     bool                  `json:"native_admission_complete"`
	FullLoad            bool                  `json:"full_load"`
	SessionCleanup      SetupSessionCleanup   `json:"setup_session_cleanup"`
	InitialInstallation *InstallationCensus   `json:"initial_installation"`
	AfterCleanup        *InstallationCensus   `json:"installation_after_cleanup"`
}

// PrepareFactoryAdmission is one real executable setup phase. It authenticates
// all declared external actors, creates five native events, transfers owners,
// removes former bootstrap event grants and submits/selects the actual cohort.
// It is not the later freeze/release/archive/export or measured load phase.
func PrepareFactoryAdmission(
	ctx context.Context,
	plan FactoryPlan,
	origins map[Surface]string,
	issuer string,
	transport http.RoundTripper,
	database NativeDatabase,
) (receipt AdmissionPreparation, returnedErr error) {
	if err := plan.Validate(); err != nil {
		return receipt, err
	}
	events := make([]uuid.UUID, len(plan.Events))
	for index, item := range plan.Events {
		events[index] = item.Input.ID
	}
	if database.pool == nil {
		return receipt, fmt.Errorf(
			"%w: independent read-only admission observer required",
			errCohort,
		)
	}
	if err := database.scoped(events); err != nil {
		return receipt, err
	}
	initial, err := database.Installation(ctx)
	if err != nil {
		return receipt, err
	}
	receipt.InitialInstallation = &initial
	if initial.Events != 0 || initial.People != 0 || initial.UnexpiredSessions != 0 {
		return receipt, fmt.Errorf(
			"%w: native factory requires a fresh isolated installation",
			errCohort,
		)
	}
	actors, authentication, err := AuthenticateCohort(
		ctx,
		plan.Cohort,
		origins,
		issuer,
		transport,
		20,
	)
	receipt.Authentication = authentication
	defer func() {
		returnedErr = errors.Join(returnedErr, receipt.closeSessions(ctx, actors, database))
	}()
	if err != nil {
		return receipt, err
	}
	var bootstrap *Session
	for _, actor := range actors {
		if actor.ID == plan.BootstrapSubject {
			bootstrap = actor.Session
		}
	}
	if bootstrap == nil {
		return receipt, fmt.Errorf("%w: declared native bootstrap session missing", errCohort)
	}
	preparer := &NativePreparer{}
	for _, item := range plan.Events {
		var cohort []CohortActor
		for _, actor := range actors {
			if actor.Event == item.Input.ID {
				cohort = append(cohort, actor)
			}
		}
		admission, err := preparer.PrepareAdmission(ctx, bootstrap, item, cohort)
		receipt.Calls = preparer.Calls
		receipt.Admissions = append(receipt.Admissions, admission)
		if err != nil {
			return receipt, err
		}
	}
	receipt.Census, err = database.Admission(ctx, events)
	if err != nil {
		return receipt, err
	}
	for _, fact := range receipt.Census {
		if fact.ConfiguredCapacity != 180 || fact.Applications != 180 || fact.Confirmed != 180 ||
			fact.ClaimedConfirmed != 180 || fact.Staff != 20 || fact.People != 200 || fact.Waitlisted != 0 ||
			fact.Offered != 0 || fact.DuplicateActive != 0 || fact.ExcludedCompetitors != 0 {
			return receipt, fmt.Errorf(
				"%w: independent native event census differs from180+20",
				errCohort,
			)
		}
	}
	receipt.NativeAdmission = true
	return receipt, nil
}

func (r *AdmissionPreparation) closeSessions(
	ctx context.Context,
	actors []CohortActor,
	database NativeDatabase,
) error {
	// Cancellation interrupts setup itself. Cleanup still makes one bounded
	// normal logout attempt per known jar; a lost original response stays lost.
	cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer cancel()
	cleanup, calls, cleanupErr := closeSetupSessions(cleanupContext, actors)
	r.SessionCleanup = cleanup
	r.Calls = append(r.Calls, calls...)
	for _, actor := range actors {
		actor.Session.Close()
	}
	census, censusErr := database.Installation(cleanupContext)
	if censusErr != nil {
		return errors.Join(cleanupErr, censusErr)
	}
	r.AfterCleanup = &census
	if census.UnexpiredSessions != 0 || (r.NativeAdmission &&
		(census.Events != 5 || census.People != 1000 || census.ActivePeople != 1000)) {
		return errors.Join(
			cleanupErr,
			fmt.Errorf("%w: native setup installation/session handover census differs", errCohort),
		)
	}
	return cleanupErr
}
