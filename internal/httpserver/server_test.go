package httpserver_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpserver"
)

func TestShutdownDrainsRequestsAndStopsAcceptingConnections(t *testing.T) {
	t.Parallel()
	for _, force := range []bool{false, true} {
		name := "drain"
		if force {
			name = "deadline"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var lc net.ListenConfig
			listener, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			started, release, finished := make(
				chan struct{},
			), make(
				chan struct{},
			), make(
				chan struct{},
			)
			defer close(release)
			var requestCtx context.Context
			server := httpserver.New(
				config.Config{RequestTimeout: time.Second},
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					defer close(finished)
					requestCtx = r.Context()
					close(started)
					select {
					case <-release:
						_, _ = w.Write([]byte("completed"))
					case <-r.Context().Done():
					}
				}),
			)
			grace := 2 * time.Second
			if force {
				grace = 50 * time.Millisecond
			}
			result := make(chan error, 1)
			go func() {
				result <- httpserver.Serve(ctx, server, listener, grace, slog.New(slog.NewTextHandler(io.Discard, nil)))
			}()
			clientResult := make(chan error, 1)
			go func() {
				client := &http.Client{Timeout: 3 * time.Second}
				request, err := http.NewRequestWithContext(
					context.Background(),
					http.MethodGet,
					"http://"+listener.Addr().String(),
					nil,
				)
				if err != nil {
					clientResult <- err
					return
				}
				response, err := client.Do(request)
				if err == nil {
					defer response.Body.Close()
					var body []byte
					body, err = io.ReadAll(response.Body)
					if err == nil &&
						(response.StatusCode != http.StatusOK || string(body) != "completed") {
						err = errors.New("in-flight request did not complete successfully")
					}
				}
				clientResult <- err
			}()
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("request did not start")
			}
			cancel()
			// Wait until shutdown has closed the listener, before releasing work.
			deadline := time.Now().Add(time.Second)
			for {
				var dialer net.Dialer
				conn, err := dialer.DialContext(
					context.Background(),
					"tcp",
					listener.Addr().String(),
				)
				if err != nil {
					break
				}
				_ = conn.Close()
				if time.Now().After(deadline) {
					t.Fatal("shutdown kept accepting connections")
				}
				time.Sleep(time.Millisecond)
			}
			if !force {
				if requestCtx.Err() != nil {
					t.Fatal("shutdown cancelled an in-flight request")
				}
				release <- struct{}{}
			}
			select {
			case err := <-result:
				if force && !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("expected deadline failure, got %v", err)
				}
				if !force && err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("shutdown did not finish")
			}
			if err := <-clientResult; !force && err != nil {
				t.Fatal(err)
			}
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Fatal("handler did not finish")
			}
		})
	}
}
