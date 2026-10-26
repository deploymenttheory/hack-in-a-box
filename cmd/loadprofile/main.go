// Command loadprofile drives the private release fixture through normal HTTPS
// APIs and separately reads a scoped, read-only PostgreSQL observer connection.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/loadprofile"
)

var errDriver = errors.New("invalid private load driver")

func main() {
	if err := run(); err != nil {
		// Provider responses, cookies, DSNs and private fixture bodies never
		// appear on stdout/stderr; the private receipt retains aggregate facts.
		fmt.Fprintln(
			os.Stderr,
			"load profile failed; inspect the private aggregate receipt and fixture configuration",
		)
		os.Exit(1)
	}
}

func run() error {
	preparedPath := flag.String("prepared", "", "private acknowledged native fixture export")
	caPath := flag.String("ca", "", "fixture root CA PEM")
	targetPath := flag.String(
		"targets",
		"",
		"private exact HTTPS host:port to loopback:port JSON map",
	)
	containerPath := flag.String(
		"containers",
		"",
		"private explicit service to full scoped Docker container ID JSON map",
	)
	outputPath := flag.String(
		"output",
		"",
		"new private aggregate evidence file (never overwritten)",
	)
	smokeDuration := flag.Duration(
		"smoke-duration",
		15*time.Second,
		"non-release smoke duration only",
	)
	smokeInterval := flag.Duration(
		"smoke-interval",
		time.Second,
		"non-release per-actor interval only",
	)
	flag.Parse()
	if flag.NArg() != 0 || *outputPath == "" {
		return fmt.Errorf("%w: explicit private input/output paths required", errDriver)
	}
	raw, err := privateInput(*preparedPath, loadprofile.MaximumPreparedBytes)
	if err != nil {
		return fmt.Errorf("load driver setup: %w", err)
	}
	fixture, err := loadprofile.ParsePreparedFixture(raw)
	if err != nil {
		return fmt.Errorf("load driver setup: %w", err)
	}
	if fixture.Cohort.Profile == "release" {
		var override bool
		flag.Visit(func(f *flag.Flag) {
			if f.Name == "smoke-duration" || f.Name == "smoke-interval" {
				override = true
			}
		})
		if override {
			return fmt.Errorf(
				"%w: release cannot override real stage durations using smoke flags",
				errDriver,
			)
		}
	}
	ca, err := os.ReadFile(*caPath)
	if err != nil || len(ca) > 1<<20 {
		return fmt.Errorf("%w: bounded fixture CA unavailable", errDriver)
	}
	targets, err := privateStringMap(*targetPath)
	if err != nil {
		return fmt.Errorf("load driver setup: %w", err)
	}
	expectedTargets, err := fixture.FixtureTargets()
	if err != nil || len(targets) != len(expectedTargets) {
		return fmt.Errorf(
			"%w: declared TCP targets must equal prepared external origins",
			errDriver,
		)
	}
	for _, target := range expectedTargets {
		if targets[target] == "" {
			return fmt.Errorf(
				"%w: prepared external origin has no explicit loopback TCP target",
				errDriver,
			)
		}
	}
	transport, err := loadprofile.NewVerifiedTransport(ca, targets)
	if err != nil {
		return fmt.Errorf("load driver setup: %w", err)
	}
	defer transport.CloseIdleConnections()
	containers, err := privateStringMap(*containerPath)
	if err != nil {
		return fmt.Errorf("load driver setup: %w", err)
	}
	config := loadprofile.ReleaseRuntimeConfig(containers)
	if fixture.Cohort.Profile == "smoke" {
		config.Duration, config.Interval = *smokeDuration, *smokeInterval
		config.Saturation = loadprofile.SaturationConfig{
			Profile: "smoke", Duration: 2 * time.Second,
			Intervals: []time.Duration{time.Second, 100 * time.Millisecond},
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := observerPool(ctx, os.Getenv("HACKBOX_LOAD_OBSERVER_DSN"))
	if err != nil {
		return fmt.Errorf("%w: private read-only loopback database observer unavailable", errDriver)
	}
	defer pool.Close()
	ids := make([]uuid.UUID, len(fixture.Events))
	for index, event := range fixture.Events {
		ids[index] = event.ID
	}
	database, err := loadprofile.NewNativeDatabase(pool, ids)
	if err != nil {
		return fmt.Errorf("load driver setup: %w", err)
	}
	// Reserve the path before starting any native traffic. A repeated command
	// must choose a new receipt instead of replacing evidence from an earlier run.
	output, err := os.OpenFile(*outputPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("%w: new private receipt path unavailable", errDriver)
	}
	defer func() { _ = output.Close() }()
	evidence, runErr := loadprofile.RunNativeProfile(ctx, fixture, config, transport, database)
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	writeErr := encoder.Encode(evidence)
	syncErr := output.Sync()
	// This command currently exports workload evidence, never an affirmative
	// release decision without the separate provider/privacy/follow-up oracles.
	return errors.Join(runErr, writeErr, syncErr)
}

func privateInput(path string, limit int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: private input unavailable", errDriver)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 ||
		info.Size() > int64(limit) {
		return nil, fmt.Errorf(
			"%w: private input must be a bounded owner-only regular file",
			errDriver,
		)
	}
	raw, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil || len(raw) > limit {
		return nil, fmt.Errorf("%w: bounded private input read failed", errDriver)
	}
	return raw, nil
}

func privateStringMap(path string) (map[string]string, error) {
	raw, err := privateInput(path, 64<<10)
	if err != nil {
		return nil, fmt.Errorf("load driver input or observer: %w", err)
	}
	var values map[string]string
	if json.Unmarshal(raw, &values) != nil || len(values) == 0 {
		return nil, fmt.Errorf("%w: explicit private string map required", errDriver)
	}
	return values, nil
}

func observerPool(ctx context.Context, raw string) (*pgxpool.Pool, error) {
	target, err := url.Parse(raw)
	if err != nil || (target.Scheme != "postgres" && target.Scheme != "postgresql") ||
		target.User == nil || target.Hostname() == "" || target.Port() == "" {
		return nil, fmt.Errorf("%w: explicit private loopback observer URL required", errDriver)
	}
	address := net.ParseIP(target.Hostname())
	if address == nil || !address.IsLoopback() {
		return nil, fmt.Errorf("%w: observer database must be explicitly loopback-bound", errDriver)
	}
	config, err := pgxpool.ParseConfig(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid private observer configuration", errDriver)
	}
	config.MaxConns = 4
	config.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	config.ConnConfig.RuntimeParams["application_name"] = "load123-read-only-observer"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("load driver input or observer: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("load driver input or observer: %w", err)
	}
	return pool, nil
}
