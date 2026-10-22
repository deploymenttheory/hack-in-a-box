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
		"HACKBOX_PUBLIC_ORIGIN":    "https://hack.example.test",
		"HACKBOX_OIDC_ISSUER":      "https://login.example.test/tenant",
		"HACKBOX_OIDC_CLIENT_ID":   "test-client",
		"HACKBOX_METRICS_TOKEN":    "synthetic-test-token-32-characters-long",
		"HACKBOX_ENV":              "production",
		"HACKBOX_HTTP_ADDR":        "[::1]:9090",
		"HACKBOX_LOG_LEVEL":        "DEBUG",
		"HACKBOX_MAX_BODY_BYTES":   "4096",
		"HACKBOX_REQUEST_TIMEOUT":  "2s",
		"HACKBOX_SHUTDOWN_TIMEOUT": "4s",
	}
	cfg, err := config.Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Environment != "production" || cfg.HTTPAddr != "[::1]:9090" ||
		cfg.LogLevel != slog.LevelDebug ||
		cfg.MaxBodyBytes != 4096 ||
		cfg.RequestTimeout != 2*time.Second ||
		cfg.ShutdownTimeout != 4*time.Second {
		t.Fatalf("unexpected configuration: %+v", cfg)
	}
}

func TestLoadRejectsInvalidConfigurationWithoutValues(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ key, value string }{
		{"HACKBOX_SURFACE", "secret-surface"},
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

func TestOIDCIssuerRetainsProviderTrailingSlash(t *testing.T) {
	t.Parallel()
	env := map[string]string{
		"HACKBOX_ENV":            "production",
		"HACKBOX_PUBLIC_ORIGIN":  "https://hack.example.test",
		"HACKBOX_OIDC_ISSUER":    "https://issuer.example.test/",
		"HACKBOX_OIDC_CLIENT_ID": "synthetic-client",
		"HACKBOX_METRICS_TOKEN":  "synthetic-metrics-token-32-characters",
	}
	cfg, err := config.Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OIDCIssuer != env["HACKBOX_OIDC_ISSUER"] {
		t.Fatal("issuer was normalized instead of retained exactly")
	}
}

func TestDeploymentSurfaces(t *testing.T) {
	t.Parallel()
	for _, surface := range []string{"", "public", "admin"} {
		cfg, err := config.Load(func(key string) string {
			switch key {
			case "HACKBOX_ENV":
				return "test"
			case "HACKBOX_SURFACE":
				return surface
			default:
				return ""
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		expected := surface
		if expected == "" {
			expected = "public"
		}
		if cfg.Surface != expected {
			t.Fatalf("surface = %q, want %q", cfg.Surface, expected)
		}
	}
}

func TestCredentialKeyringConfigurationErrorsAreSafe(t *testing.T) {
	_, err := config.Load(func(name string) string {
		if name == "HACKBOX_ENV" {
			return "test"
		}
		if name == "HACKBOX_CREDENTIAL_KEYRING" {
			return `{"active":"synthetic","keys":{"synthetic":"DO-NOT-ECHO-SYNTHETIC"}}`
		}
		return ""
	})
	if err == nil || strings.Contains(err.Error(), "DO-NOT-ECHO-SYNTHETIC") {
		t.Fatalf("unsafe key configuration error: %v", err)
	}
}

func TestWorkspaceOriginIsIndependentAndSafelyValidated(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"ftp://secret-host", "https://user:secret-password@example.test", "https://example.test/path", "https://example.test?secret-query", "https://example.test#secret-fragment"} {
		_, err := config.Load(func(key string) string {
			if key == "HACKBOX_ENV" {
				return "test"
			}
			if key == "HACKBOX_WORKSPACE_ORIGIN" {
				return value
			}
			return ""
		})
		if err == nil || !strings.Contains(err.Error(), "HACKBOX_WORKSPACE_ORIGIN") ||
			strings.Contains(err.Error(), "secret-") {
			t.Fatalf("unsafe or absent origin validation: %v", err)
		}
	}
	cfg, err := config.Load(func(key string) string {
		if key == "HACKBOX_ENV" {
			return "test"
		}
		if key == "HACKBOX_PUBLIC_ORIGIN" {
			return "http://localhost:8081"
		}
		return ""
	})
	if err != nil || cfg.WorkspaceOrigin != cfg.PublicOrigin {
		t.Fatal("single-surface workspace fallback failed")
	}
	_, err = config.Load(func(key string) string {
		switch key {
		case "HACKBOX_ENV":
			return "production"
		case "HACKBOX_PUBLIC_ORIGIN":
			return "https://admin.example.test"
		case "HACKBOX_WORKSPACE_ORIGIN":
			return "http://workspace.example.test"
		case "HACKBOX_OIDC_ISSUER":
			return "https://issuer.example.test"
		case "HACKBOX_OIDC_CLIENT_ID":
			return "synthetic"
		case "HACKBOX_METRICS_TOKEN":
			return "synthetic-metrics-token-32-characters"
		}
		return ""
	})
	if err == nil || !strings.Contains(err.Error(), "HACKBOX_WORKSPACE_ORIGIN") {
		t.Fatal("production workspace HTTP accepted")
	}
}
