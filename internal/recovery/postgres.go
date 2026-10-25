package recovery

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
)

var ErrPostgresTool = errors.New(
	"PostgreSQL recovery client failed; verify client version, connection and target readiness",
)

// PostgresClient uses the pinned PostgreSQL client image or separately installed
// matching client tools. The connection is passed through libpq's environment,
// never command arguments or diagnostic output. Raw client stderr is suppressed.
type PostgresClient struct{ Connection string }

func (p PostgresClient) Dump(ctx context.Context, output io.Writer) error {
	return p.run(
		ctx,
		"pg_dump",
		nil,
		output,
		"--no-password",
		"--no-owner",
		"--no-privileges",
		"--format=plain",
		"--encoding=UTF8",
	)
}

func (p PostgresClient) Restore(ctx context.Context, input io.Reader) error {
	return p.run(
		ctx,
		"psql",
		input,
		io.Discard,
		"--no-password",
		"--no-psqlrc",
		"--set=ON_ERROR_STOP=1",
		"--single-transaction",
	)
}

func (p PostgresClient) run(
	ctx context.Context,
	program string,
	input io.Reader,
	output io.Writer,
	args ...string,
) error {
	command := exec.CommandContext(ctx, program, args...)
	command.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"LANG=C.UTF-8",
		"PGDATABASE=" + p.Connection,
		"PGAPPNAME=hackbox_recovery",
	}
	command.Stdin = input
	command.Stdout = output
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("PostgreSQL client cancelled: %w", ctx.Err())
		}
		return ErrPostgresTool
	}
	return nil
}
