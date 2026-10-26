package loadprofile

import (
	"context"
	"fmt"
	"maps"
	"net/http"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/deploymenttheory/hack-in-a-box/internal/observations"
)

// Journey selects native role-specific behavior prepared by the fixture. It
// confers no permission: every request obtains current native authority. Only
// one designated competitor per team should request help in the declared mix.
type Journey struct {
	Read          ReadBinding
	Observation   uuid.UUID
	RequestHelp   bool
	WorkHelpQueue bool
}

type NativeWorkload struct {
	reads    *NativeReads
	writes   NativeWrites
	journeys map[string]Journey
	run      uuid.UUID
}

func NewNativeWorkload(
	journeys map[string]Journey,
	writes NativeWrites,
	run uuid.UUID,
) (*NativeWorkload, error) {
	if run == uuid.Nil || writes.Attempts == nil || writes.Metrics == nil {
		return nil, fmt.Errorf(
			"%w: run identity and complete write accounting required",
			errNativeWrite,
		)
	}
	bindings := make(map[string]ReadBinding, len(journeys))
	for actor, journey := range journeys {
		if (journey.Observation != uuid.Nil || journey.RequestHelp) &&
			journey.Read.Team == uuid.Nil {
			return nil, fmt.Errorf("%w: role write needs a prepared native team", errNativeWrite)
		}
		bindings[actor] = journey.Read
	}
	reads, err := NewNativeReads(bindings)
	if err != nil {
		return nil, err
	}
	owned := maps.Clone(journeys)
	for actor, journey := range owned {
		journey.Read = reads.bindings[actor]
		owned[actor] = journey
	}
	return &NativeWorkload{reads: reads, writes: writes, journeys: owned, run: run}, nil
}

// Action runs within the actor's sequential stream. An unknown original write
// blocks all further writes for that person; ordinary authorised reads may
// continue, but their success cannot resolve that original uncertainty.
func (w *NativeWorkload) Action(
	ctx context.Context,
	actor Actor,
	sequence uint64,
) (Measurement, *AcceptedWrite, error) {
	journey, exists := w.journeys[actor.ID]
	if !exists {
		return Measurement{}, nil, fmt.Errorf("%w: undeclared native journey", errNativeRead)
	}
	if !w.writes.Attempts.ActorBlocked(actor.Session.Person()) {
		if journey.Observation != uuid.Nil && sequence%3 == 2 {
			return w.writes.Mentor(ctx, actor, journey.Read.Event, journey.Read.Team,
				journey.Observation, observations.Evidence{
					Category: "progress",
					Evidence: fmt.Sprintf("Load run %s observation %d", w.run, sequence),
					Outcome:  "Synthetic capacity evidence; not a competition score",
				})
		}
		if journey.RequestHelp && sequence%30 == 7 {
			return w.writes.HelpCreate(ctx, actor, journey.Read.Event, uuid.New(), help.Input{
				TeamID: journey.Read.Team, Category: "setup",
				Description: fmt.Sprintf("Load run %s help request %d", w.run, sequence),
				Location:    "Capacity fixture desk",
			})
		}
		if journey.WorkHelpQueue && sequence%2 == 1 {
			return w.helpQueue(ctx, actor, journey.Read)
		}
	}
	return w.reads.Action(ctx, actor, sequence)
}

func (w *NativeWorkload) helpQueue(
	ctx context.Context,
	actor Actor,
	binding ReadBinding,
) (Measurement, *AcceptedWrite, error) {
	path := "/api/v1/events/" + binding.Event.String() + "/help"
	response, err := actor.Session.Exchange(ctx, http.MethodGet, path, nil, nil)
	measurement := nativeSample(actor, HelpRead, response)
	if err != nil {
		return measurement, nil, err
	}
	if response.Status != http.StatusOK ||
		validateNativeRead(response.Body, actor.Session.Person(), binding, HelpRead) != nil {
		return measurement, nil, fmt.Errorf("%w: current helper queue unavailable", errNativeRead)
	}
	var queue help.Queue
	if err := decodeNative(response.Body, &queue); err != nil {
		return measurement, nil, err
	}
	measurement.Outcome = Accepted
	// Complete owned work before claiming the oldest waiting request. The queue
	// is not an authority/expected-version substitute; read its native page next.
	var request uuid.UUID
	for _, item := range queue.Items {
		if item.Status == "claimed" && item.AssigneeID != nil &&
			*item.AssigneeID == actor.Session.Person() {
			request = item.ID
			break
		}
		if item.Status == "waiting" && request == uuid.Nil {
			request = item.ID
		}
	}
	if request == uuid.Nil {
		return measurement, nil, nil
	}
	if err := w.writes.Metrics.Observe(measurement); err != nil {
		return measurement, nil, err
	}
	response, err = actor.Session.Exchange(ctx, http.MethodGet, path+"/"+request.String(), nil, nil)
	measurement = nativeSample(actor, HelpRead, response)
	var page help.Page
	if err != nil {
		return measurement, nil, err
	}
	if response.Status != http.StatusOK || decodeNative(response.Body, &page) != nil ||
		page.Request.ID != request || page.Request.EventID != binding.Event ||
		!binding.Visibility.AllowedTeams[page.Request.TeamID] {
		return measurement, nil, fmt.Errorf("%w: current help request scope changed", errNativeRead)
	}
	measurement.Outcome = Accepted
	resolveOwn := page.CanResolve && page.Request.AssigneeID != nil &&
		*page.Request.AssigneeID == actor.Session.Person()
	if !page.CanClaim && !resolveOwn {
		return measurement, nil, nil
	}
	if err := w.writes.Metrics.Observe(measurement); err != nil {
		return measurement, nil, err
	}
	if page.CanResolve && page.Request.AssigneeID != nil &&
		*page.Request.AssigneeID == actor.Session.Person() {
		return w.writes.HelpResolve(ctx, actor, binding.Event, request, page.Request.Revision)
	}
	return w.writes.HelpClaim(ctx, actor, binding.Event, request, page.Request.Revision)
}
