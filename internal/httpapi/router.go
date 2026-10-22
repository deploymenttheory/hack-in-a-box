// Package httpapi adapts HTTP requests to application operations.
// Business rules and database access belong in domain packages, not handlers.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi/v5"
	validator "github.com/oapi-codegen/nethttp-middleware"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

func NewRouter(cfg config.Config, logger *slog.Logger, dependencies ...Dependencies) *chi.Mux {
	var deps Dependencies
	if len(dependencies) > 0 {
		deps = dependencies[0]
	}
	metrics := new(measurements)
	router := chi.NewRouter()
	router.Use(
		identify,
		metrics.observe(logger),
		recoverPanics(logger),
		finiteOrDisplayTimeout(cfg.RequestTimeout),
		limitBody(cfg.MaxBodyBytes),
	)
	if deps.Auth != nil {
		deps.Auth.Sessions.ErrorFunc = func(w http.ResponseWriter, r *http.Request, _ error) {
			writeError(w, r, 503, "unavailable", "Session storage is temporarily unavailable.")
		}
		router.Use(func(next http.Handler) http.Handler {
			session := deps.Auth.Sessions.LoadAndSave(next)
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !displayRoute(r) && (strings.HasPrefix(r.URL.Path, "/api/") ||
					strings.HasPrefix(r.URL.Path, "/auth/")) {
					// TimeoutHandler buffers headers separately from identify's
					// writer. Preserve no-store when SCS adds its cookie directive.
					w.Header().Set("Cache-Control", "no-store")
					session.ServeHTTP(w, r)
				} else {
					next.ServeHTTP(w, r)
				}
			})
		})
	}
	router.NotFound(assets(cfg.WebDir))
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeError(
			w,
			r,
			http.StatusMethodNotAllowed,
			"method_not_allowed",
			"This method is not supported for this resource.",
		)
	})
	spec, err := contract.GetSpec()
	if err != nil {
		panic("invalid embedded OpenAPI contract")
	}
	validate := validator.OapiRequestValidatorWithOptions(spec, &validator.Options{
		Options: openapi3filter.Options{
			AuthenticationFunc: func(ctx context.Context, input *openapi3filter.AuthenticationInput) error {
				if input.SecuritySchemeName == "display" {
					if displayToken(ctx) == "" {
						return access.ErrUnauthenticated
					}
					return nil
				}
				if actor(ctx).ID.String() == "00000000-0000-0000-0000-000000000000" {
					return access.ErrUnauthenticated
				}
				return nil
			},
		},
		ErrorHandlerWithOpts: func(_ context.Context, _ error, w http.ResponseWriter, r *http.Request, _ validator.ErrorHandlerOpts) {
			writeError(w, r, 400, "invalid_request", "Check the request fields and try again.")
		},
	})
	protect := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if displayRoute(r) {
				token, err := displayAuthorization(r)
				if err != nil {
					respondError(w, r, err)
					return
				}
				next.ServeHTTP(
					w,
					r.WithContext(context.WithValue(r.Context(), displayTokenKey{}, token)),
				)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/api/") {
				if deps.Auth == nil {
					respondError(w, r, access.ErrUnauthenticated)
					return
				}
				person, err := deps.Auth.Current(r.Context())
				if err != nil {
					respondError(w, r, err)
					return
				}
				if r.Method != http.MethodGet && r.Method != http.MethodHead {
					if err := deps.Auth.CSRF(r); err != nil {
						respondError(w, r, err)
						return
					}
				}
				r = r.WithContext(context.WithValue(r.Context(), personKey{}, person))
			}
			next.ServeHTTP(w, r)
		})
	}
	strict := contract.NewStrictHandlerWithOptions(
		handlers{deps},
		nil,
		contract.StrictHTTPServerOptions{
			RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, _ error) {
				writeError(w, r, 400, "invalid_request", "Check the request fields and try again.")
			},
			ResponseErrorHandlerFunc: respondError,
		},
	)
	contract.HandlerWithOptions(
		strict,
		contract.ChiServerOptions{
			BaseRouter:  router,
			Middlewares: []contract.MiddlewareFunc{validate, protect},
			ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, _ error) {
				writeError(w, r, 400, "invalid_request", "Check the request fields and try again.")
			},
		},
	)
	router.Get("/ops/metrics", metrics.serve(deps.Pool, cfg.MetricsToken))
	if deps.Auth != nil {
		if deps.GitHub != nil {
			router.Get("/auth/github/callback", githubCallback(deps))
		}
		router.Get("/auth/login", func(w http.ResponseWriter, r *http.Request) {
			if err := deps.Auth.Login(w, r); err != nil {
				respondError(w, r, err)
			}
		})
		router.Get("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
			if err := deps.Auth.Callback(w, r); err != nil {
				respondError(w, r, err)
			}
		})
	}
	return router
}

func notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, http.StatusNotFound, "not_found", "The requested resource was not found.")
}
