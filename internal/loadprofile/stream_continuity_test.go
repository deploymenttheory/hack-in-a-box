package loadprofile

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Synthetic normalized connection-reader controls, not native five-minute or
// thirty-minute evidence. Runtime MaintainDisplay obtains these timestamps
// exclusively from its actual HTTPS connection; it accepts no clock/shortener.
func TestDisplayContinuityReconnectsOnlyCleanNativeRotation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls := 0
	report, err := maintainDisplayConnections(
		ctx,
		"one",
		func(context.Context) (DisplayConnection, error) {
			calls++
			now := time.Now()
			if calls == 1 {
				return DisplayConnection{
					Started:  now.Add(-5 * time.Minute),
					Finished: now,
					Status:   http.StatusOK,
				}, nil
			}
			cancel()
			return DisplayConnection{
				Started:  now,
				Finished: now,
				Status:   http.StatusOK,
			}, context.Canceled
		},
	)
	if err != nil || calls != 2 || report.FiniteRotations != 1 || report.UnexpectedEnds != 0 || len(report.Connections) != 2 ||
		report.Connections[0].End != "native_finite_rotation" ||
		report.Connections[1].End != "driver_stopped" {
		t.Fatal("native finite rotation was dropped or hidden as an error", report, err)
	}
}

func TestDisplayContinuityDoesNotHideEarlyEOFOrAuthorityFailure(t *testing.T) {
	for _, failure := range []bool{false, true} {
		calls := 0
		report, err := maintainDisplayConnections(
			t.Context(),
			"one",
			func(context.Context) (DisplayConnection, error) {
				calls++
				now := time.Now()
				connection := DisplayConnection{
					Started:  now.Add(-time.Second),
					Finished: now,
					Status:   http.StatusOK,
				}
				if failure {
					return connection, errors.New("synthetic revoked/dependency/frame failure")
				}
				return connection, nil
			},
		)
		if err == nil || calls != 1 || report.FiniteRotations != 0 || report.UnexpectedEnds != 1 {
			t.Fatal("unexpected or unauthorised stream was retried/hidden", report, err)
		}
	}
}

func TestDisplayParserRejectsTruncatedMetadataWithoutData(t *testing.T) {
	event := uuid.New()
	ledger, err := NewDisplayLedger(map[string]uuid.UUID{"one": event})
	if err != nil {
		t.Fatal(err)
	}
	for _, frame := range []string{"event: snapshot\n", "id: 1\n", "event: snapshot\nid: 1\n"} {
		if err := consumeDisplay(strings.NewReader(frame), "one", event, nil, ledger); err == nil {
			t.Fatal("truncated native frame became clean finite rotation")
		}
	}
}
