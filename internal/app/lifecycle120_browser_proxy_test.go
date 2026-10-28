package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/httpserver"
)

// This proxy belongs only to the explicit verification binary and isolated
// Compose network. It lets a host browser use the actual native origins and
// OIDC callbacks without replacing APIs, cookies, sessions or CSRF checks.
func lifecycleBrowserProxy(transport http.RoundTripper) http.Handler {
	proxy := &httputil.ReverseProxy{
		Director:  func(r *http.Request) { r.Header.Del("Proxy-Authorization"); r.Header.Del("Proxy-Connection") },
		Transport: transport,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, "isolated browser upstream unavailable", http.StatusBadGateway)
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !r.URL.IsAbs() && r.Method == http.MethodGet && r.URL.Path == "/readyz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == http.MethodConnect || r.URL.Scheme != "http" || r.URL.User != nil ||
			r.Host != r.URL.Host {
			http.Error(w, "outside isolated browser origins", http.StatusForbidden)
			return
		}
		switch r.URL.Host {
		case "public.localhost:8080", "admin.localhost:8080", "provider.localhost:8080", "styleguide.localhost:8080":
			proxy.ServeHTTP(w, r)
		default:
			http.Error(w, "outside isolated browser origins", http.StatusForbidden)
		}
	})
}

func runLifecycleBrowserProxy(ctx context.Context) error {
	if os.Getenv("HACKBOX_LIFECYCLE_ISOLATED") != "synthetic-only" {
		return errors.New("browser proxy requires isolated synthetic composition")
	}
	server := &http.Server{
		Addr:              "0.0.0.0:8080",
		Handler:           lifecycleBrowserProxy(http.DefaultTransport),
		ReadHeaderTimeout: 5 * time.Second,
	}
	var listen net.ListenConfig
	network, err := listen.Listen(ctx, "tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("listen isolated browser proxy: %w", err)
	}
	return httpserver.Serve(
		ctx,
		server,
		network,
		10*time.Second,
		slog.New(slog.NewJSONHandler(os.Stderr, nil)),
	)
}

type lifecycleBrowserRoundTrip func(*http.Request) (*http.Response, error)

func (f lifecycleBrowserRoundTrip) RoundTrip(
	r *http.Request,
) (*http.Response, error) {
	return f(r)
}

// Component isolation proof only; it is not a browser/native journey verdict.
func TestLifecycleBrowserProxyRestrictsOriginsAndPreservesNativeAuthority(t *testing.T) {
	cases := []struct {
		name, method, url string
		want              int
	}{
		{
			"native public",
			http.MethodGet,
			"http://public.localhost:8080/api/v1/session",
			http.StatusNoContent,
		},
		{"native admin", http.MethodGet, "http://admin.localhost:8080/auth/login", http.StatusNoContent},
		{
			"synthetic issuer",
			http.MethodGet,
			"http://provider.localhost:8080/authorize",
			http.StatusNoContent,
		},
		{"independent guide", http.MethodGet, "http://styleguide.localhost:8080/", http.StatusNoContent},
		{"external origin", http.MethodGet, "http://example.com/", http.StatusForbidden},
		{"external TLS tunnel", http.MethodConnect, "https://github.com/", http.StatusForbidden},
		{"wrong native port", http.MethodGet, "http://admin:5432/", http.StatusForbidden},
		{
			"credential-bearing URL",
			http.MethodGet,
			"http://synthetic:synthetic@admin:8080/",
			http.StatusForbidden,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			called := false
			handler := lifecycleBrowserProxy(
				lifecycleBrowserRoundTrip(func(r *http.Request) (*http.Response, error) {
					called = true
					if r.Header.Get("Cookie") != "synthetic-session=original" ||
						r.Header.Get("X-CSRF-Token") != "synthetic-original-csrf" ||
						r.Header.Get("Origin") != "http://public.localhost:8080" {
						t.Fatal("proxy replaced native browser authority")
					}
					if r.Header.Get("Proxy-Authorization") != "" {
						t.Fatal("proxy-specific credentials reached the application")
					}
					return &http.Response{
						StatusCode: http.StatusNoContent,
						Header:     make(http.Header),
						Body:       http.NoBody,
					}, nil
				}),
			)
			request := httptest.NewRequest(test.method, test.url, nil)
			request.Header.Set("Cookie", "synthetic-session=original")
			request.Header.Set("X-CSRF-Token", "synthetic-original-csrf")
			request.Header.Set("Origin", "http://public.localhost:8080")
			request.Header.Set("Proxy-Authorization", "synthetic-proxy-only")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.want || called != (test.want == http.StatusNoContent) {
				t.Fatalf("origin scope status=%d forwarded=%t", response.Code, called)
			}
		})
	}
}
