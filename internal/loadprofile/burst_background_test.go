package loadprofile

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
)

// Synthetic coordinator/real HTTPS mutation counter, not native auth/domain
// evidence. One unresolved reader must prevent ANY registration wire command;
// a stored or authenticated actor count cannot release the burst by itself.
func TestBurstDoesNotWriteBeforeEveryBackgroundPersonHasAcceptedRead(t *testing.T) {
	var writes atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writes.Add(1)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	event, form := uuid.New(), uuid.New()
	var actors []Actor
	var tasks []RegistrationTask
	for _, id := range []string{"ready", "held"} {
		actor := Actor{
			ID: id,
			Session: &Session{
				person:  uuid.New(),
				origin:  server.URL,
				client:  server.Client(),
				csrf:    "synthetic",
				surface: PublicSurface,
			},
		}
		actors = append(actors, actor)
		tasks = append(
			tasks,
			RegistrationTask{
				Actor: actor,
				Input: registration.Application{ID: uuid.New(), FormID: form, FormVersion: 1},
			},
		)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	held := make(chan struct{})
	var once sync.Once
	read := func(ctx context.Context, actor Actor, _ uint64) (Measurement, *AcceptedWrite, error) {
		sample := Measurement{
			Phase: actor.Phase, Surface: PublicSurface, Operation: SessionRead,
			Duration: time.Millisecond, Status: http.StatusOK, Outcome: Accepted,
		}
		if actor.ID == "held" {
			once.Do(func() { close(held) })
			<-ctx.Done()
			sample.Status, sample.Outcome = 0, Unexpected
			return sample, nil, ctx.Err()
		}
		return sample, nil, nil
	}
	finished := make(chan error, 1)
	var metrics Metrics
	var ledger WriteLedger
	var attempts AttemptLedger
	go func() {
		_, _, err := runBurstWithBackground(ctx, RuntimeConfig{Interval: 10 * time.Millisecond},
			BurstConfig{Profile: "smoke", Event: event, Form: form, FormVersion: 1}, actors, tasks,
			read, NativeWrites{Attempts: &attempts, Metrics: &metrics}, &metrics, &ledger)
		finished <- err
	}()
	<-held
	select {
	case err := <-finished:
		t.Fatal("readiness stage ended before held reader was released", err)
	case <-time.After(10 * time.Millisecond):
	}
	if writes.Load() != 0 {
		t.Fatal("registration burst preceded full reader readiness", writes.Load())
	}
	cancel()
	if err := <-finished; err == nil || writes.Load() != 0 || len(attempts.Specs()) != 0 {
		t.Fatal("cancelled incomplete readiness emitted a registration command", err, writes.Load())
	}
}
