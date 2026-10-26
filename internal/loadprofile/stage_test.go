package loadprofile

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestReleaseCannotSelectACompressedClockOrSmallerCohort(t *testing.T) {
	for _, cfg := range []StageConfig{{"release", 1799 * time.Second, 5 * time.Second, SustainedPhase}, {"release", 30 * time.Minute, time.Minute, SustainedPhase}, {"unknown", time.Second, time.Millisecond, SustainedPhase}} {
		if err := cfg.Validate(1000); err == nil {
			t.Fatal("invalid release profile accepted", cfg)
		}
	}
	if err := (StageConfig{"release", 30 * time.Minute, 5 * time.Second, SustainedPhase}).Validate(
		999,
	); err == nil {
		t.Fatal("999 actors became 1000 active sessions")
	}
}

func TestDuplicatedPersonCannotFillTheCohort(t *testing.T) {
	// Synthetic unit objects exercise rejection before any request. They do not
	// establish authenticated sessions or count as native load evidence.
	session := &Session{person: uuid.New()}
	_, err := RunStage(
		context.Background(),
		StageConfig{"smoke", time.Second, time.Millisecond, SustainedPhase},
		[]Actor{{ID: "one", Session: session}, {ID: "two", Session: session}},
		func(context.Context, Actor, uint64) (Measurement, *AcceptedWrite, error) {
			t.Fatal("duplicate actor reached runtime")
			return Measurement{Phase: SustainedPhase, Surface: PublicSurface}, nil, nil
		},
		new(Metrics),
		new(WriteLedger),
	)
	if err == nil {
		t.Fatal("duplicated person accepted")
	}
}

func TestCancelledRunCannotStartOrCertifySustainedExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := RunStage(
		ctx,
		StageConfig{
			Profile:  "smoke",
			Duration: time.Second,
			Interval: time.Millisecond,
			Phase:    SustainedPhase,
		},
		nil,
		func(context.Context, Actor, uint64) (Measurement, *AcceptedWrite, error) {
			t.Fatal("cancelled phase ran an actor")
			return Measurement{}, nil, nil
		},
		new(Metrics),
		new(WriteLedger),
	)
	if err == nil || result.Completed || !result.Interrupted || result.Elapsed != 0 ||
		result.Executed != 0 {
		t.Fatal("cancelled/draining work certified runtime", result, err)
	}
}

func TestPostAcknowledgementOracleErrorCannotEraseAcceptedWrite(t *testing.T) {
	var ledger WriteLedger
	var metrics Metrics
	ack := AcceptedWrite{Identity: "synthetic-exact-original-ack", Fingerprint: "synthetic-fingerprint"}
	_, _ = RunStage(t.Context(), StageConfig{Profile: "smoke", Duration: 20 * time.Millisecond, Interval: time.Millisecond, Phase: SustainedPhase}, []Actor{{ID: "synthetic-actor", Session: &Session{person: uuid.New()}}}, func(context.Context, Actor, uint64) (Measurement, *AcceptedWrite, error) {
		return Measurement{Phase: SustainedPhase, Surface: PublicSurface, Operation: CueWrite, Status: 200, Outcome: Accepted}, &ack, errors.New("synthetic independent oracle failure after exact ACK")
	}, &metrics, &ledger)
	reports := metrics.Reports()
	if len(reports) != 1 || reports[0].Outcomes[Unexpected] == 0 {
		t.Fatal("independent oracle error hidden", reports)
	}
	report := ledger.Reconcile(nil)
	if report.Accepted != 1 || report.Missing != 1 {
		t.Fatal("post-ACK oracle error erased original accepted write", report)
	}
}
