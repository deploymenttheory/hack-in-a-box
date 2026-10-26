package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpserver"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/recovery"
)

// TestLoad123SetupProcess is ONLY the private historical preparation process.
// Every domain mutation still traverses the real native owner and authority
// graph. It is stopped before normal production images run the measured load;
// the production executables cannot select this clock or this entrypoint.
func TestLoad123SetupProcess(t *testing.T) {
	if os.Getenv("HACKBOX_LOAD_SETUP") != "synthetic-only" {
		t.Skip("explicit private load preparation process only")
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		t.Fatal("private load setup configuration invalid")
	}
	dsn, err := url.Parse(cfg.DatabaseURL)
	if err != nil || dsn.Hostname() != "postgres" || dsn.Path != "/load123" ||
		cfg.Environment != "production" || cfg.OIDCIssuer != "https://issuer.load.test:8445" {
		t.Fatal(
			"private load setup requires isolated database, verified issuer and secure native cookies",
		)
	}
	clock, err := load123SetupClock(os.Getenv("HACKBOX_LOAD_SETUP_CLOCK"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(t.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runLoad123Setup(ctx, cfg, clock, os.Getenv("HACKBOX_LOAD_SETUP_ROLE")); err != nil {
		t.Fatal("private load setup process failed")
	}
}

func runLoad123Setup(
	ctx context.Context,
	cfg config.Config,
	clock func() time.Time,
	role string,
) error {
	if role != "public" && role != "admin" && role != "worker" {
		return errors.New("explicit private load setup role required")
	}
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := database.CheckSchema(ctx, pool); err != nil {
		return err
	}
	lease, err := recovery.Runtime(ctx, pool)
	if err != nil {
		return err
	}
	defer func() { _ = lease.Close(context.WithoutCancel(ctx)) }()
	storage, err := files.OpenLocal(cfg.StoragePath)
	if err != nil {
		return err
	}
	defer func() { _ = storage.Close() }()
	if err := storage.Ready(ctx); err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	if role == "worker" {
		if err := storage.Cleanup(ctx, dbgen.New(pool).ObjectExists); err != nil {
			return err
		}
		runtime, err := newWorkerRuntime(pool, cfg, logger, storage, clock)
		if err != nil {
			return err
		}
		return runtime.Jobs.Run(ctx, cfg.ShutdownTimeout)
	}

	auth, err := identity.New(ctx, pool, identity.Settings{
		CookiePrefix: "hackbox_" + cfg.Surface, Issuer: cfg.OIDCIssuer,
		ClientID: cfg.OIDCClientID, ClientSecret: cfg.OIDCClientSecret,
		Origin: cfg.PublicOrigin, BootstrapSubject: cfg.BootstrapSubject, Secure: true,
	})
	if err != nil {
		return err
	}
	dependencies, err := newAPIDependencies(ctx, pool, cfg, logger, storage, auth, clock)
	if err != nil {
		return err
	}
	server := httpserver.New(cfg, httpapi.NewRouter(cfg, logger, dependencies))
	var listen net.ListenConfig
	listener, err := listen.Listen(ctx, "tcp", cfg.HTTPAddr)
	if err != nil {
		return err
	}
	return httpserver.Serve(ctx, server, listener, cfg.ShutdownTimeout, logger)
}

func load123SetupClock(path string) (func() time.Time, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("owner-private preparation clock path must be absolute")
	}
	read := func() (time.Time, error) {
		file, err := os.Open(path)
		if err != nil {
			return time.Time{}, errors.New("private preparation clock unavailable")
		}
		defer func() { _ = file.Close() }()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 ||
			info.Size() > 64 {
			return time.Time{}, errors.New(
				"private preparation clock must be bounded and owner-only",
			)
		}
		raw, err := io.ReadAll(io.LimitReader(file, 65))
		if err != nil || len(raw) > 64 {
			return time.Time{}, errors.New("private preparation clock read failed")
		}
		value, err := time.Parse(time.RFC3339Nano, string(raw))
		if err != nil || value.IsZero() {
			return time.Time{}, errors.New("private preparation clock invalid")
		}
		return value.UTC(), nil
	}
	if _, err := read(); err != nil {
		return nil, err
	}
	return func() time.Time {
		value, err := read()
		if err != nil {
			// A broken owner control must stop setup instead of silently using
			// wall time and manufacturing a different lifecycle history.
			panic("private load preparation clock lost")
		}
		return value
	}, nil
}
