package loadprofile

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestReleaseCannotSelectACompressedClockOrSmallerCohort(t *testing.T) {
	for _, cfg := range []StageConfig{{"release", 1799 * time.Second, 5 * time.Second}, {"release", 30 * time.Minute, time.Minute}, {"unknown", time.Second, time.Millisecond}} {
		if err := cfg.Validate(1000); err == nil {
			t.Fatal("invalid release profile accepted", cfg)
		}
	}
	if err := (StageConfig{"release", 30 * time.Minute, 5 * time.Second}).Validate(
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
		StageConfig{"smoke", time.Second, time.Millisecond},
		[]Actor{{"one", session}, {"two", session}},
		func(context.Context, Actor, uint64) (Measurement, *AcceptedWrite, error) {
			t.Fatal("duplicate actor reached runtime")
			return Measurement{}, nil, nil
		},
		new(Metrics),
		new(WriteLedger),
	)
	if err == nil {
		t.Fatal("duplicated person accepted")
	}
}
