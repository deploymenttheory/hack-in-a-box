package config

import (
	"errors"
	"net/url"
	"strconv"
)

var (
	errOrigin = errors.New(
		"HACKBOX_PUBLIC_ORIGIN must be an HTTP origin (HTTPS in production), with no path, query, fragment or credentials",
	)
	errWorkspaceOrigin = errors.New(
		"HACKBOX_WORKSPACE_ORIGIN must be an HTTP origin (HTTPS in production), with no path, query, fragment or credentials",
	)
	errOIDC = errors.New(
		"HACKBOX_OIDC_ISSUER and HACKBOX_OIDC_CLIENT_ID must be configured together; production requires HTTPS OIDC",
	)
	errQuota = errors.New(
		"HACKBOX_FILE_MAX_BYTES and HACKBOX_FILE_EVENT_QUOTA must be positive limits, with uploads no larger than 32 MiB and quota at least the upload limit",
	)
	errMetrics = errors.New(
		"HACKBOX_METRICS_TOKEN must contain at least 32 characters when configured, and is required in production",
	)
)

func loadFoundation(cfg *Config, getenv func(string) string) []error {
	problems := validateEndpoints(cfg)
	if (cfg.MetricsToken != "" || cfg.Environment == "production") && len(cfg.MetricsToken) < 32 {
		problems = append(problems, errMetrics)
	}
	for _, setting := range []struct {
		name  string
		value *int64
	}{{"HACKBOX_FILE_MAX_BYTES", &cfg.FileMaxBytes}, {"HACKBOX_FILE_EVENT_QUOTA", &cfg.FileEventQuota}} {
		if raw := getenv(setting.name); raw != "" {
			number, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				problems = append(problems, errQuota)
			} else {
				*setting.value = number
			}
		}
	}
	if cfg.FileMaxBytes < 1 || cfg.FileMaxBytes > 32<<20 || cfg.FileEventQuota < cfg.FileMaxBytes {
		problems = append(problems, errQuota)
	}
	return problems
}

func validateEndpoints(cfg *Config) []error {
	var problems []error
	if !validOrigin(cfg.PublicOrigin, cfg.Environment) {
		problems = append(problems, errOrigin)
	}
	if !validOrigin(cfg.WorkspaceOrigin, cfg.Environment) {
		problems = append(problems, errWorkspaceOrigin)
	}
	if cfg.OIDCIssuer != "" || cfg.OIDCClientID != "" || cfg.Environment == "production" {
		issuer, err := url.Parse(cfg.OIDCIssuer)
		if err != nil || issuer.Host == "" || issuer.User != nil || issuer.RawQuery != "" ||
			issuer.Fragment != "" ||
			cfg.OIDCClientID == "" ||
			(issuer.Scheme != "https" && (cfg.Environment == "production" || issuer.Scheme != "http")) {
			problems = append(problems, errOIDC)
		}
	}
	return problems
}

func validOrigin(raw, environment string) bool {
	origin, err := url.Parse(raw)
	return err == nil && origin.Host != "" &&
		(origin.Scheme == "http" || origin.Scheme == "https") &&
		origin.Path == "" &&
		origin.RawQuery == "" &&
		origin.Fragment == "" &&
		origin.User == nil &&
		(environment != "production" || origin.Scheme == "https")
}
