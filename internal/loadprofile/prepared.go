package loadprofile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
)

const MaximumPreparedBytes = 16 << 20

// PreparedFixture is a PRIVATE export of actual native setup results. IDs,
// visibility allowlists and display capabilities must come from acknowledged
// owner commands, never from SQL insertion. Validation checks shape only;
// runtime native authorization and independent database oracles still apply.
// It is not a public report and must be stored with mode 0600.
type PreparedFixture struct {
	Run      uuid.UUID          `json:"run"`
	Cohort   CohortPlan         `json:"cohort"`
	Origins  map[Surface]string `json:"origins"`
	Issuer   string             `json:"issuer"`
	Events   []PreparedEvent    `json:"events"`
	Journeys map[string]Journey `json:"journeys"`
	Burst    PreparedBurst      `json:"burst"`
	Displays []PreparedDisplay  `json:"displays"`
	Due      []DueExpectation   `json:"due"`
}

type PreparedEvent struct {
	ID    uuid.UUID `json:"id"`
	Stage string    `json:"stage"`
}

type PreparedBurst struct {
	Config       BurstConfig                         `json:"config"`
	Applications map[string]registration.Application `json:"applications"`
}

type PreparedDisplay struct {
	ID        string    `json:"id"`
	Event     uuid.UUID `json:"event"`
	Token     string    `json:"token"`
	Forbidden []string  `json:"forbidden"`
}

func ParsePreparedFixture(raw []byte) (PreparedFixture, error) {
	var fixture PreparedFixture
	if len(raw) == 0 || len(raw) > MaximumPreparedBytes {
		return fixture, fmt.Errorf("%w: bounded private native fixture required", errCohort)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&fixture) != nil || decoder.Decode(new(any)) != io.EOF {
		return fixture, fmt.Errorf("%w: native fixture must be one exact JSON object", errCohort)
	}
	return fixture, fixture.Validate()
}

func (f PreparedFixture) Validate() error {
	if err := f.Cohort.Validate(); err != nil {
		return err
	}
	if f.Run == uuid.Nil || len(f.Events) == 0 || len(f.Events) > 5 ||
		len(f.Journeys) != len(f.Cohort.Members) {
		return fmt.Errorf("%w: actual run, event scope and every actor journey required", errCohort)
	}
	for _, surface := range []Surface{PublicSurface, AdminSurface} {
		if err := validateSignInOrigins(surface, f.Origins[surface], f.Issuer); err != nil {
			return err
		}
	}
	if len(f.Origins) != 2 || f.Origins[PublicSurface] == f.Origins[AdminSurface] {
		return fmt.Errorf("%w: separate exact public and admin origins required", errCohort)
	}
	events, stages, err := f.eventScope()
	if err != nil {
		return err
	}
	subjects, err := f.validateJourneys(events)
	if err != nil {
		return err
	}
	burstHomes, err := f.validateBurst(events, subjects)
	if err != nil {
		return err
	}
	liveStreams, err := f.validateDisplays(events)
	if err != nil {
		return err
	}
	for _, intent := range f.Due {
		if events[intent.Event] == "" || intent.ID == uuid.Nil || intent.Kind == "" ||
			intent.Key == "" ||
			intent.Due.IsZero() {
			return fmt.Errorf(
				"%w: actual native due intent must remain inside prepared event scope",
				errCohort,
			)
		}
	}
	return f.validateReleaseShape(stages, burstHomes, liveStreams)
}

// FixtureTargets is the exact front-door allowlist for the host driver's
// verified transport. Callers still explicitly supply each loopback dial port.
func (f PreparedFixture) FixtureTargets() ([]string, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	var hosts []string
	for _, raw := range []string{f.Origins[PublicSurface], f.Origins[AdminSurface], f.Issuer} {
		target, err := url.Parse(raw)
		if err != nil || target.Port() == "" {
			return nil, fmt.Errorf(
				"%w: explicit TLS port required on each fixture origin",
				errCohort,
			)
		}
		hosts = append(hosts, target.Host)
	}
	return hosts, nil
}

func (f PreparedFixture) eventScope() (map[uuid.UUID]string, map[string]int, error) {
	events := make(map[uuid.UUID]string, len(f.Events))
	stages := make(map[string]int)
	for _, event := range f.Events {
		if event.ID == uuid.Nil || events[event.ID] != "" {
			return nil, nil, fmt.Errorf("%w: distinct native target events required", errCohort)
		}
		switch event.Stage {
		case "registration", "preparation", "live", "archived":
		default:
			return nil, nil, fmt.Errorf(
				"%w: explicit supported native event stage required",
				errCohort,
			)
		}
		events[event.ID] = event.Stage
		stages[event.Stage]++
	}
	return events, stages, nil
}

func (f PreparedFixture) validateJourneys(
	events map[uuid.UUID]string,
) (map[string]CohortMember, error) {
	subjects := make(map[string]CohortMember, len(f.Cohort.Members))
	for _, member := range f.Cohort.Members {
		journey, exists := f.Journeys[member.Subject]
		if !exists || events[member.Event] == "" || journey.Read.Event != member.Event {
			return nil, fmt.Errorf(
				"%w: each declared actor must retain its own prepared home event",
				errCohort,
			)
		}
		subjects[member.Subject] = member
	}
	bindings := make(map[string]ReadBinding, len(f.Journeys))
	for subject, journey := range f.Journeys {
		bindings[subject] = journey.Read
	}
	if _, err := NewNativeReads(bindings); err != nil {
		return nil, err
	}
	return subjects, nil
}

func (f PreparedFixture) validateBurst(
	events map[uuid.UUID]string,
	subjects map[string]CohortMember,
) (map[uuid.UUID]int, error) {
	if f.Burst.Config.Profile != f.Cohort.Profile ||
		events[f.Burst.Config.Event] != "registration" ||
		f.Burst.Config.Form == uuid.Nil ||
		f.Burst.Config.FormVersion < 1 ||
		len(f.Burst.Applications) == 0 ||
		len(f.Burst.Applications) > 200 {
		return nil, fmt.Errorf(
			"%w: burst requires actual published registration source and declared cohort actors",
			errCohort,
		)
	}
	applicationIDs := make(map[uuid.UUID]bool)
	burstHomes := make(map[uuid.UUID]int)
	for subject, application := range f.Burst.Applications {
		member, exists := subjects[subject]
		if !exists || member.Staff || member.Event == f.Burst.Config.Event ||
			application.ID == uuid.Nil ||
			applicationIDs[application.ID] ||
			application.FormID != f.Burst.Config.Form ||
			application.FormVersion != f.Burst.Config.FormVersion {
			return nil, fmt.Errorf(
				"%w: burst must use distinct existing cross-event competitors and frozen form",
				errCohort,
			)
		}
		applicationIDs[application.ID] = true
		burstHomes[member.Event]++
	}
	return burstHomes, nil
}

func (f PreparedFixture) validateDisplays(events map[uuid.UUID]string) (map[uuid.UUID]int, error) {
	streamIDs := make(map[string]bool)
	liveStreams := make(map[uuid.UUID]int)
	for _, display := range f.Displays {
		if display.ID == "" || streamIDs[display.ID] || events[display.Event] != "live" ||
			display.Token == "" {
			return nil, fmt.Errorf(
				"%w: distinct actual paired displays on live events required",
				errCohort,
			)
		}
		streamIDs[display.ID] = true
		liveStreams[display.Event]++
	}
	return liveStreams, nil
}

func (f PreparedFixture) validateReleaseShape(
	stages map[string]int,
	burstHomes map[uuid.UUID]int,
	liveStreams map[uuid.UUID]int,
) error {
	if f.Cohort.Profile == "release" {
		stagesMatch := stages["registration"] == 1 && stages["preparation"] == 1 &&
			stages["live"] == 2 && stages["archived"] == 1
		burstMatches := len(f.Burst.Applications) == 200 && len(burstHomes) == 4
		backgroundMatches := len(f.Displays) >= 5 && len(liveStreams) == 2 && len(f.Due) > 0
		if !stagesMatch || !burstMatches || !backgroundMatches {
			return fmt.Errorf(
				"%w: release requires full five-stage mix, 200 cross-event applications, live displays and due native work",
				errCohort,
			)
		}
		for _, count := range burstHomes {
			if count != 50 {
				return fmt.Errorf(
					"%w: release burst requires fifty existing competitors from each other home event",
					errCohort,
				)
			}
		}
	}
	return nil
}
