package loadprofile

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

// Real HTTPS/HTTP1 transport control, not a domain/provider fixture: the server
// consumes the complete first POST then loses its response. The native driver's
// missing original response must not silently become a replayed command.
func TestNativeCommandDoesNotReplayAfterConsumedBodyLosesResponse(t *testing.T) {
	var writes atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte("warm-idle-connection"))
			return
		}
		if r.Header.Get("Idempotency-Key") == "" {
			t.Error("native command key was stripped to suppress replay")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || len(body) == 0 {
			t.Error("write body was not consumed", err)
		}
		if writes.Add(1) == 1 {
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Error("HTTP1 control lacks hijack support")
				return
			}
			connection, _, err := hijacker.Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = connection.Close()
			return
		}
		_, _ = w.Write([]byte("{}"))
	}))
	defer server.Close()
	session := &Session{origin: server.URL, person: uuid.New(), csrf: "synthetic-csrf", client: server.Client(), surface: PublicSurface}
	if _, err := session.Exchange(t.Context(), http.MethodGet, "/api/v1/session", nil, nil); err != nil {
		t.Fatal("warmup failed", err)
	}
	_, err := session.Command(t.Context(), http.MethodPost, "/api/v1/events/fixture/help/request/actions", nil,
		map[string]string{"action": "synthetic-transport-control"}, uuid.New())
	if writes.Load() != 1 || err == nil {
		t.Fatal("lost original response was automatically replayed", "post_count", writes.Load(), "transport_error", err)
	}
}
