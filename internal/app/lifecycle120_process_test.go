package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
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

// This entrypoint exists only in the explicitly built lifecycle test binary.
// Ordinary app/worker executables contain neither these controls nor a clock
// configuration. Compose starts distinct instances for each native process.
func TestLifecycleComposeProcess(t *testing.T) {
	role := os.Getenv("HACKBOX_LIFECYCLE_ROLE")
	if role == "" {
		t.Skip("run the isolated #120 lifecycle Compose profile")
	}
	if os.Getenv("HACKBOX_LIFECYCLE_ISOLATED") != "synthetic-only" {
		t.Fatal("the lifecycle fixture requires an explicit isolated synthetic project")
	}
	transport, err := newLifecycleProviderTransport()
	if err != nil {
		t.Fatal(err)
	}
	previous := http.DefaultTransport
	http.DefaultTransport = transport
	defer func() {
		http.DefaultTransport = previous
		transport.base.CloseIdleConnections()
	}()
	ctx, stop := signal.NotifyContext(t.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if role == "browser-proxy" {
		err = runLifecycleBrowserProxy(ctx)
	} else if role == "provider" {
		err = runLifecycleProvider(ctx)
	} else if role == "public" || role == "admin" || role == "worker" {
		err = runLifecycleNativeProcess(ctx, role)
	} else {
		err = errors.New("unknown lifecycle process role")
	}
	if err != nil {
		t.Fatal(err)
	}
}

func runLifecycleNativeProcess(ctx context.Context, role string) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	dsn, err := url.Parse(cfg.DatabaseURL)
	if err != nil || dsn.Hostname() != "postgres" || dsn.Path != "/lifecycle" ||
		cfg.Environment != "test" ||
		cfg.OIDCIssuer == "" {
		return errors.New(
			"lifecycle fixture must use its isolated Compose database and configured OIDC",
		)
	}
	clock, err := newLifecycleFileClock(os.Getenv("HACKBOX_LIFECYCLE_CONTROL"))
	if err != nil {
		return err
	}
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err = database.CheckSchema(ctx, pool); err != nil {
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
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil)).With("fixture_role", role)
	if role == "worker" {
		if err := storage.Cleanup(ctx, dbgen.New(pool).ObjectExists); err != nil {
			return err
		}
		runtime, err := newWorkerRuntime(pool, cfg, logger, storage, clock.read)
		if err != nil {
			return err
		}
		return runtime.Jobs.Run(ctx, cfg.ShutdownTimeout)
	}
	if err := storage.Ready(ctx); err != nil {
		return err
	}
	auth, err := identity.New(ctx, pool, identity.Settings{
		CookiePrefix: "hackbox_" + cfg.Surface, Issuer: cfg.OIDCIssuer,
		ClientID: cfg.OIDCClientID, ClientSecret: cfg.OIDCClientSecret,
		Origin: cfg.PublicOrigin, BootstrapSubject: cfg.BootstrapSubject,
		Secure: cfg.Environment == "production",
	})
	if err != nil {
		return err
	}
	deps, err := newAPIDependencies(ctx, pool, cfg, logger, storage, auth, clock.read)
	if err != nil {
		return err
	}
	server := httpserver.New(cfg, httpapi.NewRouter(cfg, logger, deps))
	var listen net.ListenConfig
	listener, err := listen.Listen(ctx, "tcp", cfg.HTTPAddr)
	if err != nil {
		return err
	}
	return httpserver.Serve(ctx, server, listener, cfg.ShutdownTimeout, logger)
}

type lifecycleFileClock struct {
	path string
	mu   sync.Mutex
	last time.Time
}

func newLifecycleFileClock(directory string) (*lifecycleFileClock, error) {
	if directory == "" || !filepath.IsAbs(directory) {
		return nil, errors.New("owner-private lifecycle control directory is required")
	}
	clock := &lifecycleFileClock{path: filepath.Join(directory, "domain-time")}
	if _, err := clock.load(); err != nil {
		return nil, err
	}
	return clock, nil
}

func (c *lifecycleFileClock) load() (time.Time, error) {
	raw, err := os.ReadFile(c.path)
	if err != nil || len(raw) > 64 {
		return time.Time{}, errors.New("isolated lifecycle clock is unavailable")
	}
	value, err := time.Parse(time.RFC3339Nano, string(raw))
	if err != nil {
		return time.Time{}, errors.New("isolated lifecycle clock is invalid")
	}
	return value.UTC(), nil
}

func (c *lifecycleFileClock) read() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	value, err := c.load()
	if err != nil || value.Before(c.last) {
		// A corrupt/backwards fixture clock must fail the rehearsal, never fall
		// through to real time and accidentally permit a different domain action.
		panic("invalid or backwards isolated lifecycle domain clock")
	}
	c.last = value
	return value
}

func runLifecycleProvider(ctx context.Context) error {
	directory := os.Getenv("HACKBOX_LIFECYCLE_CONTROL")
	if directory == "" || !filepath.IsAbs(directory) {
		return errors.New("owner-private lifecycle control directory is required")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(directory, "domain-time")); !errors.Is(err, os.ErrNotExist) {
		return errors.New(
			"lifecycle provider requires a fresh isolated control volume; refusing to reset domain time",
		)
	}
	timeline, err := newLifecycleTimeline(time.Now().UTC())
	if err != nil {
		return err
	}
	if err = writeLifecycleClock(directory, timeline.Initial); err != nil {
		return err
	}
	relay, err := newLifecycleRelay(ctx, directory)
	if err != nil {
		return err
	}
	defer func() { _ = relay.close() }()
	actors := lifecycleDefaultGitHubActors()
	actors["mentor-a"] = 37
	actors["mentor-b"] = 38
	github, err := newLifecycleGitHubForActors(directory, actors)
	if err != nil {
		return err
	}
	provider, err := newLifecycleOIDC("http://provider.localhost:8080", map[string]string{
		"lifecycle-public": "http://public.localhost:8080/auth/callback",
		"lifecycle-admin":  "http://admin.localhost:8080/auth/callback",
	})
	if err != nil {
		return err
	}
	clock, err := newLifecycleFileClock(directory)
	if err != nil {
		return err
	}
	socket := filepath.Join(directory, "control.sock")
	var listen net.ListenConfig
	listener, err := listen.Listen(ctx, "unix", socket)
	if err != nil {
		return err
	}
	if err = os.Chmod(socket, 0o600); err != nil {
		_ = listener.Close()
		return err
	}
	control := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/github/") {
				github.control(w, r)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/mail/") {
				relay.control(w, r)
				return
			}
			if r.URL.Path != "/clock" {
				http.NotFound(w, r)
				return
			}
			if r.Method == http.MethodGet {
				lifecycleJSON(w, map[string]any{"now": clock.read(), "timeline": timeline})
				return
			}
			var input struct {
				At time.Time `json:"at"`
			}
			if r.Method != http.MethodPost ||
				json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&input) != nil {
				http.Error(w, "invalid monotonic lifecycle phase", http.StatusBadRequest)
				return
			}
			if err := clock.advance(
				directory,
				input.At,
				// Cover the explicit120-day policy after the one-day postponement.
				// This bounds only the private fixture clock, not a production window.
				timeline.Day90.AddDate(0, 0, 31),
			); err != nil {
				http.Error(w, "clock fixture unavailable", http.StatusInternalServerError)
				return
			}
			lifecycleJSON(w, map[string]any{"now": clock.read()})
		}),
	}
	go func() { _ = control.Serve(listener) }()
	defer func() { _ = control.Close() }()
	defer func() { _ = os.Remove(socket) }()
	mux := http.NewServeMux()
	mux.Handle("/", provider)
	mux.Handle("/github/", github)
	mux.Handle("/github-api/", github)
	mux.HandleFunc(
		"/readyz",
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) },
	)
	server := &http.Server{Addr: "0.0.0.0:8080", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	network, err := listen.Listen(ctx, "tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("listen isolated provider: %w", err)
	}
	return httpserver.Serve(
		ctx,
		server,
		network,
		10*time.Second,
		slog.New(slog.NewJSONHandler(os.Stderr, nil)),
	)
}

func writeLifecycleClock(directory string, value time.Time) error {
	temporary := filepath.Join(directory, "domain-time.next")
	if err := os.WriteFile(
		temporary,
		[]byte(value.UTC().Format(time.RFC3339Nano)),
		0o600,
	); err != nil {
		return err
	}
	return os.Rename(temporary, filepath.Join(directory, "domain-time"))
}

func (c *lifecycleFileClock) advance(directory string, value, maximum time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	current, err := c.load()
	if err != nil || value.Before(current) || value.After(maximum) {
		return errors.New("clock advance is outside the monotonic scenario")
	}
	if err = writeLifecycleClock(directory, value); err != nil {
		return err
	}
	c.last = value
	return nil
}
