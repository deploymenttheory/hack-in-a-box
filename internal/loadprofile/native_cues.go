package loadprofile

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/presentation"
)

type CueReceipt struct {
	Event    uuid.UUID
	Actor    uuid.UUID
	Revision int64
	Current  presentation.DisplayCue
}

// Cue sends one reviewed plain-text public override after a fresh controller
// read. The synthetic decision marker is provenance, not an invented native
// idempotency key. There is no retry after a conflict or uncertain original ACK.
func (w NativeWrites) Cue(
	ctx context.Context,
	actor Actor,
	event uuid.UUID,
	cue presentation.Cue,
) (Measurement, *AcceptedWrite, *CueReceipt, error) {
	measurement := nativeSample(actor, CueWrite, Response{})
	if cue.ID == uuid.Nil || cue.ContentID != nil {
		return measurement, nil, nil, fmt.Errorf(
			"%w: explicit plain-text cue required",
			errNativeWrite,
		)
	}
	path := "/api/v1/events/" + event.String() + "/presentation"
	base, err := actor.Session.Exchange(ctx, http.MethodGet, path, nil, nil)
	read := nativeSample(actor, CueRead, base)
	var page presentation.Page
	if err != nil {
		return read, nil, nil, err
	}
	if base.Status != http.StatusOK || decodeNative(base.Body, &page) != nil || !page.CanControl {
		return read, nil, nil, fmt.Errorf(
			"%w: current cue-controller authority unavailable",
			errNativeWrite,
		)
	}
	read.Outcome = Accepted
	if err := w.Metrics.Observe(read); err != nil {
		return read, nil, nil, err
	}
	fingerprint, err := payloadFingerprint(cue)
	if err != nil {
		return measurement, nil, nil, err
	}
	marker := uuid.New()
	spec := WriteSpec{
		Kind: CueEffect, Action: "override", Event: event,
		Actor: actor.Session.Person(), Resource: event, Version: page.Revision + 1,
		Command: marker, Fingerprint: fingerprint,
	}
	id, err := w.Attempts.Begin(spec)
	if err != nil {
		return measurement, nil, nil, err
	}
	input := presentation.Command{
		Action: "override", Override: &cue,
		Reason: "load-cue:" + marker.String() + ":" + fingerprint,
	}
	response, transport := actor.Session.Exchange(ctx, http.MethodPost, path, &page.Revision, input)
	measurement.Duration, measurement.Status = response.Duration, response.Status
	var result presentation.Page
	ack := transport == nil && response.Status == http.StatusOK &&
		decodeNative(response.Body, &result) == nil && result.Revision == spec.Version &&
		result.Override != nil && *result.Override == cue && result.OverrideMode == "running"
	measurement.Outcome, err = w.finish(id, response, transport, ack, true)
	if err != nil || !ack {
		return measurement, nil, nil, err
	}
	return measurement, &AcceptedWrite{Identity: spec.identity(), Fingerprint: fingerprint},
		&CueReceipt{
			Event:    event,
			Actor:    actor.Session.Person(),
			Revision: result.Revision,
			Current: presentation.DisplayCue{
				ID:    cue.ID,
				Title: cue.Title,
				Text:  cue.Text,
				URL:   cue.URL,
			},
		}, nil
}
