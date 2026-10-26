package loadprofile

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ReadPrivateInput accepts only bounded owner-only regular files. Callers never
// print their contents or parse credentials from command-line arguments.
func ReadPrivateInput(path string, limit int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: private input unavailable", errCohort)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 ||
		info.Size() > int64(limit) {
		return nil, fmt.Errorf(
			"%w: private input must be a bounded owner-only regular file",
			errCohort,
		)
	}
	raw, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil || len(raw) > limit {
		return nil, fmt.Errorf("%w: bounded private input read failed", errCohort)
	}
	return raw, nil
}

func ReadPrivateStringMap(path string) (map[string]string, error) {
	raw, err := ReadPrivateInput(path, 64<<10)
	if err != nil {
		return nil, fmt.Errorf("load driver input or observer: %w", err)
	}
	var values map[string]string
	if json.Unmarshal(raw, &values) != nil || len(values) == 0 {
		return nil, fmt.Errorf("%w: explicit private string map required", errCohort)
	}
	return values, nil
}

// OpenReadOnlyObserver dials only an explicitly configured loopback database.
// URL query parameters cannot retarget the connection beyond that exact host
// and port. All observer transactions default to read-only; the Compose setup
// additionally grants its observer role SELECT without application writes.
func OpenReadOnlyObserver(ctx context.Context, raw string) (*pgxpool.Pool, error) {
	target, err := url.Parse(raw)
	if err != nil || (target.Scheme != "postgres" && target.Scheme != "postgresql") ||
		target.User == nil || target.Hostname() == "" || target.Port() == "" {
		return nil, fmt.Errorf("%w: explicit private loopback observer URL required", errCohort)
	}
	password, hasPassword := target.User.Password()
	if target.User.Username() != "load123_observer" || !hasPassword || password == "" {
		return nil, fmt.Errorf(
			"%w: dedicated private read-only observer credentials required",
			errCohort,
		)
	}
	address := net.ParseIP(target.Hostname())
	if address == nil || !address.IsLoopback() {
		return nil, fmt.Errorf("%w: observer database must be explicitly loopback-bound", errCohort)
	}
	config, err := pgxpool.ParseConfig(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid private observer configuration", errCohort)
	}
	if config.ConnConfig.Host != target.Hostname() ||
		strconv.Itoa(int(config.ConnConfig.Port)) != target.Port() {
		return nil, fmt.Errorf(
			"%w: observer query parameters cannot retarget explicit loopback destination",
			errCohort,
		)
	}
	if config.ConnConfig.User != target.User.Username() || config.ConnConfig.Password != password {
		return nil, fmt.Errorf(
			"%w: observer query parameters cannot replace explicit credentials",
			errCohort,
		)
	}
	for _, fallback := range config.ConnConfig.Fallbacks {
		if fallback.Host != target.Hostname() || fallback.Port != config.ConnConfig.Port {
			return nil, fmt.Errorf(
				"%w: observer fallback cannot retarget explicit loopback destination",
				errCohort,
			)
		}
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
