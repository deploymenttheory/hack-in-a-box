package app

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestLifecycleHTTPIntentKeepsExplicitNativeHeadersAndDefault(t *testing.T) {
	id := uuid.New()
	var keyed, ordinary atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodGet {
			ordinary.Add(1)
			if r.Header.Get("Idempotency-Key") != "" {
				t.Error("default read acquired a synthetic intent")
			}
			_, _ = io.WriteString(w, `{}`)
			return
		}
		keyed.Add(1)
		if r.Header.Get("Idempotency-Key") != id.String() || r.Header.Get("X-CSRF-Token") != "synthetic-csrf" || r.Header.Get("If-Match") != `"7"` || r.Header.Get("Origin") == "" {
			t.Error("keyed native request lost explicit intent/revision/auth transport metadata")
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"error":{"code":"conflict"}}`)
	}))
	defer server.Close()
	client := &lifecycleClient{origin: server.URL, csrf: "synthetic-csrf", http: &http.Client{Timeout: 3 * time.Second}}
	if err := client.call(t.Context(), http.MethodGet, "/api/v1/probe", -1, nil, nil, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	err := client.callIntent(t.Context(), http.MethodPost, "/api/v1/probe", 7, map[string]string{"action": "claim"}, nil, http.StatusOK, id)
	var status *lifecycleStatusError
	if !errors.As(err, &status) || status.Actual != http.StatusConflict || status.Expected != http.StatusOK || status.Error() != "POST native API returned 409, expected 200" || keyed.Load() != 1 || ordinary.Load() != 1 {
		t.Fatal("native conflict became a success, lost its exact status, or replayed")
	}
}

func TestLifecycleHTTPKeyedAcceptedWriteDoesNotReplayAfterConnectionLoss(t *testing.T) {
	var effects atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Cache-Control", "no-store")
			_, _ = io.WriteString(w, `{}`)
			return
		}
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Error(err)
		}
		effects.Add(1)
		connection, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = connection.Close()
	}))
	defer server.Close()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := &lifecycleClient{origin: server.URL, csrf: "synthetic-csrf", http: &http.Client{Transport: transport, Timeout: 3 * time.Second}}
	// Warm the reusable connection: keyed requests are otherwise eligible for
	// Go's retry path on a reused connection after the server accepts the write.
	if err := client.call(t.Context(), http.MethodGet, "/api/v1/probe", -1, nil, nil, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	err := client.callIntent(t.Context(), http.MethodPost, "/api/v1/probe", 0, map[string]string{"action": "claim"}, nil, http.StatusOK, uuid.New())
	if err == nil || effects.Load() != 1 {
		t.Fatalf("accepted keyed write replay/uncertainty: effects=%d caller_error=%t", effects.Load(), err != nil)
	}
}
