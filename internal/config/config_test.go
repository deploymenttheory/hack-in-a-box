package config_test

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
)

func TestLoad(t *testing.T) {
	t.Parallel()
	env := map[string]string{
		"HACKBOX_ENV": "production", "HACKBOX_HTTP_ADDR": "[::1]:9090",
		"HACKBOX_LOG_LEVEL": "DEBUG", "HACKBOX_MAX_BODY_BYTES": "4096",
		"HACKBOX_REQUEST_TIMEOUT": "2s", "HACKBOX_SHUTDOWN_TIMEOUT": "4s",
	}
	cfg, err := config.Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Environment != "production" || cfg.HTTPAddr != "[::1]:9090" || cfg.LogLevel != slog.LevelDebug ||
		cfg.MaxBodyBytes != 4096 || cfg.RequestTimeout != 2*time.Second ||
		cfg.ShutdownTimeout != 4*time.Second {
		t.Fatalf("unexpected configuration: %+v", cfg)
	}
}

func TestLoadRejectsInvalidConfigurationWithoutValues(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ key, value string }{
		{"HACKBOX_ENV", ""},
		{"HACKBOX_ENV", "secret-environment"},
		{"HACKBOX_HTTP_ADDR", "secret-address"},
		{"HACKBOX_HTTP_ADDR", "secret-host:8080"},
		{"HACKBOX_HTTP_ADDR", "127.0.0.1:65536"},
		{"HACKBOX_HTTP_ADDR", "127.0.0.1:0"},
		{"HACKBOX_LOG_LEVEL", "secret-log-level"},
		{"HACKBOX_MAX_BODY_BYTES", "0"},
		{"HACKBOX_MAX_BODY_BYTES", "67108865"},
		{"HACKBOX_REQUEST_TIMEOUT", "-1s"},
		{"HACKBOX_REQUEST_TIMEOUT", "6m"},
		{"HACKBOX_SHUTDOWN_TIMEOUT", "secret-duration"},
	} {
		t.Run(tc.key+"/"+tc.value, func(t *testing.T) {
			env := map[string]string{"HACKBOX_ENV": "test", tc.key: tc.value}
			_, err := config.Load(func(key string) string { return env[key] })
			if err == nil || !strings.Contains(err.Error(), tc.key) {
				t.Fatalf("expected actionable error for %s, got %v", tc.key, err)
			}
			if strings.Contains(err.Error(), "secret-") {
				t.Fatalf("configuration value leaked: %v", err)
			}
		})
	}
}
