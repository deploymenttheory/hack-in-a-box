package loadprofile

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

// HelpCreate starts a new native request through the ordinary team-member
// boundary. Creation is a measured, reconciled write, never invisible setup
// traffic inside the runtime claim workload.
func (w NativeWrites) HelpCreate(
	ctx context.Context,
	actor Actor,
	event, request uuid.UUID,
	input help.Input,
) (Measurement, *AcceptedWrite, error) {
	input.CommandID = uuid.New()
	wire := contract.HelpInput{
		TeamId: input.TeamID, Category: contract.HelpInputCategory(input.Category),
		Description: input.Description, Location: input.Location, MeetingUrl: input.MeetingURL,
	}
	return w.helpWrite(ctx, actor, event, request, 0, "create", input.CommandID,
		input, wire, http.MethodPut, "", func(r help.Request) bool {
			return r.TeamID == input.TeamID && r.RequesterID == actor.Session.Person() &&
				r.Category == input.Category && r.Description == input.Description &&
				r.Location == input.Location && r.MeetingURL == input.MeetingURL &&
				r.Status == "waiting" && r.AssigneeID == nil
		})
}

// HelpResolve performs one deliberate resolution against the captured claimed
// version. A conflict remains a sample; this method never reads a newer version
// and retries the command against it.
func (w NativeWrites) HelpResolve(
	ctx context.Context,
	actor Actor,
	event, request uuid.UUID,
	revision int64,
) (Measurement, *AcceptedWrite, error) {
	input := help.Command{Action: "resolve", CommandID: uuid.New()}
	wire := contract.HelpCommand{Action: contract.HelpCommandActionResolve}
	return w.helpWrite(ctx, actor, event, request, revision, "resolve", input.CommandID,
		input, wire, http.MethodPost, "/actions", func(r help.Request) bool {
			return r.Status == "resolved" && r.ResolvedAt != nil &&
				r.AssigneeID != nil && *r.AssigneeID == actor.Session.Person()
		})
}

func (w NativeWrites) helpWrite(
	ctx context.Context,
	actor Actor,
	event, request uuid.UUID,
	revision int64,
	action string,
	command uuid.UUID,
	domain, wire any,
	method, suffix string,
	exact func(help.Request) bool,
) (Measurement, *AcceptedWrite, error) {
	measurement := nativeSample(actor, HelpWrite, Response{})
	fingerprint, err := payloadFingerprint(domain)
	if err != nil {
		return measurement, nil, err
	}
	spec := WriteSpec{
		Kind:        HelpEffect,
		Action:      action,
		Event:       event,
		Actor:       actor.Session.Person(),
		Resource:    request,
		Command:     command,
		Version:     revision + 1,
		Fingerprint: fingerprint,
	}
	id, err := w.Attempts.Begin(spec)
	if err != nil {
		return measurement, nil, err
	}
	response, transport := actor.Session.Command(ctx, method,
		"/api/v1/events/"+event.String()+"/help/"+request.String()+suffix,
		&revision, wire, command)
	measurement.Duration, measurement.Status = response.Duration, response.Status
	var page help.Page
	ack := transport == nil && response.Status == http.StatusOK &&
		decodeNative(response.Body, &page) == nil && page.Request.ID == request &&
		page.Request.EventID == event && page.Request.Revision == revision+1 && exact(page.Request)
	measurement.Outcome, err = w.finish(id, response, transport, ack, action == "resolve")
	if err != nil || !ack {
		return measurement, nil, err
	}
	return measurement, &AcceptedWrite{Identity: spec.identity(), Fingerprint: fingerprint}, nil
}
