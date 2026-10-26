package loadprofile

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/presentation"
)

var errDisplayProof = errors.New("invalid load display evidence")

type (
	displayReceipt struct {
		at     time.Time
		digest string
	}
	cueKey struct {
		event    uuid.UUID
		sequence int64
	}
	cueObservation struct {
		phase       Phase
		published   time.Time
		offsetUpper time.Duration
		expected    string
		received    map[string]displayReceipt
	}
)

// DisplayLedger correlates only REAL published source revisions to their first
// matching receipt on EVERY expected display. Heartbeats/repeated snapshots do
// not count as new updates. A receiver can beat the publisher's HTTP response;
// that early matching receipt is preserved until its native audit anchor arrives.
type DisplayLedger struct {
	mu      sync.Mutex
	streams map[string]uuid.UUID
	cues    map[cueKey]*cueObservation
	changed chan struct{}
}

func NewDisplayLedger(streams map[string]uuid.UUID) (*DisplayLedger, error) {
	if len(streams) == 0 {
		return nil, fmt.Errorf("%w: current stream cohort required", errDisplayProof)
	}
	owned := make(map[string]uuid.UUID, len(streams))
	for id, event := range streams {
		if id == "" || event == uuid.Nil {
			return nil, fmt.Errorf("%w: invalid current display identity", errDisplayProof)
		}
		owned[id] = event
	}
	return &DisplayLedger{
		streams: owned,
		cues:    make(map[cueKey]*cueObservation),
		changed: make(chan struct{}),
	}, nil
}

func (l *DisplayLedger) Published(
	event uuid.UUID,
	sequence int64,
	anchor CueAnchor,
	expected presentation.DisplayCue,
) error {
	digest, err := payloadFingerprint(expected)
	if err != nil || expected.ID == uuid.Nil {
		return fmt.Errorf("%w: exact acknowledged current cue required", errDisplayProof)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !anchor.Phase.valid() || sequence < 1 || anchor.AuditAt.IsZero() ||
		anchor.DatabaseAt.IsZero() ||
		anchor.DriverBefore.IsZero() ||
		anchor.DriverAfter.Before(anchor.DriverBefore) ||
		anchor.AuditAt.After(anchor.DatabaseAt) {
		return fmt.Errorf(
			"%w: actual publication revision and native audit timestamp required",
			errDisplayProof,
		)
	}
	wanted := false
	for _, scope := range l.streams {
		if scope == event {
			wanted = true
			break
		}
	}
	if !wanted {
		return fmt.Errorf("%w: publication has no scoped display", errDisplayProof)
	}
	key := cueKey{event, sequence}
	entry := l.cues[key]
	if entry == nil {
		entry = &cueObservation{received: make(map[string]displayReceipt)}
		l.cues[key] = entry
	}
	if !entry.published.IsZero() &&
		(!entry.published.Equal(anchor.AuditAt) || entry.phase != anchor.Phase || entry.expected != digest) {
		return fmt.Errorf("%w: contradictory publication anchor", errDisplayProof)
	}
	entry.expected = digest
	entry.published = anchor.AuditAt
	entry.phase = anchor.Phase
	entry.offsetUpper = anchor.DatabaseAt.Sub(anchor.DriverBefore)
	l.signal()
	return nil
}

func (l *DisplayLedger) Received(
	stream string,
	snapshot presentation.Snapshot,
	at time.Time,
) error {
	event, sequence := snapshot.EventID, snapshot.Sequence
	digest, err := payloadFingerprint(snapshot.Current)
	if err != nil {
		return fmt.Errorf("%w: current display projection fingerprint failed", errDisplayProof)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.streams[stream] != event || event == uuid.Nil || sequence < 0 || at.IsZero() {
		return fmt.Errorf("%w: snapshot does not match current scoped stream", errDisplayProof)
	}
	key := cueKey{event, sequence}
	entry := l.cues[key]
	if entry == nil {
		entry = &cueObservation{received: make(map[string]displayReceipt)}
		l.cues[key] = entry
	}
	if previous, exists := entry.received[stream]; !exists || at.Before(previous.at) {
		entry.received[stream] = displayReceipt{at: at, digest: digest}
		l.signal()
	}
	return nil
}

type DisplayReport struct {
	Publications       int `json:"publications"`
	ExpectedReceipts   int `json:"expected_receipts"`
	MatchedReceipts    int `json:"matched_receipts"`
	MismatchedReceipts int `json:"mismatched_receipts"`
	MissingReceipts    int `json:"missing_receipts"`
	InvalidClockOrder  int `json:"invalid_clock_order"`
}

// signal runs only while mu is held. Waiters observe actual receipt events,
// not arbitrary sleeps or a changed latency threshold.
func (l *DisplayLedger) signal() {
	close(l.changed)
	l.changed = make(chan struct{})
}

// AwaitPublishedReceipts drains only already-published sources after writers
// stop. Its bound is the existing native-client I/O deadline, independently of
// the two-second P95 target. Measurements still
// use the actual publication/receipt timestamps, so a late frame stays late and
// a missing frame remains in the denominator. No heartbeat can satisfy it.
func (l *DisplayLedger) AwaitPublishedReceipts(ctx context.Context) error {
	deadline := time.NewTimer(NativeRequestTimeout)
	defer deadline.Stop()
	for {
		pending, changed := l.pendingPublished()
		if !pending {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("drain already-published display receipts: %w", ctx.Err())
		case <-deadline.C:
			if pending, _ := l.pendingPublished(); !pending {
				return nil
			}
			return fmt.Errorf(
				"%w: published receipt drain incomplete within native I/O deadline",
				errDisplayProof,
			)
		case <-changed:
		}
	}
}

func (l *DisplayLedger) pendingPublished() (bool, <-chan struct{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, cue := range l.cues {
		if cue.published.IsZero() {
			continue
		}
		for stream, event := range l.streams {
			if _, received := cue.received[stream]; event == key.event && !received {
				return true, l.changed
			}
		}
	}
	return false, l.changed
}

// Measurements is a final snapshot, not an incremental counter: callers add
// these observations once after streams have drained. Missing revisions never
// vanish into a success-only p95. They also fail the independent SSE oracle.
func (l *DisplayLedger) Measurements(finished time.Time) (DisplayReport, []Measurement) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var report DisplayReport
	var measurements []Measurement
	for key, entry := range l.cues {
		if entry.published.IsZero() {
			continue
		}
		report.Publications++
		for stream, event := range l.streams {
			if event != key.event {
				continue
			}
			report.ExpectedReceipts++
			receipt, exists := entry.received[stream]
			at := receipt.at
			outcome := Accepted
			switch {
			case !exists:
				report.MissingReceipts++
				at = finished
				outcome = Unexpected
			case receipt.digest != entry.expected:
				report.MismatchedReceipts++
				outcome = Unexpected
			default:
				report.MatchedReceipts++
			}
			delay := at.Sub(entry.published) + entry.offsetUpper
			if delay < 0 {
				report.InvalidClockOrder++
				outcome = Unexpected
				delay = 0
			}
			measurements = append(
				measurements,
				Measurement{
					Phase: entry.phase, Surface: DisplaySurface,
					Operation: DisplayUpdate,
					Duration:  delay,
					Status:    200,
					Outcome:   outcome,
				},
			)
		}
	}
	return report, measurements
}

// ReadDisplay consumes one normal authorised SSE connection. The caller may
// reconnect after the server's bounded stream EOF using the SAME current
// bearer token; it must record connection failures/revocation separately. No
// browser cookies, URL token, fetch replacement or provider response is used.
func ReadDisplay(
	ctx context.Context,
	client *http.Client,
	origin, token, stream string,
	event uuid.UUID,
	forbidden []string,
	ledger *DisplayLedger,
) error {
	return readDisplay(ctx, client, origin, token, stream, event, forbidden, ledger, nil)
}

func readDisplay(
	ctx context.Context, client *http.Client, origin, token, stream string,
	event uuid.UUID, forbidden []string, ledger *DisplayLedger,
	opened func(int, time.Duration),
) error {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.Path != "" || u.RawQuery != "" ||
		u.User != nil ||
		u.Fragment != "" ||
		token == "" ||
		strings.ContainsAny(token, "\r\n") {
		return fmt.Errorf("%w: exact HTTPS origin and header capability required", errDisplayProof)
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		origin+"/api/v1/events/"+event.String()+"/display/stream",
		nil,
	)
	if err != nil {
		return fmt.Errorf("%w: create current stream request", errDisplayProof)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "text/event-stream")
	// A fresh client has no cookie jar and never follows redirects. Reuse only
	// the caller's verified-TLS transport and cancellation policy.
	transport := client.Transport
	connection := &http.Client{
		Transport:     transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	started := time.Now()
	response, err := connection.Do(request)
	if err != nil {
		if opened != nil {
			opened(0, time.Since(started))
		}
		return fmt.Errorf("%w: current stream connection failed", errDisplayProof)
	}
	if opened != nil {
		opened(response.StatusCode, time.Since(started))
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK ||
		!strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") ||
		!strings.Contains(response.Header.Get("Cache-Control"), "no-store") {
		return fmt.Errorf(
			"%w: current stream status %d or headers invalid",
			errDisplayProof,
			response.StatusCode,
		)
	}
	return consumeDisplay(response.Body, stream, event, forbidden, ledger)
}

func consumeDisplay(
	reader io.Reader,
	stream string,
	event uuid.UUID,
	forbidden []string,
	ledger *DisplayLedger,
) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	var kind, id string
	var data bytes.Buffer
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, ":") {
			continue
		}
		if line != "" {
			field, value, found := strings.Cut(line, ":")
			if !found {
				value = ""
			}
			value = strings.TrimPrefix(value, " ")
			switch field {
			case "event":
				kind = value
			case "id":
				id = value
			case "data":
				if data.Len() > 0 {
					data.WriteByte('\n')
				}
				if data.Len()+len(value) > 1<<20 {
					return fmt.Errorf("%w: stream record exceeds bounded capture", errDisplayProof)
				}
				data.WriteString(value)
			}
			continue
		}
		switch kind {
		case "snapshot":
			raw := data.Bytes()
			for _, canary := range forbidden {
				if canary != "" && bytes.Contains(raw, []byte(canary)) {
					return fmt.Errorf(
						"%w: forbidden private canary in display projection",
						errDisplayProof,
					)
				}
			}
			var snapshot presentation.Snapshot
			if decodeNative(raw, &snapshot) != nil || snapshot.EventID != event ||
				id != strconv.FormatInt(snapshot.Sequence, 10) {
				return fmt.Errorf("%w: stream source/event/revision mismatch", errDisplayProof)
			}
			if err := ledger.Received(stream, snapshot, time.Now()); err != nil {
				return err
			}
		case "revoked", "reconnect":
			return fmt.Errorf("%w: stream authority or dependency ended", errDisplayProof)
		case "":
			if data.Len() != 0 {
				return fmt.Errorf("%w: stream data lacks an explicit source event", errDisplayProof)
			}
		default:
			return fmt.Errorf("%w: unknown stream event", errDisplayProof)
		}
		kind, id = "", ""
		data.Reset()
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%w: SSE body read failed", errDisplayProof)
	}
	if data.Len() != 0 || kind != "" || id != "" {
		return fmt.Errorf("%w: incomplete SSE record", errDisplayProof)
	}
	return nil
}
