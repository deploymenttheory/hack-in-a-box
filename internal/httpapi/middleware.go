package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/githubwebhooks"
)

type requestIDKey struct{}

func requestID(r *http.Request) string {
	id, _ := r.Context().Value(requestIDKey{}).(string)
	return id
}

func identify(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Generate our own ID rather than trusting an arbitrary incoming header.
		id := rand.Text()
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}

func recoverPanics(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					if err, ok := recovered.(error); ok && errors.Is(err, http.ErrAbortHandler) {
						panic(http.ErrAbortHandler)
					}
					// Panic payloads and stack locals can contain secrets.
					logger.Error("request panicked", "request_id", requestID(r))
					writeError(
						w,
						r,
						http.StatusInternalServerError,
						"internal_error",
						"An internal error occurred.",
					)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// timeout bounds finite JSON responses and propagates cancellation to handlers.
// Streaming/download routes must use a separate policy when introduced.
func timeout(duration time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// The timeout goroutine can outlive Chi's pooled request context.
			// Give it an independent routing context before it starts routing.
			if routing := chi.RouteContext(r.Context()); routing != nil {
				r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, routing.Clone()))
			}
			body, err := json.Marshal(
				errorBody(r, "request_timeout", "The request exceeded its time limit."),
			)
			if err != nil {
				panic(err)
			}
			http.TimeoutHandler(next, duration, string(body)+"\n").
				ServeHTTP(jsonTimeoutWriter{w}, r)
		})
	}
}

type jsonTimeoutWriter struct {
	http.ResponseWriter
}

func (w jsonTimeoutWriter) WriteHeader(status int) {
	if status == http.StatusServiceUnavailable {
		// TimeoutHandler uses text/html even for a supplied JSON error body.
		w.Header().Set("Content-Type", "application/json")
	}
	w.ResponseWriter.WriteHeader(status)
}

func limitBody(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestLimit := limit
			if githubWebhookRoute(r) && requestLimit > githubwebhooks.MaxBodyBytes {
				requestLimit = githubwebhooks.MaxBodyBytes
			}
			if r.ContentLength > requestLimit {
				writeError(
					w,
					r,
					http.StatusRequestEntityTooLarge,
					"body_too_large",
					"The request body exceeds the configured limit.",
				)
				return
			}
			if r.Body != nil && r.Body != http.NoBody {
				// Reading up to the cap also checks chunked bodies and handlers which
				// do not consume a body. Allocation is bounded by the configured cap.
				body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, requestLimit))
				if err != nil {
					var tooLarge *http.MaxBytesError
					if errors.As(err, &tooLarge) {
						writeError(
							w,
							r,
							http.StatusRequestEntityTooLarge,
							"body_too_large",
							"The request body exceeds the configured limit.",
						)
					} else {
						writeError(
							w,
							r,
							http.StatusBadRequest,
							"invalid_body",
							"The request body could not be read.",
						)
					}
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(body))
			}
			next.ServeHTTP(w, r)
		})
	}
}
