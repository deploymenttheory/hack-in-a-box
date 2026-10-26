// Command loadfixture creates private external-fixture inputs. It never seeds
// native application identities, sessions, teams, completed events or jobs.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
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
	knownPrefix := flag.String(
		"known-release-prefix",
		"",
		"ONE strictly recognised read-only failed release prefix; no domain replay",
	)
	release := flag.String(
		"release-from-sources",
		"",
		"private original source receipt for actual worker release/capture",
	)
	setupProject := flag.String(
		"setup-project",
		"",
		"exact owned hackbox-load123 project for private clock controls",
	)
	setupImage := flag.String(
		"setup-image",
		"",
		"immutable private fixture image SHA256 for clock controls",
	)
	setupSource := flag.String(
		"setup-source",
		"",
		"exact source SHA of private fixture clock controller",
	)
	sources := flag.String(
		"sources-from-admission",
		"",
		"continue source preparation from this private original admission receipt",
	)
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
	phase := nativePreparationPhase{
		Admission: *sources, KnownPrefix: *knownPrefix,
		Sources: *release,
		Clock: loadprofile.DockerSetupClock{
			Project: *setupProject,
			Image:   *setupImage,
			Source:  *setupSource,
		},
	}
	if flag.NArg() != 0 || modes != 1 || !phase.valid(*admit != "") ||
		(*sources != "" && *admit == "") ||
		(*advance == "" && (*expected != "" || *next != "")) {
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
		if err := admitNative(*admit, *ca, *targets, *output, phase); err != nil {
			fmt.Fprintln(
				os.Stderr,
				"native preparation failed; inspect the private checkpoint receipt",
			)
			os.Exit(1)
		}
		fmt.Println(
			"Native preparation checkpoint recorded. Full lifecycle/export/load acceptance remains pending.",
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

func admitNative(
	planPath, caPath, targetsPath, outputPath string,
	phase nativePreparationPhase,
) error {
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
	origins := map[loadprofile.Surface]string{
		loadprofile.PublicSurface: "https://public.load.test:8443",
		loadprofile.AdminSurface:  "https://admin.load.test:8444",
	}
	var receipt any
	var runErr error
	switch {
	case phase.Sources != "":
		receipt, runErr = releaseNativeSources(ctx, plan, phase, origins, transport, database)
	case phase.Admission == "":
		receipt, runErr = loadprofile.PrepareFactoryAdmission(
			ctx,
			plan,
			origins,
			"https://issuer.load.test:8445",
			transport,
			database,
		)
	default:
		raw, err := loadprofile.ReadPrivateInput(phase.Admission, 8<<20)
		if err != nil {
			return fmt.Errorf("read private original admission checkpoint: %w", err)
		}
		original, err := loadprofile.ParseAdmissionCheckpoint(raw)
		if err != nil {
			return fmt.Errorf("validate original native admission checkpoint: %w", err)
		}
		prepared, err := loadprofile.PrepareFactorySources(
			ctx,
			plan,
			original,
			origins,
			"https://issuer.load.test:8445",
			transport,
			database,
		)
		digest := sha256.Sum256(raw)
		prepared.OriginalAdmissionSHA256 = hex.EncodeToString(digest[:])
		receipt, runErr = prepared, err
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return errors.Join(runErr, encoder.Encode(receipt), output.Sync())
}

var errSetupCommand = errors.New("invalid private setup command")

type nativePreparationPhase struct {
	KnownPrefix string
	Admission   string
	Sources     string
	Clock       loadprofile.DockerSetupClock
}

func (p nativePreparationPhase) valid(native bool) bool {
	if p.KnownPrefix != "" && p.Sources == "" {
		return false
	}
	if !native {
		return p.Admission == "" && p.Sources == "" && p.Clock.Project == "" &&
			p.Clock.Image == "" &&
			p.Clock.Source == ""
	}
	if p.Sources != "" {
		return p.Admission == "" && p.Clock.Project != "" && p.Clock.Image != "" &&
			p.Clock.Source != ""
	}
	return p.Clock.Project == "" && p.Clock.Image == "" && p.Clock.Source == ""
}

func releaseNativeSources(
	ctx context.Context,
	plan loadprofile.FactoryPlan,
	phase nativePreparationPhase,
	origins map[loadprofile.Surface]string,
	transport http.RoundTripper,
	database loadprofile.NativeDatabase,
) (any, error) {
	raw, err := loadprofile.ReadPrivateInput(phase.Sources, 8<<20)
	if err != nil {
		return nil, fmt.Errorf("read private original source checkpoint: %w", err)
	}
	var prefix []byte
	if phase.KnownPrefix != "" {
		prefix, err = loadprofile.ReadPrivateInput(phase.KnownPrefix, 8<<20)
		if err != nil {
			return nil, fmt.Errorf("read known original release prefix: %w", err)
		}
	}
	prepared, err := loadprofile.PrepareFactoryRelease(
		ctx,
		loadprofile.NativeReleaseInputs{
			Plan:            plan,
			SourcesJSON:     raw,
			KnownPrefixJSON: prefix,
			Clock:           &phase.Clock,
		},
		origins,
		"https://issuer.load.test:8445",
		transport,
		database,
	)
	if err != nil {
		return prepared, fmt.Errorf("native release/capture setup incomplete: %w", err)
	}
	return prepared, nil
}
