// Styleguide serves the separately built public component documentation. It never
// loads application configuration or connects to a database/identity provider.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/styleguide"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		logger.Error("style guide stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	directory := os.Getenv("HACKBOX_WEB_DIR")
	if directory == "" {
		directory = "/app/web"
	}
	address := os.Getenv("HACKBOX_HTTP_ADDR")
	if address == "" {
		address = "0.0.0.0:8080"
	}
	handler, err := styleguide.Open(directory)
	if err != nil {
		return err
	}
	defer func() { _ = handler.Close() }()
	server := &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       time.Minute,
	}
	stopped := make(chan error, 1)
	go func() { stopped <- server.ListenAndServe() }()
	select {
	case err := <-stopped:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve static style guide: %w", err)
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			return fmt.Errorf("stop static style guide: %w", err)
		}
		return nil
	}
}
