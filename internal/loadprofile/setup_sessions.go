package loadprofile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type SetupSessionCleanup struct {
	Attempted    int           `json:"attempted"`
	OriginalACKs int           `json:"original_acknowledgements"`
	Unresolved   int           `json:"unresolved_original_responses"`
	Statuses     map[int]int   `json:"http_statuses"`
	Elapsed      time.Duration `json:"monotonic_elapsed_ns"`
}

// closeSetupSessions uses the normal CSRF-protected logout once per setup jar.
// Dropping a jar is not enough: leaving 1000 dormant native sessions would
// double the runtime database's session census after fresh authentication.
// Missing original responses remain unresolved; no logout is replayed.
func closeSetupSessions(
	ctx context.Context,
	actors []CohortActor,
) (SetupSessionCleanup, []PreparationCall, error) {
	started := time.Now()
	result := SetupSessionCleanup{Statuses: make(map[int]int)}
	calls := make([]PreparationCall, len(actors))
	queue := make(chan int)
	var workers sync.WaitGroup
	for range min(20, len(actors)) {
		workers.Go(func() {
			for index := range queue {
				actor := actors[index]
				response, err := actor.Session.Exchange(
					ctx,
					http.MethodPost,
					"/api/v1/logout",
					nil,
					nil,
				)
				input := sha256.Sum256([]byte("null"))
				output := sha256.Sum256(response.Body)
				calls[index] = PreparationCall{
					Actor:            actor.Session.Person(),
					Method:           http.MethodPost,
					Path:             "/api/v1/logout",
					Status:           response.Status,
					OriginalComplete: err == nil,
					Duration:         response.Duration,
					InputSHA256: hex.EncodeToString(
						input[:],
					),
					ResponseSHA256: hex.EncodeToString(output[:]),
				}
			}
		})
	}
	for index := range actors {
		queue <- index
	}
	close(queue)
	workers.Wait()
	for _, call := range calls {
		result.Attempted++
		result.Statuses[call.Status]++
		if call.Status == http.StatusNoContent && call.OriginalComplete {
			result.OriginalACKs++
		} else {
			result.Unresolved++
		}
	}
	result.Elapsed = time.Since(started)
	if result.Unresolved != 0 {
		return result, calls, fmt.Errorf(
			"%w: setup session logout did not retain every original204 response",
			errCohort,
		)
	}
	return result, calls, nil
}
