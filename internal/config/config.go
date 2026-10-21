// Package config loads and validates deployment configuration without echoing values.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strconv"
	"time"
)

var (
	errEnvironment = errors.New("HACKBOX_ENV is required: use development, test or production")
	errAddress     = errors.New(
		"HACKBOX_HTTP_ADDR must be an IP address or localhost and a port between 1 and 65535",
	)
	errLogLevel = errors.New(
		"HACKBOX_LOG_LEVEL must be a valid slog level, such as DEBUG, INFO, WARN or ERROR",
	)
	errBodyLimit = errors.New("HACKBOX_MAX_BODY_BYTES must be between 1 and 67108864 bytes")
	errDuration  = errors.New("must be a positive duration no greater than 5m (for example 10s)")
)

// Config is shared by the API and worker processes. HTTP fields apply to the API.
type Config struct {
	DatabaseURL      string
	PublicOrigin     string
	OIDCIssuer       string
	OIDCClientID     string
	OIDCClientSecret string
	BootstrapSubject string
	StoragePath      string
	WebDir           string
	MetricsToken     string
	FileMaxBytes     int64
	FileEventQuota   int64
	Environment      string
	LogLevel         slog.Level
	HTTPAddr         string
	MaxBodyBytes     int64
	RequestTimeout   time.Duration
	ShutdownTimeout  time.Duration
}

// Load uses an injected environment lookup so configuration tests are isolated.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		DatabaseURL:      getenv("HACKBOX_DATABASE_URL"),
		PublicOrigin:     valueOr(getenv("HACKBOX_PUBLIC_ORIGIN"), "http://127.0.0.1:8080"),
		OIDCIssuer:       getenv("HACKBOX_OIDC_ISSUER"),
		OIDCClientID:     getenv("HACKBOX_OIDC_CLIENT_ID"),
		OIDCClientSecret: getenv("HACKBOX_OIDC_CLIENT_SECRET"),
		BootstrapSubject: getenv("HACKBOX_BOOTSTRAP_SUBJECT"),
		StoragePath:      valueOr(getenv("HACKBOX_STORAGE_PATH"), "data/files"),
		WebDir:           valueOr(getenv("HACKBOX_WEB_DIR"), "web/dist"),
		MetricsToken:     getenv("HACKBOX_METRICS_TOKEN"),
		FileMaxBytes:     8 << 20,
		FileEventQuota:   1 << 30,
		Environment:      getenv("HACKBOX_ENV"),
		HTTPAddr:         valueOr(getenv("HACKBOX_HTTP_ADDR"), "127.0.0.1:8080"),
		MaxBodyBytes:     16 << 20,
		RequestTimeout:   10 * time.Second,
		ShutdownTimeout:  20 * time.Second,
	}
	var problems []error
	problems = append(problems, loadFoundation(&cfg, getenv)...)
	switch cfg.Environment {
	case "development", "test", "production":
	default:
		problems = append(problems, errEnvironment)
	}
	host, port, err := net.SplitHostPort(cfg.HTTPAddr)
	portNumber, portErr := strconv.Atoi(port)
	_, ipErr := netip.ParseAddr(host)
	if err != nil || portErr != nil || portNumber < 1 || portNumber > 65535 ||
		(host != "" && host != "localhost" && ipErr != nil) {
		problems = append(problems, errAddress)
	}
	if err := cfg.LogLevel.UnmarshalText(
		[]byte(valueOr(getenv("HACKBOX_LOG_LEVEL"), "INFO")),
	); err != nil {
		problems = append(problems, errLogLevel)
	}
	if value := getenv("HACKBOX_MAX_BODY_BYTES"); value != "" {
		limit, err := strconv.ParseInt(value, 10, 64)
		if err != nil || limit < 1 || limit > 64<<20 {
			problems = append(problems, errBodyLimit)
		} else {
			cfg.MaxBodyBytes = limit
		}
	}
	for _, setting := range []struct {
		name   string
		target *time.Duration
	}{
		{"HACKBOX_REQUEST_TIMEOUT", &cfg.RequestTimeout},
		{"HACKBOX_SHUTDOWN_TIMEOUT", &cfg.ShutdownTimeout},
	} {
		if value := getenv(setting.name); value != "" {
			duration, err := time.ParseDuration(value)
			if err != nil || duration <= 0 || duration > 5*time.Minute {
				problems = append(problems, fmt.Errorf("%s %w", setting.name, errDuration))
			} else {
				*setting.target = duration
			}
		}
	}
	if len(problems) != 0 {
		return Config{}, fmt.Errorf("invalid configuration: %w", errors.Join(problems...))
	}
	return cfg, nil
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
