package loadprofile

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDisplayDrainWaitsForAlreadyPublishedReceiptWithoutPaddingLatency(t *testing.T) {
	event := uuid.New()
	ledger, err := NewDisplayLedger(map[string]uuid.UUID{"one": event})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	snapshot := syntheticSnapshot(event, 1)
	if err := ledger.Published(event, 1, syntheticAnchor(start), *snapshot.Current); err != nil {
		t.Fatal(err)
	}
	// This reproduces the old runner boundary: a final native publication has
	// been acknowledged, but its allowed asynchronous receipt has not arrived.
	if premature, _ := ledger.Measurements(time.Now()); premature.MissingReceipts != 1 {
		t.Fatal("control did not reproduce premature missing-receipt classification")
	}
	finished := make(chan error, 1)
	go func() { finished <- ledger.AwaitPublishedReceipts(t.Context()) }()
	select {
	case err := <-finished:
		t.Fatal("drain returned before actual receipt", err)
	case <-time.After(10 * time.Millisecond):
	}
	received := time.Now()
	if err := ledger.Received("one", snapshot, received); err != nil {
		t.Fatal(err)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	report, samples := ledger.Measurements(time.Now())
	if report.MatchedReceipts != 1 || report.MissingReceipts != 0 || len(samples) != 1 ||
		samples[0].Duration != received.Sub(start) {
		t.Fatal("drain rewrote actual publication/receipt latency", report, samples)
	}
}

func TestDisplayDrainKeepsLateAndPartialReceiptsAsFailures(t *testing.T) {
	event := uuid.New()
	ledger, err := NewDisplayLedger(map[string]uuid.UUID{"one": event, "two": event})
	if err != nil {
		t.Fatal(err)
	}
	published := time.Now().Add(-3 * time.Second)
	snapshot := syntheticSnapshot(event, 1)
	if err := ledger.Published(
		event,
		1,
		syntheticAnchor(published),
		*snapshot.Current,
	); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Received("one", snapshot, time.Now()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := ledger.AwaitPublishedReceipts(ctx); err == nil {
		t.Fatal("partial/cancelled stream became a complete receipt proof")
	}
	report, samples := ledger.Measurements(time.Now())
	if report.MatchedReceipts != 1 || report.MissingReceipts != 1 || len(samples) != 2 {
		t.Fatal("partial receipts vanished from denominator", report, samples)
	}
	for _, sample := range samples {
		if sample.Duration <= DisplayUpdate.Limit() {
			t.Fatal(
				"drain made a late or missing update meet the unchanged two-second target",
				samples,
			)
		}
	}
}
