package httpapi

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/felixge/httpsnoop"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

type measurements struct {
	mu               sync.Mutex
	requests, errors uint64
	seconds          float64
}

func (m *measurements) observe(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			measured := httpsnoop.CaptureMetrics(next, w, r)
			m.mu.Lock()
			m.requests++
			if measured.Code >= 500 {
				m.errors++
			}
			m.seconds += measured.Duration.Seconds()
			m.mu.Unlock()
			// TimeoutHandler may return while routing still runs. Do not inspect
			// Chi's mutable routing context here; use a bounded request category.
			route := requestCategory(r.URL.Path)
			// Never log URL/query values, headers, bodies, cookies or provider errors.
			logger.Info(
				"request completed",
				"request_id",
				requestID(r),
				"route",
				route,
				"status",
				measured.Code,
				"duration_ms",
				measured.Duration.Milliseconds(),
			)
		})
	}
}

func requestCategory(path string) string {
	for _, prefix := range []string{"/api/", "/auth/", "/ops/", "/assets/"} {
		if strings.HasPrefix(path, prefix) {
			return strings.Trim(prefix, "/")
		}
	}
	if path == "/healthz" || path == "/readyz" {
		return "health"
	}
	return "page"
}

func (m *measurements) serve(pool *pgxpool.Pool, token string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		supplied := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" || subtle.ConstantTimeCompare([]byte(supplied), []byte(token)) != 1 ||
			!strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			writeError(w, r, 403, "forbidden", "Operational access is required.")
			return
		}
		if pool == nil {
			writeError(w, r, 503, "unavailable", "Metrics are temporarily unavailable.")
			return
		}
		job, err := dbgen.New(pool).JobMetrics(r.Context())
		if err != nil {
			writeError(w, r, 503, "unavailable", "Metrics are temporarily unavailable.")
			return
		}
		m.mu.Lock()
		requests, failures, seconds := m.requests, m.errors, m.seconds
		m.mu.Unlock()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = fmt.Fprintf(
			w,
			"# TYPE hackbox_http_requests_total counter\nhackbox_http_requests_total %d\n# TYPE hackbox_http_errors_total counter\nhackbox_http_errors_total %d\n# TYPE hackbox_http_duration_seconds summary\nhackbox_http_duration_seconds_sum %g\nhackbox_http_duration_seconds_count %d\n# TYPE hackbox_due_job_lag_seconds gauge\nhackbox_due_job_lag_seconds %g\n# TYPE hackbox_failed_deliveries gauge\nhackbox_failed_deliveries %d\n# TYPE hackbox_ambiguous_deliveries gauge\nhackbox_ambiguous_deliveries %d\n# TYPE hackbox_recovered_leases_total counter\nhackbox_recovered_leases_total %d\n",
			requests,
			failures,
			seconds,
			requests,
			job.DueLagSeconds,
			job.Failed,
			job.Ambiguous,
			job.RecoveredLeases,
		)
	}
}

type personKey struct{}

func actor(ctx context.Context) dbgen.Person {
	person, _ := ctx.Value(personKey{}).(dbgen.Person)
	return person
}
