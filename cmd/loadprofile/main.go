// Command loadprofile drives the private release fixture through normal HTTPS
// APIs and separately reads a scoped, read-only PostgreSQL observer connection.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"

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
	raw, err := loadprofile.ReadPrivateInput(*preparedPath, loadprofile.MaximumPreparedBytes)
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
	targets, err := loadprofile.ReadPrivateStringMap(*targetPath)
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
	containers, err := loadprofile.ReadPrivateStringMap(*containerPath)
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
	pool, err := loadprofile.OpenReadOnlyObserver(ctx, os.Getenv("HACKBOX_LOAD_OBSERVER_DSN"))
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
