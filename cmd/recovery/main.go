// recovery is a deployment-operator tool. It has no public HTTP surface.
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

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/recovery"
)

var errArguments = errors.New(
	"usage: recovery -action status|hold|resume [-hold UUID -review FINGERPRINT]; configure HACKBOX_DATABASE_URL separately",
)

func main() { os.Exit(exitCode()) }

func exitCode() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("recovery", flag.ContinueOnError)
	action := flags.String("action", "status", "operator action")
	hold := flags.String("hold", "", "exact hold UUID from current status")
	review := flags.String("review", "", "exact reviewed current fingerprint")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse recovery arguments: %w", err)
	}
	if flags.NArg() != 0 || (*action != "status" && *action != "hold" && *action != "resume") {
		return errArguments
	}
	var id uuid.UUID
	if *action == "resume" {
		var err error
		id, err = uuid.Parse(*hold)
		if err != nil || len(*review) != 64 {
			return errArguments
		}
	}
	pool, err := database.Open(ctx, os.Getenv("HACKBOX_DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("open recovery database: %w", err)
	}
	defer pool.Close()
	if err = database.CheckSchema(ctx, pool); err != nil {
		return fmt.Errorf("check recovery schema: %w", err)
	}
	var status recovery.Status
	switch *action {
	case "hold":
		status, err = recovery.Hold(ctx, pool)
	case "status":
		status, err = recovery.Read(ctx, pool)
	case "resume":
		err = recovery.Resume(ctx, pool, id, *review)
		if err == nil {
			status, err = recovery.Read(ctx, pool)
		}
	}
	if err != nil {
		return fmt.Errorf("recovery %s: %w", *action, err)
	}
	if err = json.NewEncoder(os.Stdout).Encode(status); err != nil {
		return fmt.Errorf("write recovery status: %w", err)
	}
	return nil
}
