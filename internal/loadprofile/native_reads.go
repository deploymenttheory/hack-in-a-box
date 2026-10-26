package loadprofile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/deploymenttheory/hack-in-a-box/internal/observations"
	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
)

var errNativeRead = errors.New("native load reader invariant failed")

// SearchVisibility comes from the coherent fixture's ACTUAL reviewed native
// publications and current role/audience grants. Never derive an allowlist from
// the response currently being tested. These private IDs/prose are not reports.
type SearchVisibility struct {
	ProjectEvent    uuid.UUID            `json:"project_event"`
	ProjectStatus   string               `json:"project_status"`
	Answers         map[uuid.UUID]int64  `json:"answers"`
	Projects        map[uuid.UUID]int64  `json:"projects"`
	Events          map[uuid.UUID]string `json:"events"`
	AllowedTeams    map[uuid.UUID]bool   `json:"allowed_teams"`
	Forbidden       []string             `json:"forbidden"`
	Needle          string               `json:"needle"`
	MinimumAnswers  int                  `json:"minimum_answers"`
	MinimumProjects int                  `json:"minimum_projects"`
}

// ReadBinding selects only journeys valid at this target event's real stage.
// Negative/revocation probes are separate deliberate cases, not a blanket
// classification that hides unexpected 403/409/429 from normal traffic.
type ReadBinding struct {
	Event      uuid.UUID        `json:"event"`
	Team       uuid.UUID        `json:"team"`
	Operations []Operation      `json:"operations"`
	Visibility SearchVisibility `json:"visibility"`
}

type NativeReads struct{ bindings map[string]ReadBinding }

func NewNativeReads(bindings map[string]ReadBinding) (*NativeReads, error) {
	if len(bindings) == 0 {
		return nil, fmt.Errorf("%w: actor bindings required", errNativeRead)
	}
	owned := make(map[string]ReadBinding, len(bindings))
	for actor, binding := range bindings {
		if actor == "" || binding.Event == uuid.Nil || len(binding.Operations) == 0 {
			return nil, fmt.Errorf("%w: explicit actor/event journeys required", errNativeRead)
		}
		for _, operation := range binding.Operations {
			switch operation {
			case SessionRead,
				RegistrationRead,
				HelpRead,
				MentorRead,
				ClarificationSearch,
				ShowcaseSearch:
			default:
				return nil, fmt.Errorf("%w: operation is not a native read", errNativeRead)
			}
		}
		binding.Operations = slices.Clone(binding.Operations)
		binding.Visibility.Answers = maps.Clone(binding.Visibility.Answers)
		binding.Visibility.Projects = maps.Clone(binding.Visibility.Projects)
		binding.Visibility.Events = maps.Clone(binding.Visibility.Events)
		binding.Visibility.AllowedTeams = maps.Clone(binding.Visibility.AllowedTeams)
		binding.Visibility.Forbidden = slices.Clone(binding.Visibility.Forbidden)
		owned[actor] = binding
	}
	return &NativeReads{bindings: owned}, nil
}

func (n *NativeReads) Action(
	ctx context.Context,
	actor Actor,
	sequence uint64,
) (Measurement, *AcceptedWrite, error) {
	binding, exists := n.bindings[actor.ID]
	if !exists {
		return Measurement{}, nil, fmt.Errorf("%w: unknown cohort actor", errNativeRead)
	}
	operation := binding.Operations[sequence%uint64(len(binding.Operations))]
	return n.Read(ctx, actor, binding, operation)
}

func (n *NativeReads) Read(
	ctx context.Context,
	actor Actor,
	binding ReadBinding,
	operation Operation,
) (Measurement, *AcceptedWrite, error) {
	path, err := nativeReadPath(binding, operation)
	if err != nil {
		return nativeSample(actor, operation, Response{}), nil, err
	}
	response, err := actor.Session.Exchange(ctx, http.MethodGet, path, nil, nil)
	measurement := nativeSample(actor, operation, response)
	if err != nil {
		return measurement, nil, err
	}
	if response.Status != http.StatusOK {
		return measurement, nil, fmt.Errorf(
			"%w: routine read returned status %d",
			errNativeRead,
			response.Status,
		)
	}
	if err := validateNativeRead(
		response.Body,
		actor.Session.Person(),
		binding,
		operation,
	); err != nil {
		return measurement, nil, err
	}
	measurement.Outcome = Accepted
	return measurement, nil, nil
}

func nativeReadPath(binding ReadBinding, operation Operation) (string, error) {
	base := "/api/v1/events/" + binding.Event.String()
	switch operation {
	case SessionRead:
		return "/api/v1/session", nil
	case RegistrationRead:
		return base + "/registrations/mine", nil
	case HelpRead:
		return base + "/help", nil
	case MentorRead:
		if binding.Team == uuid.Nil {
			return "", fmt.Errorf("%w: scoped mentor team required", errNativeRead)
		}
		return base + "/teams/" + binding.Team.String() + "/mentor-observations?limit=5", nil
	case ClarificationSearch:
		if binding.Visibility.Needle == "" || binding.Visibility.MinimumAnswers < 1 ||
			binding.Visibility.MinimumAnswers > 5 ||
			len(binding.Visibility.Answers) < binding.Visibility.MinimumAnswers {
			return "", fmt.Errorf(
				"%w: real nonempty published answer cohort required",
				errNativeRead,
			)
		}
		return base + "/clarifications?limit=5&search=" + url.QueryEscape(
			binding.Visibility.Needle,
		), nil
	case ShowcaseSearch:
		if binding.Visibility.Needle == "" || binding.Visibility.MinimumProjects < 1 ||
			binding.Visibility.MinimumProjects > 5 ||
			len(binding.Visibility.Projects) < binding.Visibility.MinimumProjects {
			return "", fmt.Errorf(
				"%w: real nonempty reviewed project cohort required",
				errNativeRead,
			)
		}
		query := url.Values{"limit": {"5"}, "search": {binding.Visibility.Needle}}
		if binding.Visibility.ProjectEvent != uuid.Nil {
			query.Set("event_id", binding.Visibility.ProjectEvent.String())
		}
		if binding.Visibility.ProjectStatus != "" {
			query.Set("status", binding.Visibility.ProjectStatus)
		}
		return "/api/v1/projects?" + query.Encode(), nil
	default:
		return "", fmt.Errorf("%w: unsupported routine operation", errNativeRead)
	}
}

func decodeNative(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil {
		return fmt.Errorf("%w: native positive projection shape invalid", errNativeRead)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("%w: trailing native projection data", errNativeRead)
	}
	return nil
}

func validateNativeRead(
	raw []byte,
	person uuid.UUID,
	binding ReadBinding,
	operation Operation,
) error {
	switch operation {
	case SessionRead:
		var session struct {
			Person struct {
				ID uuid.UUID `json:"id"`
			} `json:"person"`
			CSRF string `json:"csrf_token"`
		}
		// Session's canonical person projection contains additional public identity
		// fields; verify identity without copying its credentials into evidence.
		if json.Unmarshal(raw, &session) != nil || session.Person.ID != person ||
			len(session.CSRF) < 20 {
			return fmt.Errorf("%w: current native person/session mismatch", errNativeRead)
		}
	case RegistrationRead:
		var page struct {
			Items []registration.View `json:"items"`
		}
		if err := decodeNative(raw, &page); err != nil {
			return err
		}
		if len(page.Items) == 0 {
			return fmt.Errorf("%w: actual registered cohort missing", errNativeRead)
		}
		for _, item := range page.Items {
			if item.EventID != binding.Event || item.PersonID != person {
				return fmt.Errorf("%w: another actor/event registration disclosed", errNativeRead)
			}
		}
	case HelpRead:
		var page help.Queue
		if err := decodeNative(raw, &page); err != nil {
			return err
		}
		for _, item := range page.Items {
			if !binding.Visibility.AllowedTeams[item.TeamID] {
				return fmt.Errorf(
					"%w: help queue disclosed another permission scope",
					errNativeRead,
				)
			}
		}
	case MentorRead:
		var page observations.Page
		if err := decodeNative(raw, &page); err != nil {
			return err
		}
		for _, item := range page.Observations {
			if item.TeamID != binding.Team {
				return fmt.Errorf("%w: mentor reader disclosed another team", errNativeRead)
			}
		}
	case ClarificationSearch, ShowcaseSearch:
		return validatePublicSearch(raw, binding, operation)
	default:
		return fmt.Errorf("%w: unsupported current reader", errNativeRead)
	}
	return nil
}

func validatePublicSearch(raw []byte, binding ReadBinding, operation Operation) error {
	policy := binding.Visibility
	for _, canary := range policy.Forbidden {
		if canary != "" && bytes.Contains(raw, []byte(canary)) {
			return fmt.Errorf("%w: private canary disclosed by common search", errNativeRead)
		}
	}
	if operation == ClarificationSearch {
		var page clarification.PublicList
		if err := decodeNative(raw, &page); err != nil {
			return err
		}
		if len(page.Items) < policy.MinimumAnswers {
			return fmt.Errorf(
				"%w: answer permission filtering/pagination lost known matches",
				errNativeRead,
			)
		}
		seen := map[uuid.UUID]bool{}
		for _, answer := range page.Items {
			version, allowed := policy.Answers[answer.ID]
			if !allowed || version != answer.Version || seen[answer.ID] || !answer.Current ||
				!answer.SourceCurrent {
				return fmt.Errorf(
					"%w: answer source/version outside current approved allowlist",
					errNativeRead,
				)
			}
			seen[answer.ID] = true
		}
		return nil
	}
	var page outcomes.ProjectPage
	if err := decodeNative(raw, &page); err != nil {
		return err
	}
	if len(page.Items) < policy.MinimumProjects {
		return fmt.Errorf(
			"%w: project permission filtering/pagination lost known matches",
			errNativeRead,
		)
	}
	seen := map[uuid.UUID]bool{}
	for _, project := range page.Items {
		version, allowed := policy.Projects[project.ID]
		if !allowed || project.Version != version || seen[project.ID] {
			return fmt.Errorf(
				"%w: project source/version outside current reviewed allowlist",
				errNativeRead,
			)
		}
		seen[project.ID] = true
	}
	for _, event := range page.Events {
		name, allowed := policy.Events[event.ID]
		if !allowed || name != event.Name {
			return fmt.Errorf("%w: project search disclosed hidden event metadata", errNativeRead)
		}
	}
	return nil
}
