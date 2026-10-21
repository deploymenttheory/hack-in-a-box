// Package httpserver owns listener configuration and graceful HTTP shutdown.
package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
)

func New(cfg config.Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.RequestTimeout + 5*time.Second,
		WriteTimeout:      cfg.RequestTimeout + 10*time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}

// Serve takes ownership of listener. Cancellation stops new connections while
// existing requests retain their contexts until completion or the drain deadline.
func Serve(
	ctx context.Context,
	server *http.Server,
	listener net.Listener,
	grace time.Duration,
	logger *slog.Logger,
) error {
	requestContext, cancelRequests := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelRequests()
	server.BaseContext = func(net.Listener) context.Context { return requestContext }
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.Serve(listener) }()
	select {
	case err := <-serveResult:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	case <-ctx.Done():
		logger.Info("HTTP shutdown started")
	}
	shutdownContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), grace)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		cancelRequests()
		closeErr := server.Close()
		return fmt.Errorf("HTTP shutdown exceeded its deadline: %w", errors.Join(err, closeErr))
	}
	if err := <-serveResult; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("stop HTTP: %w", err)
	}
	logger.Info("HTTP shutdown complete")
	return nil
}
