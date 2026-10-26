package loadprofile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/observations"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
)

var errNativeWrite = errors.New("native load write invariant failed")

type NativeWrites struct {
	Attempts *AttemptLedger
	Metrics  *Metrics
}

func payloadFingerprint(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode private native write fingerprint: %w", err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func originalConflict(response Response) bool {
	if response.Status != http.StatusConflict {
		return false
	}
	var envelope contract.ErrorEnvelope
	return decodeNative(response.Body, &envelope) == nil &&
		envelope.Error.Code == "revision_conflict" &&
		envelope.Error.RequestId != ""
}

func (w NativeWrites) finish(
	id uuid.UUID,
	response Response,
	transport error,
	acknowledged bool,
	allowConflict bool,
) (Outcome, error) {
	if allowConflict && transport == nil && originalConflict(response) {
		if err := w.Attempts.Reject(id, response.Status); err != nil {
			return Unexpected, err
		}
		return ExpectedConflict, nil
	}
	if err := w.Attempts.Finish(id, response.Status, transport == nil && acknowledged); err != nil {
		return Unexpected, err
	}
	if transport != nil {
		return Unexpected, transport
	}
	if !acknowledged {
		return Unexpected, fmt.Errorf(
			"%w: original response did not acknowledge the exact native effect",
			errNativeWrite,
		)
	}
	return Accepted, nil
}

// Apply is one normal native attendee registration submission. A waitlisted response is
// an ACCEPTED registration, not a dropped capacity error. It must be reconciled
// against its committed row/hash just like a confirmed response.
func (w NativeWrites) Apply(
	ctx context.Context,
	actor Actor,
	event uuid.UUID,
	input registration.Application,
) (Measurement, *AcceptedWrite, error) {
	measurement := nativeSample(actor, RegistrationWrite, Response{})
	fingerprint, err := payloadFingerprint(input)
	if err != nil {
		return measurement, nil, err
	}
	spec := WriteSpec{
		Kind:        RegistrationEffect,
		Event:       event,
		Actor:       actor.Session.Person(),
		Resource:    input.ID,
		Fingerprint: fingerprint,
	}
	id, err := w.Attempts.Begin(spec)
	if err != nil {
		return measurement, nil, err
	}
	response, transport := actor.Session.Exchange(
		ctx,
		http.MethodPost,
		"/api/v1/events/"+event.String()+"/registrations",
		nil,
		input,
	)
	measurement.Duration, measurement.Status = response.Duration, response.Status
	var view registration.View
	ack := transport == nil && response.Status == http.StatusOK &&
		decodeNative(response.Body, &view) == nil &&
		view.ID == input.ID &&
		view.EventID == event &&
		view.PersonID == actor.Session.Person() &&
		view.FormID == input.FormID &&
		view.FormVersion == input.FormVersion &&
		view.Kind == "attendee" && view.Audience == "attendee" &&
		view.TeamPreference == input.TeamPreference &&
		view.InvitationReference == input.InvitationReference &&
		view.Sequence > 0 &&
		view.Revision >= 1 &&
		(view.State == "confirmed" || view.State == "waitlisted")
	measurement.Outcome, err = w.finish(id, response, transport, ack, false)
	if err != nil || !ack {
		return measurement, nil, err
	}
	return measurement, &AcceptedWrite{Identity: spec.identity(), Fingerprint: fingerprint}, nil
}

// Mentor captures a real current read before proposing a new version. A
// conflict is observed, never replayed; the next scheduled action must read
// current native authority again and author a different observation payload.
func (w NativeWrites) Mentor(
	ctx context.Context,
	actor Actor,
	event, team, observationID uuid.UUID,
	evidence observations.Evidence,
) (Measurement, *AcceptedWrite, error) {
	path := "/api/v1/events/" + event.String() + "/teams/" + team.String() + "/mentor-observations"
	base, err := actor.Session.Exchange(ctx, http.MethodGet, path+"?limit=100", nil, nil)
	read := nativeSample(actor, MentorRead, base)
	var page observations.Page
	if err != nil {
		return read, nil, err
	}
	if base.Status != http.StatusOK || decodeNative(base.Body, &page) != nil || !page.CanRecord {
		return read, nil, fmt.Errorf("%w: fresh mentor authority unavailable", errNativeWrite)
	}
	read.Outcome = Accepted
	if err := w.Metrics.Observe(read); err != nil {
		return read, nil, err
	}
	var version int64
	for _, observation := range page.Observations {
		if observation.ID == observationID {
			if observation.AuthorID != actor.Session.Person() || observation.TeamID != team {
				return Measurement{}, nil, fmt.Errorf(
					"%w: current observation belongs to another author/team",
					errNativeWrite,
				)
			}
			version = observation.Version
		}
	}
	input := observations.Command{
		Action:   "observation",
		ID:       observationID,
		Version:  version,
		Evidence: evidence,
	}
	fingerprint, err := payloadFingerprint(evidence)
	measurement := nativeSample(actor, MentorWrite, Response{})
	if err != nil {
		return measurement, nil, err
	}
	spec := WriteSpec{
		Kind:        MentorEffect,
		Event:       event,
		Actor:       actor.Session.Person(),
		Team:        team,
		Resource:    observationID,
		Version:     version + 1,
		Fingerprint: fingerprint,
	}
	id, err := w.Attempts.Begin(spec)
	if err != nil {
		return measurement, nil, err
	}
	response, transport := actor.Session.Exchange(ctx, http.MethodPost, path, &page.Revision, input)
	measurement.Duration, measurement.Status = response.Duration, response.Status
	var result observations.Page
	ack := false
	if transport == nil && response.Status == http.StatusOK &&
		decodeNative(response.Body, &result) == nil {
		for _, observed := range result.Observations {
			if observed.ID == observationID && observed.Version == version+1 &&
				observed.AuthorID == actor.Session.Person() &&
				observed.TeamID == team &&
				observed.Evidence == evidence {
				ack = true
			}
		}
	}
	measurement.Outcome, err = w.finish(id, response, transport, ack, true)
	if err != nil || !ack {
		return measurement, nil, err
	}
	return measurement, &AcceptedWrite{Identity: spec.identity(), Fingerprint: fingerprint}, nil
}

// HelpClaim submits exactly one contending native command against the captured
// request revision. Losers keep their real409 samples and are independently
// checked against native commands/transitions; they are never automatically
// retried against the winner's newer request.
func (w NativeWrites) HelpClaim(
	ctx context.Context,
	actor Actor,
	event, request uuid.UUID,
	revision int64,
) (Measurement, *AcceptedWrite, error) {
	input := help.Command{Action: "claim", CommandID: uuid.New()}
	fingerprint, err := payloadFingerprint(input)
	measurement := nativeSample(actor, HelpClaim, Response{})
	if err != nil {
		return measurement, nil, err
	}
	spec := WriteSpec{
		Kind:        HelpEffect,
		Action:      "claim",
		Event:       event,
		Actor:       actor.Session.Person(),
		Resource:    request,
		Command:     input.CommandID,
		Version:     revision + 1,
		Fingerprint: fingerprint,
	}
	id, err := w.Attempts.Begin(spec)
	if err != nil {
		return measurement, nil, err
	}
	wire := contract.HelpCommand{Action: contract.HelpCommandActionClaim, AssigneeId: nil}
	response, transport := actor.Session.Command(
		ctx,
		http.MethodPost,
		"/api/v1/events/"+event.String()+"/help/"+request.String()+"/actions",
		&revision,
		wire,
		input.CommandID,
	)
	measurement.Duration, measurement.Status = response.Duration, response.Status
	var page help.Page
	ack := transport == nil && response.Status == http.StatusOK &&
		decodeNative(response.Body, &page) == nil &&
		page.Request.ID == request &&
		page.Request.EventID == event &&
		page.Request.Revision == revision+1 &&
		page.Request.Status == "claimed" &&
		page.Request.AssigneeID != nil &&
		*page.Request.AssigneeID == actor.Session.Person()
	measurement.Outcome, err = w.finish(id, response, transport, ack, true)
	if err != nil || !ack {
		return measurement, nil, err
	}
	return measurement, &AcceptedWrite{Identity: spec.identity(), Fingerprint: fingerprint}, nil
}
