// Command loadfixture creates private external-fixture inputs. It never seeds
// native application identities, sessions, teams, completed events or jobs.
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

func main() {
	directory := flag.String("new-directory", "", "NEW absolute private fixture input directory")
	advance := flag.String(
		"advance-setup-directory", "",
		"private setup volume directory containing domain-time; never a production clock",
	)
	expected := flag.String("expected-time", "", "exact existing setup domain time, RFC3339Nano")
	next := flag.String("next-time", "", "strictly later setup domain time, RFC3339Nano")
	admit := flag.String("admit-plan", "", "private canonical plan for native admission checkpoint")
	ca := flag.String("ca", "", "fixture CA PEM for native admission")
	targets := flag.String("targets", "", "private exact loopback target map for native admission")
	output := flag.String("output", "", "new private native admission receipt")
	flag.Parse()
	modes := 0
	for _, value := range []string{*directory, *admit, *advance} {
		if value != "" {
			modes++
		}
	}
	if flag.NArg() != 0 || modes != 1 || (*advance == "" && (*expected != "" || *next != "")) {
		fmt.Fprintln(
			os.Stderr,
			"select one private input, native admission or explicit setup clock operation",
		)
		os.Exit(1)
	}
	if *advance != "" {
		previous, parseErr := time.Parse(time.RFC3339Nano, *expected)
		following, nextErr := time.Parse(time.RFC3339Nano, *next)
		advanceErr := errors.Join(parseErr, nextErr)
		if advanceErr == nil {
			advanceErr = loadprofile.AdvanceSetupClock(*advance, previous, following)
		}
		if advanceErr != nil {
			fmt.Fprintln(
				os.Stderr,
				"private setup clock advance rejected; expected phase and private control required",
			)
			os.Exit(1)
		}
		fmt.Println(
			"Private setup clock advanced. Native worker release/capture remains to be observed; no runtime load evidence created.",
		)
		return
	}
	if *admit != "" {
		if err := admitNative(*admit, *ca, *targets, *output); err != nil {
			fmt.Fprintln(
				os.Stderr,
				"native admission failed; inspect the private checkpoint receipt",
			)
			os.Exit(1)
		}
		fmt.Println(
			"Native admission checkpoint recorded. Full lifecycle/export/load acceptance remains pending.",
		)
		return
	}
	plan, err := loadprofile.NewReleaseFactoryPlan(uuid.New(), time.Now())
	if err == nil {
		err = loadprofile.WriteFactoryInputs(*directory, plan)
	}
	if err != nil {
		fmt.Fprintln(
			os.Stderr,
			"private external-fixture input generation failed; existing inputs are never overwritten",
		)
		os.Exit(1)
	}
	fmt.Println(
		"Private external fixture inputs created. No native application facts or load acceptance evidence created.",
	)
}

func admitNative(planPath, caPath, targetsPath, outputPath string) error {
	raw, err := loadprofile.ReadPrivateInput(planPath, loadprofile.MaximumPreparedBytes)
	if err != nil {
		return fmt.Errorf("read explicit setup plan: %w", err)
	}
	plan, err := loadprofile.ParseFactoryPlan(raw)
	if err != nil {
		return fmt.Errorf("validate canonical setup plan: %w", err)
	}
	ca, err := loadprofile.ReadPrivateInput(caPath, 1<<20)
	if err != nil {
		return fmt.Errorf("read fixture CA: %w", err)
	}
	targets, err := loadprofile.ReadPrivateStringMap(targetsPath)
	if err != nil {
		return fmt.Errorf("read exact TCP targets: %w", err)
	}
	if len(targets) != 3 || targets["public.load.test:8443"] == "" ||
		targets["admin.load.test:8444"] == "" || targets["issuer.load.test:8445"] == "" {
		return fmt.Errorf("%w: exact three private front doors required", errSetupCommand)
	}
	transport, err := loadprofile.NewVerifiedTransport(ca, targets)
	if err != nil {
		return fmt.Errorf("create verified front-door transport: %w", err)
	}
	defer transport.CloseIdleConnections()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := loadprofile.OpenReadOnlyObserver(ctx, os.Getenv("HACKBOX_LOAD_OBSERVER_DSN"))
	if err != nil {
		return fmt.Errorf("open private read-only observer: %w", err)
	}
	defer pool.Close()
	eventIDs := make([]uuid.UUID, len(plan.Events))
	for index, item := range plan.Events {
		eventIDs[index] = item.Input.ID
	}
	database, err := loadprofile.NewNativeDatabase(pool, eventIDs)
	if err != nil {
		return fmt.Errorf("scope observer to native setup events: %w", err)
	}
	output, err := os.OpenFile(outputPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("reserve NEW private setup receipt: %w", err)
	}
	defer func() { _ = output.Close() }()
	receipt, runErr := loadprofile.PrepareFactoryAdmission(
		ctx,
		plan,
		map[loadprofile.Surface]string{
			loadprofile.PublicSurface: "https://public.load.test:8443",
			loadprofile.AdminSurface:  "https://admin.load.test:8444",
		},
		"https://issuer.load.test:8445",
		transport,
		database,
	)
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return errors.Join(runErr, encoder.Encode(receipt), output.Sync())
}

var errSetupCommand = errors.New("invalid private setup command")
