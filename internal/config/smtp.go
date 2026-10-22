package config

import (
	"strconv"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
)

func loadSMTP(getenv func(string) string) (maildelivery.Settings, error) {
	settings := maildelivery.Settings{
		Host:          getenv("HACKBOX_SMTP_HOST"),
		Username:      getenv("HACKBOX_SMTP_USERNAME"),
		Password:      getenv("HACKBOX_SMTP_PASSWORD"),
		From:          getenv("HACKBOX_SMTP_FROM"),
		MessageDomain: getenv("HACKBOX_SMTP_MESSAGE_DOMAIN"),
		Mode:          getenv("HACKBOX_SMTP_TLS_MODE"),
	}
	if settings.Configured() {
		settings.Port = 587
		settings.Timeout = 10 * time.Second
		if settings.Mode == "" {
			settings.Mode = "starttls"
		}
	}
	if raw := getenv("HACKBOX_SMTP_PORT"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return settings, maildelivery.ErrConfiguration
		}
		settings.Port = value
	}
	if raw := getenv("HACKBOX_SMTP_TIMEOUT"); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil {
			return settings, maildelivery.ErrConfiguration
		}
		settings.Timeout = value
	}
	return settings, settings.Validate()
}
