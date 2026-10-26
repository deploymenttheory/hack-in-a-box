package loadprofile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
)

// PreparationCall is PRIVATE setup provenance. It retains every original
// response/uncertainty, without provider credentials, cookies or native prose.
// These setup requests never enter the sustained latency distributions.
type PreparationCall struct {
	Actor            uuid.UUID     `json:"actor"`
	Method           string        `json:"method"`
	Path             string        `json:"path"`
	InputSHA256      string        `json:"input_sha256"`
	ResponseSHA256   string        `json:"response_sha256"`
	Status           int           `json:"status"`
	OriginalComplete bool          `json:"original_complete"`
	Duration         time.Duration `json:"duration_ns"`
}

type NativePreparer struct {
	Calls []PreparationCall `json:"calls"`
}

// prepareCall performs exactly one normal native request. A missing response
// aborts setup, never authorizes a retry or a current-read-derived ACK.
func prepareCall[T any](ctx context.Context, preparer *NativePreparer, actor *Session,
	method, path string, revision *int64, input any,
) (T, error) {
	var value T
	if preparer == nil || actor == nil {
		return value, fmt.Errorf("%w: explicit native setup owner required", errCohort)
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return value, fmt.Errorf("encode private setup input: %w", err)
	}
	inputHash := sha256.Sum256(raw)
	response, err := actor.Exchange(ctx, method, path, revision, input)
	responseHash := sha256.Sum256(response.Body)
	preparer.Calls = append(preparer.Calls, PreparationCall{
		Actor:  actor.Person(),
		Method: method,
		Path:   path,
		InputSHA256: hex.EncodeToString(
			inputHash[:],
		),
		ResponseSHA256:   hex.EncodeToString(responseHash[:]),
		Status:           response.Status,
		OriginalComplete: err == nil,
		Duration:         response.Duration,
	})
	if err != nil {
		return value, err
	}
	if response.Status != http.StatusOK {
		return value, fmt.Errorf(
			"%w: native setup %s returned status %d",
			errCohort,
			method,
			response.Status,
		)
	}
	if err := decodeNative(response.Body, &value); err != nil {
		return value, err
	}
	return value, nil
}

func (p *NativePreparer) CreateEvent(
	ctx context.Context,
	bootstrap *Session,
	input event.Input,
) (event.View, error) {
	created, err := prepareCall[event.View](
		ctx,
		p,
		bootstrap,
		http.MethodPost,
		"/api/v1/events",
		nil,
		input,
	)
	if err == nil && (created.ID != input.ID || created.OwnerID == nil ||
		*created.OwnerID != bootstrap.Person() || created.Stage != "draft" ||
		created.Configuration.Capacity != input.Configuration.Capacity) {
		err = fmt.Errorf("%w: native creation ACK lost exact owner/event/capacity", errCohort)
	}
	return created, err
}

func (p *NativePreparer) Transition(
	ctx context.Context,
	owner *Session,
	id uuid.UUID,
	stage string,
) (event.View, error) {
	path := "/api/v1/events/" + id.String()
	current, err := prepareCall[event.View](ctx, p, owner, http.MethodGet, path, nil, nil)
	if err != nil {
		return current, err
	}
	next, err := prepareCall[event.View](
		ctx,
		p,
		owner,
		http.MethodPost,
		path+"/transition",
		&current.Revision,
		map[string]any{
			"stage":                stage,
			"reason":               "Synthetic release fixture lifecycle",
			"attendance_exception": false,
		},
	)
	if err == nil && (next.ID != id || next.Stage != stage || next.Revision <= current.Revision) {
		err = fmt.Errorf("%w: native transition ACK lost exact stage or revision", errCohort)
	}
	return next, err
}

func (p *NativePreparer) PublishForm(
	ctx context.Context,
	owner *Session,
	id uuid.UUID,
	audience string,
) (registration.FormView, error) {
	definition := forms.Definition{
		ID:       uuid.New(),
		Name:     "Synthetic load " + audience + " application",
		Audience: audience,
		Fields: []forms.Field{
			{
				ID:       uuid.New(),
				Label:    "Synthetic motivation",
				Type:     "text",
				Required: true,
				Choices:  []string{},
			},
		},
	}
	path := "/api/v1/events/" + id.String() + "/registration/forms/" + definition.ID.String()
	zero := int64(0)
	saved, err := prepareCall[registration.FormView](
		ctx,
		p,
		owner,
		http.MethodPut,
		path,
		&zero,
		definition,
	)
	if err != nil {
		return saved, err
	}
	published, err := prepareCall[registration.FormView](
		ctx,
		p,
		owner,
		http.MethodPost,
		path+"/publish",
		&saved.Revision,
		map[string]any{"version": saved.Version, "acknowledge_response_versions": true},
	)
	if err == nil &&
		(published.Definition.ID != definition.ID || published.Definition.Audience != audience ||
			published.PublishedVersion == nil || *published.PublishedVersion != saved.Version) {
		err = fmt.Errorf("%w: exact native form publication ACK missing", errCohort)
	}
	return published, err
}

func applicationForForm(form registration.FormView) (registration.Application, error) {
	if form.PublishedVersion == nil {
		return registration.Application{}, fmt.Errorf(
			"%w: actual published form version required",
			errCohort,
		)
	}
	answers := forms.Answers{}
	for _, field := range form.Definition.Fields {
		answers[field.ID.String()] = json.RawMessage(`"Synthetic release capacity motivation"`)
	}
	return registration.Application{
		ID: uuid.New(), FormID: form.Definition.ID, FormVersion: *form.PublishedVersion,
		Answers: answers, TeamPreference: "solo",
	}, nil
}

func (p *NativePreparer) ApplyForm(
	ctx context.Context,
	actor *Session,
	id uuid.UUID,
	form registration.FormView,
) (registration.View, error) {
	input, err := applicationForForm(form)
	if err != nil {
		return registration.View{}, err
	}
	view, err := prepareCall[registration.View](ctx, p, actor, http.MethodPost,
		"/api/v1/events/"+id.String()+"/registrations", nil, input)
	if err == nil &&
		(view.ID != input.ID || view.EventID != id || view.PersonID != actor.Person() ||
			view.FormID != form.Definition.ID || view.FormVersion != input.FormVersion || view.Audience != form.Definition.Audience) {
		err = fmt.Errorf("%w: exact original registration ACK missing", errCohort)
	}
	return view, err
}

func (p *NativePreparer) SelectStaff(
	ctx context.Context,
	owner, applicant *Session,
	id uuid.UUID,
	form registration.FormView,
) (registration.View, error) {
	applied, err := p.ApplyForm(ctx, applicant, id, form)
	if err != nil {
		return applied, err
	}
	if applied.State != "received" || applied.Kind != "facilitator" {
		return applied, fmt.Errorf(
			"%w: actual received facilitator application required",
			errCohort,
		)
	}
	selected, err := prepareCall[registration.View](
		ctx,
		p,
		owner,
		http.MethodPost,
		"/api/v1/events/"+id.String()+"/registrations/"+applied.ID.String()+"/decision",
		&applied.Revision,
		registration.ReviewInput{
			Action:      "selected",
			Reason:      "Synthetic release fixture role selection",
			Acknowledge: true,
		},
	)
	if err == nil &&
		(selected.ID != applied.ID || selected.PersonID != applicant.Person() || selected.State != "selected" ||
			selected.Kind != "facilitator" || selected.Audience != form.Definition.Audience || selected.Revision <= applied.Revision) {
		err = fmt.Errorf("%w: exact original staff selection ACK missing", errCohort)
	}
	return selected, err
}
