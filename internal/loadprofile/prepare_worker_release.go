package loadprofile

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissionfreeze"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/teams"
)

type NativeCutoffReceipt struct {
	TeamID       uuid.UUID             `json:"team_id"`
	ObservedWall time.Time             `json:"observed_wall"`
	Native       submissions.Page      `json:"current_native_submission"`
	Capture      submissionfreeze.Page `json:"immutable_capture_source"`
}

type NativeReleaseInputs struct {
	Plan            FactoryPlan
	SourcesJSON     []byte
	KnownPrefixJSON []byte
	Clock           *DockerSetupClock
}

type ReleasePreparation struct {
	Clarifications       []PreparedClarification `json:"native_common_clarifications"`
	KnownPrefixSHA256    string                  `json:"known_prefix_sha256,omitempty"`
	CaptureStartBoundary time.Time               `json:"capture_start_database_wall_boundary"`
	AdmissionPreparation
	OriginalSourcesSHA256   string                 `json:"original_sources_sha256"`
	OriginalAdmissionSHA256 string                 `json:"original_admission_sha256"`
	Step                    string                 `json:"last_setup_step"`
	Worker                  WorkerLeaseObservation `json:"actual_worker_lease"`
	ClockSteps              []SetupClockStep       `json:"private_clock_controls"`
	Released                []content.Page         `json:"native_released_challenges"`
	NativeEvents            []event.View           `json:"current_native_event_stages"`
	Formal                  []submissions.Page     `json:"original_formal_url_acknowledgements"`
	Cutoffs                 []NativeCutoffReceipt  `json:"native_archive_cutoffs"`
	CaptureJobs             []JobStartFact         `json:"actual_capture_attempts"`
	Projects                []PreparedProject      `json:"positive_reviewed_projects"`
}

type factoryRelease struct {
	knownArchiveControl bool
	plan                FactoryPlan
	sources             map[uuid.UUID]PreparedEventSources
	owners              map[uuid.UUID]*Session
	people              map[uuid.UUID]*Session
	preparer            *NativePreparer
	database            NativeDatabase
	clock               *DockerSetupClock
	now                 time.Time
	receipt             *ReleasePreparation
}

// PrepareFactoryRelease performs historical SETUP through the canonical worker
// and native HTTP owners. Normal auth, River scheduling, attempts, transport and
// leases keep real wall time. It cannot certify any sustained runtime target.
func PrepareFactoryRelease(
	ctx context.Context, input NativeReleaseInputs,
	origins map[Surface]string, issuer string, transport http.RoundTripper, database NativeDatabase,
) (receipt ReleasePreparation, returnedErr error) {
	plan, clock := input.Plan, input.Clock
	if err := plan.Validate(); err != nil {
		return receipt, err
	}
	original, err := ParseSourceCheckpoint(input.SourcesJSON)
	if err != nil {
		return receipt, err
	}
	sourceDigest := sha256.Sum256(input.SourcesJSON)
	receipt.OriginalSourcesSHA256 = hex.EncodeToString(sourceDigest[:])
	known, hasKnown, err := parseKnownArchivePrefix(
		input.KnownPrefixJSON,
		plan,
		original,
		receipt.OriginalSourcesSHA256,
		clock,
	)
	if err != nil {
		return receipt, err
	}
	if hasKnown {
		digest := sha256.Sum256(input.KnownPrefixJSON)
		receipt.KnownPrefixSHA256 = hex.EncodeToString(digest[:])
	}
	if clock == nil {
		return receipt, fmt.Errorf("%w: source-pinned private setup clock required", errCohort)
	}
	if err := clock.validate(ctx); err != nil {
		return receipt, err
	}
	if _, err := sourceAdmissionPlan(plan, original.AdmissionPreparation); err != nil {
		return receipt, err
	}
	sources, err := releaseSourcePlan(plan, original)
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
			"%w: exact source installation and zero old sessions required",
			errCohort,
		)
	}
	receipt.OriginalAdmissionSHA256 = original.OriginalAdmissionSHA256
	receipt.Admissions = original.Admissions
	actors, auth, err := AuthenticateCohort(ctx, plan.Cohort, origins, issuer, transport, 20)
	receipt.Authentication = auth
	defer func() { returnedErr = errors.Join(returnedErr, receipt.closeSessions(ctx, actors, database)) }()
	if err != nil {
		return receipt, err
	}
	release := factoryRelease{
		plan:     plan,
		sources:  sources,
		owners:   map[uuid.UUID]*Session{},
		people:   map[uuid.UUID]*Session{},
		preparer: &NativePreparer{},
		database: database,
		clock:    clock,
		now:      plan.SetupStart,
		receipt:  &receipt,
	}
	if hasKnown {
		release.knownArchiveControl = true
		release.now = known.ClockSteps[0].Next
		clock.Steps = append(clock.Steps, known.ClockSteps...)
	}
	defer func() {
		receipt.Calls = append(release.preparer.Calls, receipt.Calls...)
		receipt.ClockSteps = clock.Steps
	}()
	for _, actor := range actors {
		release.people[actor.Session.Person()] = actor.Session
		for _, planned := range plan.Events {
			if actor.ID == planned.OwnerSubject {
				release.owners[planned.Input.ID] = actor.Session
			}
		}
	}
	receipt.Step = "validate-current-sources"
	if err := release.validateCurrent(ctx); err != nil {
		return receipt, err
	}
	receipt.Step = "observe-real-worker-start"
	if err := setupWait(ctx, func(probe context.Context) (bool, error) {
		lease, ready, err := database.WorkerRunning(probe)
		if ready {
			receipt.Worker = lease
		}
		return ready, err
	}); err != nil {
		return receipt, err
	}
	if err := release.archive(ctx); err != nil {
		return receipt, err
	}
	if err := release.live(ctx); err != nil {
		return receipt, err
	}
	events := make([]uuid.UUID, len(plan.Events))
	for i, planned := range plan.Events {
		events[i] = planned.Input.ID
		current, err := prepareCall[event.View](
			ctx,
			release.preparer,
			release.owners[planned.Input.ID],
			http.MethodGet,
			"/api/v1/events/"+planned.Input.ID.String(),
			nil,
			nil,
		)
		if err != nil {
			return receipt, err
		}
		if current.Stage != planned.DesiredStage {
			return receipt, fmt.Errorf(
				"%w: final native event stage differs from declared mixed profile",
				errCohort,
			)
		}
		receipt.NativeEvents = append(receipt.NativeEvents, current)
	}
	receipt.Census, err = database.Admission(ctx, events)
	if err != nil {
		return receipt, err
	}
	if err := validateSetupCensus(receipt.Census); err != nil {
		return receipt, err
	}
	receipt.NativeAdmission = true
	receipt.Step = "native-release-capture-complete-runtime-still-unrun"
	return receipt, nil
}

func releaseSourcePlan(
	plan FactoryPlan,
	original SourcePreparation,
) (map[uuid.UUID]PreparedEventSources, error) {
	if len(original.Sources) != 5 || len(original.OriginalAdmissionSHA256) != 64 {
		return nil, fmt.Errorf("%w: exact five-source original checkpoint required", errCohort)
	}
	sources := map[uuid.UUID]PreparedEventSources{}
	for _, source := range original.Sources {
		if source.Event.ID == uuid.Nil || sources[source.Event.ID].Event.ID != uuid.Nil {
			return nil, fmt.Errorf("%w: duplicate/missing source event", errCohort)
		}
		sources[source.Event.ID] = source
	}
	for _, planned := range plan.Events {
		source, exists := sources[planned.Input.ID]
		if !exists {
			return nil, fmt.Errorf("%w: foreign or missing original source event", errCohort)
		}
		if planned.DesiredStage == "registration" {
			if source.Event.Stage != "registration" {
				return nil, fmt.Errorf("%w: registration source changed", errCohort)
			}
			continue
		}
		if source.Event.Stage != "preparation" || source.Challenge.ID == uuid.Nil ||
			source.Challenge.State != "approved" ||
			source.Challenge.ReleasedAt != nil ||
			source.DeliverableID == uuid.Nil ||
			len(source.Teams.Teams) != 36 ||
			len(source.Mentors.Published) != 72 ||
			len(source.Mentors.PublishedGaps) != 0 ||
			source.Teams.Frozen != (planned.DesiredStage != "preparation") {
			return nil, fmt.Errorf(
				"%w: original native go/teams/challenge/coverage checkpoint incomplete",
				errCohort,
			)
		}
	}
	return sources, nil
}

func (r *factoryRelease) validateCurrent(ctx context.Context) error {
	for _, planned := range r.plan.Events {
		owner := r.owners[planned.Input.ID]
		if owner == nil {
			return fmt.Errorf("%w: current declared owner session missing", errCohort)
		}
		current, err := prepareCall[event.View](
			ctx,
			r.preparer,
			owner,
			http.MethodGet,
			"/api/v1/events/"+planned.Input.ID.String(),
			nil,
			nil,
		)
		if err != nil {
			return err
		}
		if !releaseCurrentMatches(
			current,
			r.sources[planned.Input.ID],
			planned,
			owner.Person(),
			r.knownArchiveControl,
		) {
			return fmt.Errorf(
				"%w: native release source/authority changed; do not replay setup",
				errCohort,
			)
		}
	}
	return nil
}

func (r *factoryRelease) advance(ctx context.Context, next time.Time) error {
	if err := r.clock.Advance(ctx, r.now, next); err != nil {
		return err
	}
	r.now = next
	return nil
}

func (r *factoryRelease) common(ctx context.Context, planned FactoryEvent) error {
	source := r.sources[planned.Input.ID]
	var released content.Page
	if err := setupWait(ctx, func(probe context.Context) (bool, error) {
		current, err := prepareCall[content.Page](
			probe,
			r.preparer,
			r.owners[planned.Input.ID],
			http.MethodGet,
			"/api/v1/events/"+planned.Input.ID.String()+"/content/"+source.Challenge.ID.String()+"?preview=true",
			nil,
			nil,
		)
		if err != nil {
			return false, err
		}
		if current.State == "missed" || current.State == "blocked" {
			return false, fmt.Errorf("%w: native scheduled release failed its gate", errCohort)
		}
		released = current
		return current.State == "released" && current.ReleasedAt != nil, nil
	}); err != nil {
		return err
	}
	publication := planned.Input.Configuration.LiveStart.AddDate(0, 0, -7)
	if released.ID != source.Challenge.ID || released.ReleasedAt == nil ||
		!released.ReleasedAt.Equal(publication) {
		return fmt.Errorf(
			"%w: native challenge publication changed exact T-7 source/time",
			errCohort,
		)
	}
	member, err := r.teamMember(source.Teams.Teams[0])
	if err != nil {
		return err
	}
	public, err := prepareCall[content.Page](
		ctx,
		r.preparer,
		member,
		http.MethodGet,
		"/api/v1/events/"+planned.Input.ID.String()+"/content/"+source.Challenge.ID.String(),
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	if public.ID != released.ID || public.ReleasedVersion == nil ||
		released.ReleasedVersion == nil ||
		*public.ReleasedVersion != *released.ReleasedVersion ||
		public.Document.Rubric == nil ||
		source.Challenge.Document.Rubric == nil ||
		*public.Document.Rubric != *source.Challenge.Document.Rubric {
		return fmt.Errorf(
			"%w: confirmed member cannot read exact common released source",
			errCohort,
		)
	}
	r.receipt.Released = append(r.receipt.Released, public)
	return nil
}

func (r *factoryRelease) archive(ctx context.Context) error {
	planned := r.plan.Events[4]
	if planned.DesiredStage != "archived" {
		return fmt.Errorf("%w: fixed archive stage required", errCohort)
	}
	owner := r.owners[planned.Input.ID]
	source := r.sources[planned.Input.ID]
	r.receipt.Step = "archive-native-T-7-release"
	if !r.knownArchiveControl {
		if err := r.advance(
			ctx,
			planned.Input.Configuration.LiveStart.AddDate(0, 0, -7),
		); err != nil {
			return err
		}
	}
	if err := r.common(ctx, planned); err != nil {
		return err
	}
	r.receipt.Step = "archive-native-live-and-formal-URL"
	if err := r.advance(ctx, planned.Input.Configuration.LiveStart); err != nil {
		return err
	}
	if _, err := r.preparer.Transition(ctx, owner, planned.Input.ID, "live"); err != nil {
		return err
	}
	for _, team := range source.Teams.Teams[:34] {
		member, err := r.teamMember(team)
		if err != nil {
			return err
		}
		formal, err := r.preparer.SubmitURL(
			ctx,
			member,
			planned.Input.ID,
			team.ID,
			source.DeliverableID,
		)
		if err != nil {
			return err
		}
		r.receipt.Formal = append(r.receipt.Formal, formal)
	}
	r.receipt.Step = "archive-native-private-questions-and-common-answers"
	if err := r.clarifications(ctx, planned, source); err != nil {
		return err
	}
	r.receipt.Step = "archive-native-cutoff-capture"
	boundary, err := r.database.DatabaseTime(ctx)
	if err != nil {
		return err
	}
	r.receipt.CaptureStartBoundary = boundary
	if err := r.advance(ctx, planned.Input.Configuration.SubmissionCutoff); err != nil {
		return err
	}
	if err := r.cutoff(ctx, planned, source); err != nil {
		return err
	}
	for _, stage := range []string{"assessment", "completed", "archived"} {
		if _, err := r.preparer.Transition(ctx, owner, planned.Input.ID, stage); err != nil {
			return err
		}
	}
	r.receipt.Step = "archive-positive-native-projects"
	return r.projects(ctx, planned, source, 3)
}

func (r *factoryRelease) live(ctx context.Context) error {
	first := r.plan.Events[2]
	if first.DesiredStage != "live" || r.plan.Events[3].DesiredStage != "live" ||
		!first.Input.Configuration.LiveStart.Equal(r.plan.Events[3].Input.Configuration.LiveStart) {
		return fmt.Errorf("%w: fixed two-live-event source required", errCohort)
	}
	r.receipt.Step = "two-live-native-T-7-release"
	if err := r.advance(ctx, first.Input.Configuration.LiveStart.AddDate(0, 0, -7)); err != nil {
		return err
	}
	for _, planned := range r.plan.Events[2:4] {
		if err := r.common(ctx, planned); err != nil {
			return err
		}
	}
	r.receipt.Step = "two-native-live-stages-and-positive-projects"
	if err := r.advance(ctx, first.Input.Configuration.LiveStart); err != nil {
		return err
	}
	for _, planned := range r.plan.Events[2:4] {
		if _, err := r.preparer.Transition(
			ctx,
			r.owners[planned.Input.ID],
			planned.Input.ID,
			"live",
		); err != nil {
			return err
		}
		if err := r.projects(ctx, planned, r.sources[planned.Input.ID], 2); err != nil {
			return err
		}
		if err := r.clarifications(ctx, planned, r.sources[planned.Input.ID]); err != nil {
			return err
		}
	}
	return nil
}

func (r *factoryRelease) teamMember(team teams.Team) (*Session, error) {
	for _, member := range team.Published {
		if session := r.people[member.ID]; session != nil {
			return session, nil
		}
	}
	return nil, fmt.Errorf("%w: actual current native team member required", errCohort)
}

func (r *factoryRelease) projects(
	ctx context.Context,
	planned FactoryEvent,
	source PreparedEventSources,
	count int,
) error {
	for _, team := range source.Teams.Teams[:count] {
		member, err := r.teamMember(team)
		if err != nil {
			return err
		}
		project, err := r.preparer.PublishTeamProject(
			ctx,
			r.owners[planned.Input.ID],
			member,
			planned.Input.ID,
			team.ID,
			r.plan.Run,
		)
		if err != nil {
			return err
		}
		r.receipt.Projects = append(r.receipt.Projects, project)
	}
	return nil
}

func setupWait(ctx context.Context, probe func(context.Context) (bool, error)) error {
	bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		ready, err := probe(bounded)
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
		select {
		case <-bounded.Done():
			return fmt.Errorf("native setup observation incomplete: %w", bounded.Err())
		case <-ticker.C:
		}
	}
}

func (r *factoryRelease) cutoff(
	ctx context.Context,
	planned FactoryEvent,
	source PreparedEventSources,
) error {
	originals := map[uuid.UUID]submissions.Page{}
	for _, formal := range r.receipt.Formal {
		originals[formal.TeamID] = formal
	}
	if len(originals) != 34 {
		return fmt.Errorf("%w: 34 original formal URL acknowledgements required", errCohort)
	}
	var complete []NativeCutoffReceipt
	if err := setupWait(ctx, func(probe context.Context) (bool, error) {
		complete = nil
		for _, team := range source.Teams.Teams {
			base := "/api/v1/events/" + planned.Input.ID.String() + "/teams/" + team.ID.String()
			native, err := prepareCall[submissions.Page](
				probe,
				r.preparer,
				r.owners[planned.Input.ID],
				http.MethodGet,
				base+"/submission",
				nil,
				nil,
			)
			if err != nil {
				return false, err
			}
			capture, err := prepareCall[submissionfreeze.Page](
				probe,
				r.preparer,
				r.owners[planned.Input.ID],
				http.MethodGet,
				base+"/submission-freeze",
				nil,
				nil,
			)
			if err != nil {
				return false, err
			}
			if native.FrozenAt == nil || capture.Source == nil {
				continue
			}
			original, submitted := originals[team.ID]
			if err := validateURLCutoff(
				planned.Input.ID,
				team.ID,
				planned.Input.Configuration.SubmissionCutoff,
				native,
				capture,
				original,
				submitted,
			); err != nil {
				return false, err
			}
			complete = append(
				complete,
				NativeCutoffReceipt{
					TeamID:       team.ID,
					ObservedWall: time.Now(),
					Native:       native,
					Capture:      capture,
				},
			)
		}
		return len(complete) == 36, nil
	}); err != nil {
		return err
	}
	r.receipt.Cutoffs = complete
	return setupWait(ctx, func(probe context.Context) (bool, error) {
		attempts, err := r.database.CaptureStarts(probe, planned.Input.ID)
		if err != nil {
			return false, err
		}
		r.receipt.CaptureJobs = attempts
		if len(attempts) > 36 {
			return false, fmt.Errorf("%w: duplicate/extra native capture operations", errCohort)
		}
		if len(attempts) != 36 {
			return false, nil
		}
		if r.receipt.CaptureStartBoundary.IsZero() {
			return false, fmt.Errorf("%w: native cutoff wall boundary missing", errCohort)
		}
		for _, attempt := range attempts {
			if !attempt.Due.Equal(planned.Input.Configuration.SubmissionCutoff) ||
				!attempt.Approved ||
				attempt.Cancelled ||
				attempt.Attempts > 1 ||
				attempt.State == "permanent" ||
				attempt.State == "ambiguous" {
				return false, fmt.Errorf(
					"%w: native capture intent/attempt changed or failed",
					errCohort,
				)
			}
			if attempt.FirstStarted != nil &&
				attempt.FirstStarted.Before(r.receipt.CaptureStartBoundary) {
				return false, fmt.Errorf(
					"%w: capture attempt started before the actual cutoff control",
					errCohort,
				)
			}
			if attempt.State != "delivered" || attempt.FirstStarted == nil ||
				attempt.Attempts != 1 {
				return false, nil
			}
		}
		return true, nil
	})
}

func validateURLCutoff(
	eventID, teamID uuid.UUID,
	cutoff time.Time,
	native submissions.Page,
	capture submissionfreeze.Page,
	original submissions.Page,
	submitted bool,
) error {
	source := capture.Source
	if source == nil || source.ID == uuid.Nil || source.EventID != eventID ||
		source.TeamID != teamID ||
		source.EntryID != native.EntryID ||
		!source.Cutoff.Equal(cutoff) ||
		native.FrozenAt == nil ||
		!native.FrozenAt.Equal(cutoff) {
		return fmt.Errorf("%w: native cutoff lost exact event/team/entry/time source", errCohort)
	}
	if !captureHasNoRepository(capture) {
		return fmt.Errorf("%w: URL-only capture invented managed repository evidence", errCohort)
	}

	if submitted {
		if native.Outcome != "frozen" || source.NativeOutcome != "frozen" ||
			source.Status != "not_applicable" ||
			capture.Status != "not_applicable" ||
			!capture.BindingCurrent ||
			source.SubmissionRevision == nil ||
			*source.SubmissionRevision != 1 ||
			native.EntryID != original.EntryID ||
			!reflect.DeepEqual(native.Final, original.Final) {
			return fmt.Errorf(
				"%w: accepted formal URL did not retain its exact immutable native cutoff",
				errCohort,
			)
		}
	} else if native.Outcome != "not_submitted" || source.NativeOutcome != "not_submitted" || source.Status != "not_submitted" || source.SubmissionRevision != nil || native.Final != nil {
		return fmt.Errorf("%w: missing formal source is not explicitly not-submitted", errCohort)
	}
	return nil
}

func ParseSourceCheckpoint(raw []byte) (SourcePreparation, error) {
	var receipt SourcePreparation
	if len(raw) == 0 || len(raw) > 8<<20 {
		return receipt, fmt.Errorf("%w: bounded original source checkpoint required", errCohort)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&receipt) != nil || decoder.Decode(new(any)) != io.EOF {
		return receipt, fmt.Errorf("%w: one exact original source JSON object required", errCohort)
	}
	return receipt, nil
}

func validateSetupCensus(facts []AdmissionFact) error {
	if len(facts) != 5 {
		return fmt.Errorf("%w: complete five-event setup census required", errCohort)
	}
	for _, fact := range facts {
		if fact.ConfiguredCapacity != 180 || fact.Applications != 180 || fact.Confirmed != 180 ||
			fact.ClaimedConfirmed != 180 ||
			fact.Staff != 20 ||
			fact.People != 200 ||
			fact.Waitlisted != 0 ||
			fact.Offered != 0 ||
			fact.DuplicateActive != 0 ||
			fact.ExcludedCompetitors != 0 {
			return fmt.Errorf("%w: setup changed exact180+20 native event census", errCohort)
		}
	}
	return nil
}

func captureHasNoRepository(capture submissionfreeze.Page) bool {
	source := capture.Source
	return source != nil && source.RepositoryID == 0 && source.RepositoryURL == "" &&
		capture.Current == nil &&
		len(capture.Observations) == 0 &&
		len(capture.Decisions) == 0 &&
		len(source.ManagedEvidenceIDs) == 0 &&
		len(source.UncapturedEvidenceIDs) == 0
}

func releaseCurrentMatches(
	current event.View,
	source PreparedEventSources,
	planned FactoryEvent,
	owner uuid.UUID,
	knownArchive bool,
) bool {
	expectedRevision := source.Event.Revision
	publishedAllowed := current.PublishedAt == nil
	if knownArchive && planned.DesiredStage == "archived" && current.PublishedAt != nil {
		publishedAllowed = current.PublishedAt.Equal(
			planned.Input.Configuration.LiveStart.AddDate(0, 0, -7),
		)
		// The guarded native transaction records rubric publication (which bumps
		// the event) and then marks challenge publication (which bumps it again).
		expectedRevision += 2
	}
	return current.ID == planned.Input.ID && current.Revision == expectedRevision &&
		reflect.DeepEqual(current.Configuration, planned.Input.Configuration) &&
		current.Stage == source.Event.Stage &&
		current.OwnerID != nil &&
		*current.OwnerID == owner &&
		current.ConfirmedAttendees == 180 &&
		publishedAllowed
}

func (r *factoryRelease) clarifications(
	ctx context.Context,
	planned FactoryEvent,
	source PreparedEventSources,
) error {
	var common clarification.Source
	for _, released := range r.receipt.Released {
		if released.ID == source.Challenge.ID && released.ReleasedVersion != nil &&
			released.Document.Rubric != nil {
			common = clarification.Source{
				ChallengeID:      released.ID,
				ChallengeVersion: *released.ReleasedVersion,
				Rubric:           *released.Document.Rubric,
			}
		}
	}
	if common.ChallengeID == uuid.Nil {
		return fmt.Errorf(
			"%w: actually released common source required for clarification fixture",
			errCohort,
		)
	}
	for _, team := range source.Teams.Teams[:2] {
		member, err := r.teamMember(team)
		if err != nil {
			return err
		}
		prepared, err := r.preparer.PublishClarification(
			ctx,
			r.owners[planned.Input.ID],
			member,
			planned.Input.ID,
			team.ID,
			common,
			"PRIVATE question "+r.plan.Run.String()+" "+team.ID.String(),
			"Reviewed load clarification "+r.plan.Run.String(),
		)
		// Preserve an original private-question acknowledgement even when a
		// later answer review or public read fails. A partial setup cannot be
		// mistaken for the recognised read-only continuation prefix.
		if prepared.Question.ID != uuid.Nil {
			r.receipt.Clarifications = append(r.receipt.Clarifications, prepared)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
