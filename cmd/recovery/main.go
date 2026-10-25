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

	"filippo.io/age"
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
	bundle := flags.String("bundle", "", "private encrypted archive directory")
	files := flags.String("files", "", "native storage directory (fresh destination for restore)")
	recipientFile := flags.String("recipients", "", "archive public-recipient file")
	identityFile := flags.String("identity", "", "separately held archive identity file")
	hold := flags.String("hold", "", "exact hold UUID from current status")
	review := flags.String("review", "", "exact reviewed current fingerprint")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse recovery arguments: %w", err)
	}
	if flags.NArg() != 0 ||
		(*action != "status" && *action != "hold" && *action != "resume" && *action != "backup" && *action != "verify" && *action != "restore") {
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
	if *action == "verify" {
		identities, err := readIdentities(*identityFile)
		if err != nil {
			return err
		}
		manifest, err := recovery.Verify(ctx, *bundle, identities)
		if err != nil {
			return fmt.Errorf("verify archive: %w", err)
		}
		return emit(manifest)
	}
	pool, err := database.Open(ctx, os.Getenv("HACKBOX_DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("open recovery database: %w", err)
	}
	defer pool.Close()
	if *action != "restore" {
		if err = database.CheckSchema(ctx, pool); err != nil {
			return fmt.Errorf("check recovery schema: %w", err)
		}
	}
	client := recovery.PostgresClient{Connection: os.Getenv("HACKBOX_DATABASE_URL")}
	if *action == "backup" {
		recipients, err := readRecipients(*recipientFile)
		if err != nil {
			return err
		}
		result, err := recovery.Capture(ctx, pool, *files, *bundle, recipients, client.Dump)
		if err != nil {
			return fmt.Errorf("capture archive: %w", err)
		}
		return emit(result)
	}
	if *action == "restore" {
		identities, err := readIdentities(*identityFile)
		if err != nil {
			return err
		}
		result, err := recovery.Restore(ctx, pool, *bundle, *files, identities, client.Restore)
		if err != nil {
			return fmt.Errorf("restore archive: %w", err)
		}
		return emit(result)
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
	return emit(status)
}

func emit(value any) error {
	if err := json.NewEncoder(os.Stdout).Encode(value); err != nil {
		return fmt.Errorf("write recovery result: %w", err)
	}
	return nil
}

func readRecipients(path string) ([]age.Recipient, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, recovery.ErrArchive
	}
	defer func() { _ = file.Close() }()
	recipients, err := age.ParseRecipients(file)
	if err != nil {
		return nil, recovery.ErrArchive
	}
	return recipients, nil
}

func readIdentities(path string) ([]age.Identity, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, recovery.ErrArchive
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return nil, recovery.ErrArchive
	}
	identities, err := age.ParseIdentities(file)
	if err != nil {
		return nil, recovery.ErrArchive
	}
	return identities, nil
}
