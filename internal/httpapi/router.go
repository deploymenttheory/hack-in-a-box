// Package httpapi adapts HTTP requests to application operations.
// Business rules and database access belong in domain packages, not handlers.
package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
)

func NewRouter(cfg config.Config, logger *slog.Logger) *chi.Mux {
	router := chi.NewRouter()
	router.Use(
		identify,
		recoverPanics(logger),
		timeout(cfg.RequestTimeout),
		limitBody(cfg.MaxBodyBytes),
	)
	router.NotFound(notFound)
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeError(
			w,
			r,
			http.StatusMethodNotAllowed,
			"method_not_allowed",
			"This method is not supported for this resource.",
		)
	})
	router.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, struct {
			Status string `json:"status"`
		}{Status: "ok"})
	})
	router.Route("/api/v1", func(api chi.Router) {
		// Domain endpoints and their authorization are introduced through #25.
		api.NotFound(notFound)
	})
	return router
}

func notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, http.StatusNotFound, "not_found", "The requested resource was not found.")
}
