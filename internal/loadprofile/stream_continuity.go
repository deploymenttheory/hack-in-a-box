package loadprofile

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Native display connections intentionally rotate at five minutes in
// httpapi.finiteOrDisplayTimeout. Reconnecting a completed read is separate from
// replaying an uncertain command. The native browser waits one second as well.
const nativeDisplayConnectionLifetime = 5 * time.Minute

type DisplayConnection struct {
	Started  time.Time     `json:"started"`
	Finished time.Time     `json:"finished"`
	Headers  time.Duration `json:"headers_latency_ns"`
	Status   int           `json:"http_status"`
	End      string        `json:"end"`
}

type DisplayContinuity struct {
	Stream          string              `json:"stream"`
	Connections     []DisplayConnection `json:"connections"`
	FiniteRotations int                 `json:"native_finite_rotations"`
	UnexpectedEnds  int                 `json:"unexpected_ends"`
}

// MaintainDisplay retains a complete connection history and reconnects only a
// clean native finite rotation. Every GET rechecks the same current bearer
// capability, with no cookie jar, account grant or Last-Event-ID replay. Early
// EOF, revocation, dependency events and invalid/private frames stay failures.
func MaintainDisplay(ctx context.Context, client *http.Client, origin, token, stream string,
	event uuid.UUID, forbidden []string, ledger *DisplayLedger,
) (DisplayContinuity, error) {
	return maintainDisplayConnections(
		ctx,
		stream,
		func(ctx context.Context) (DisplayConnection, error) {
			connection := DisplayConnection{Started: time.Now()}
			err := readDisplay(
				ctx,
				client,
				origin,
				token,
				stream,
				event,
				forbidden,
				ledger,
				func(status int, elapsed time.Duration) { connection.Status, connection.Headers = status, elapsed },
			)
			connection.Finished = time.Now()
			return connection, err
		},
	)
}

func maintainDisplayConnections(ctx context.Context, stream string,
	read func(context.Context) (DisplayConnection, error),
) (DisplayContinuity, error) {
	report := DisplayContinuity{Stream: stream}
	for {
		connection, err := read(ctx)
		switch {
		case ctx.Err() != nil:
			connection.End = "driver_stopped"
			report.Connections = append(report.Connections, connection)
			return report, nil
		case err != nil:
			connection.End = "unexpected_error"
			report.UnexpectedEnds++
			report.Connections = append(report.Connections, connection)
			return report, err
		case connection.Status != http.StatusOK || connection.Finished.Sub(connection.Started) < nativeDisplayConnectionLifetime:
			connection.End = "unexpected_early_eof"
			report.UnexpectedEnds++
			report.Connections = append(report.Connections, connection)
			return report, fmt.Errorf(
				"%w: display ended before its native finite rotation",
				errDisplayProof,
			)
		default:
			connection.End = "native_finite_rotation"
			report.FiniteRotations++
			report.Connections = append(report.Connections, connection)
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return report, nil
		case <-timer.C:
		}
	}
}
