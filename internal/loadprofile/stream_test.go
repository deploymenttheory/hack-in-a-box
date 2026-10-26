package loadprofile

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/presentation"
)

func TestDisplayHeartbeatsAndCurrentReadsCannotReplacePublishedUpdateProof(t *testing.T) {
	event := uuid.New()
	ledger, err := NewDisplayLedger(map[string]uuid.UUID{"one": event, "two": event})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	for range 20 {
		if err := ledger.Received("one", syntheticSnapshot(event, 6), start); err != nil {
			t.Fatal(err)
		}
	}
	if report, _ := ledger.Measurements(start); report.Publications != 0 {
		t.Fatal("unpublished current source became update proof", report)
	}
	// The first actual SSE receipt may beat the publisher response/audit query.
	if err := ledger.Received("one", syntheticSnapshot(event, 7), start.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Published(event, 7, syntheticAnchor(start), *syntheticSnapshot(event, 7).Current); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Published(event, 8, syntheticAnchor(start.Add(2*time.Second)), *syntheticSnapshot(event, 8).Current); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Received("one", syntheticSnapshot(event, 7), start.Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	report, samples := ledger.Measurements(start.Add(12 * time.Second))
	if report.Publications != 2 || report.ExpectedReceipts != 4 || report.MatchedReceipts != 1 ||
		report.MissingReceipts != 3 ||
		len(samples) != 4 {
		t.Fatal("missing updates vanished into successful p95", report, samples)
	}
	var matched, missed int
	for _, sample := range samples {
		if sample.Outcome == Accepted {
			matched++
			if sample.Duration != time.Second {
				t.Fatal("heartbeat replaced first matching receipt", sample)
			}
		} else {
			missed++
		}
	}
	if matched != 1 || missed != 3 {
		t.Fatal(samples)
	}
}

func TestDisplayParserRequiresExactEventRevisionAndPositiveProjection(t *testing.T) {
	event := uuid.New()
	ledger, err := NewDisplayLedger(map[string]uuid.UUID{"one": event})
	if err != nil {
		t.Fatal(err)
	}
	frame := func(id int64, snapshot presentation.Snapshot) string {
		raw, err := json.Marshal(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf("id: %d\nevent: snapshot\ndata: %s\n\n", id, raw)
	}
	valid := frame(7, presentation.Snapshot{EventID: event, Sequence: 7})
	if err := consumeDisplay(
		strings.NewReader(": heartbeat\n\n"+valid+valid),
		"one",
		event,
		nil,
		ledger,
	); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"wrong-event": frame(7, presentation.Snapshot{EventID: uuid.New(), Sequence: 7}), "wrong-id": frame(8, presentation.Snapshot{EventID: event, Sequence: 7}), "private-canary": strings.Replace(valid, "\"event_name\":\"\"", "\"event_name\":\"PRIVATE_FIXTURE_WORDS\"", 1), "unknown-private-field": strings.Replace(valid, "\"sequence\":7", "\"unexpected_private_field\":\"hidden\",\"sequence\":7", 1), "truncated": strings.TrimSuffix(valid, "\n\n"), "revoked": "event: revoked\ndata: {}\n\n"} {
		t.Run(name, func(t *testing.T) {
			err := consumeDisplay(
				strings.NewReader(body),
				"one",
				event,
				[]string{"PRIVATE_FIXTURE_WORDS"},
				ledger,
			)
			if err == nil {
				t.Fatal("invalid display projection accepted")
			}
			if strings.Contains(err.Error(), "PRIVATE_FIXTURE_WORDS") {
				t.Fatal("private words copied into diagnostic")
			}
		})
	}
}

func syntheticAnchor(at time.Time) CueAnchor {
	return CueAnchor{
		Phase:        SustainedPhase,
		AuditAt:      at,
		DatabaseAt:   at,
		DriverBefore: at,
		DriverAfter:  at,
	}
}

func TestDisplayLatencyUsesConservativeServerDriverClockOffset(t *testing.T) {
	event := uuid.New()
	ledger, err := NewDisplayLedger(map[string]uuid.UUID{"one": event})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	// The database clock is one second ahead. Driver/DB sampling spans2ms;
	// the upper offset bound adds1ms rather than hiding latency as zero.
	anchor := CueAnchor{
		Phase:        SustainedPhase,
		AuditAt:      start.Add(time.Second),
		DatabaseAt:   start.Add(time.Second + 2*time.Millisecond),
		DriverBefore: start.Add(time.Millisecond),
		DriverAfter:  start.Add(3 * time.Millisecond),
	}
	if err := ledger.Published(event, 2, anchor, *syntheticSnapshot(event, 2).Current); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Received("one", syntheticSnapshot(event, 2), start.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	report, samples := ledger.Measurements(start.Add(3 * time.Second))
	if report.InvalidClockOrder != 0 || len(samples) != 1 ||
		samples[0].Duration != time.Second+time.Millisecond {
		t.Fatal("clock skew made receipt artificially fast", report, samples)
	}
}

func syntheticSnapshot(event uuid.UUID, sequence int64) presentation.Snapshot {
	return presentation.Snapshot{EventID: event, Sequence: sequence, Current: &presentation.DisplayCue{ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte("synthetic-cue")), Title: "Synthetic current cue", Text: "Synthetic public projection"}}
}

func TestSameRevisionWithWrongCueDoesNotBecomeSuccessfulSSELatency(t *testing.T) {
	event := uuid.New()
	ledger, err := NewDisplayLedger(map[string]uuid.UUID{"one": event})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	expected := syntheticSnapshot(event, 7)
	wrong := syntheticSnapshot(event, 7)
	wrong.Current.Text = "Different stale cue"
	if err := ledger.Received("one", wrong, start.Add(time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Published(event, 7, syntheticAnchor(start), *expected.Current); err != nil {
		t.Fatal(err)
	}
	report, measurements := ledger.Measurements(start.Add(time.Second))
	if report.MatchedReceipts != 0 || report.MismatchedReceipts != 1 || len(measurements) != 1 || measurements[0].Outcome != Unexpected {
		t.Fatal("matching revision concealed wrong projection", report, measurements)
	}
}
