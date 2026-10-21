package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
)

func testRouter(t *testing.T) (*chi.Mux, *bytes.Buffer) {
	t.Helper()
	cfg, err := config.Load(func(key string) string {
		if key == "HACKBOX_ENV" {
			return "test"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxBodyBytes = 8
	cfg.RequestTimeout = 50 * time.Millisecond
	logs := new(bytes.Buffer)
	return NewRouter(cfg, slog.New(slog.NewJSONHandler(logs, nil))), logs
}

func assertError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status || response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected response: %d %v %s", response.Code, response.Header(), response.Body)
	}
	var body errorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != code || body.Error.Message == "" || body.Error.RequestId == "" ||
		body.Error.RequestId != response.Header().Get("X-Request-ID") {
		t.Fatalf("invalid error envelope: %+v", body)
	}
}

func TestHealthAndRoutingErrors(t *testing.T) {
	t.Parallel()
	router, _ := testRouter(t)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Header.Set("X-Request-ID", "untrusted-client-id")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "{\"status\":\"ok\"}\n" {
		t.Fatalf("health failed: %d %s", response.Code, response.Body)
	}
	if id := response.Header().Get("X-Request-ID"); id == "" || id == "untrusted-client-id" {
		t.Fatalf("request ID was not generated: %q", id)
	}
	for _, tc := range []struct {
		method, path, code string
		status             int
	}{
		{http.MethodGet, "/missing", "not_found", http.StatusNotFound},
		{http.MethodGet, "/api/v1/missing", "not_found", http.StatusNotFound},
		{http.MethodPost, "/healthz", "method_not_allowed", http.StatusMethodNotAllowed},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
		assertError(t, response, tc.status, tc.code)
	}
}

func TestBodyLimitIncludesUnknownLengths(t *testing.T) {
	t.Parallel()
	for _, length := range []int64{9, -1} {
		router, _ := testRouter(t)
		reached := false
		router.Post("/test", func(w http.ResponseWriter, r *http.Request) { reached = true })
		request := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("123456789"))
		request.ContentLength = length
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		assertError(t, response, http.StatusRequestEntityTooLarge, "body_too_large")
		if reached {
			t.Fatal("oversized request reached handler")
		}
	}
}

func TestBodyAtLimitIsPreserved(t *testing.T) {
	t.Parallel()
	router, _ := testRouter(t)
	router.Post("/test", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}
		writeJSON(w, http.StatusOK, string(body))
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("12345678")),
	)
	if response.Code != http.StatusOK || response.Body.String() != "\"12345678\"\n" {
		t.Fatalf("body was not preserved: %d %s", response.Code, response.Body)
	}
}

func TestPanicDiscardsPartialResponseAndRedactsPayload(t *testing.T) {
	t.Parallel()
	router, logs := testRouter(t)
	router.Get("/panic", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("private-partial-response"))
		panic("private-panic-payload")
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/panic", nil))
	assertError(t, response, http.StatusInternalServerError, "internal_error")
	if strings.Contains(response.Body.String()+logs.String(), "private-") {
		t.Fatal("panic details leaked")
	}
}

func TestTimeoutReturnsBeforeUncooperativeHandler(t *testing.T) {
	t.Parallel()
	router, _ := testRouter(t)
	release, finished, cancelled := make(chan struct{}), make(chan struct{}), make(chan struct{})
	router.Get("/slow", func(w http.ResponseWriter, r *http.Request) {
		defer close(finished)
		<-r.Context().Done()
		close(cancelled)
		<-release
		_, _ = w.Write([]byte("late-response"))
	})
	defer func() { close(release); <-finished }()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/slow", nil))
	assertError(t, response, http.StatusServiceUnavailable, "request_timeout")
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("handler context was not cancelled")
	}
}
