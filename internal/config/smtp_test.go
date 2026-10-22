package config

import (
	"strings"
	"testing"
	"time"
)

func TestSMTPConfigurationRequiresAuthenticatedTLSWithoutEcho(t *testing.T) {
	values := map[string]string{
		"HACKBOX_SMTP_HOST":           "smtp.example.test",
		"HACKBOX_SMTP_USERNAME":       "synthetic-relay-user",
		"HACKBOX_SMTP_PASSWORD":       "SYNTHETIC-RELAY-PASSWORD",
		"HACKBOX_SMTP_FROM":           "sender@example.test",
		"HACKBOX_SMTP_MESSAGE_DOMAIN": "example.test",
	}
	configured, err := loadSMTP(func(key string) string { return values[key] })
	if err != nil || !configured.Configured() || configured.Port != 587 ||
		configured.Mode != "starttls" ||
		configured.Timeout != 10*time.Second {
		t.Fatal("valid default SMTP configuration rejected")
	}
	for _, change := range []struct{ key, value string }{{"HACKBOX_SMTP_HOST", ""}, {"HACKBOX_SMTP_HOST", "bad\r\nhost"}, {"HACKBOX_SMTP_USERNAME", ""}, {"HACKBOX_SMTP_PASSWORD", ""}, {"HACKBOX_SMTP_FROM", "a@example.test,b@example.test"}, {"HACKBOX_SMTP_PORT", "not-a-port"}, {"HACKBOX_SMTP_PORT", "0"}, {"HACKBOX_SMTP_TLS_MODE", "none"}, {"HACKBOX_SMTP_TIMEOUT", "10m"}, {"HACKBOX_SMTP_TIMEOUT", "bad-secret-setting"}, {"HACKBOX_SMTP_MESSAGE_DOMAIN", "example.test\r\nBcc:other@example.test"}} {
		_, err := loadSMTP(func(key string) string {
			if key == change.key {
				return change.value
			}
			return values[key]
		})
		if err == nil || strings.Contains(err.Error(), "SYNTHETIC") ||
			strings.Contains(err.Error(), "example.test") ||
			strings.Contains(err.Error(), "bad-secret-setting") {
			t.Fatal("invalid SMTP setting accepted or echoed")
		}
	}
	empty, err := loadSMTP(func(string) string { return "" })
	if err != nil || empty.Configured() {
		t.Fatal("unconfigured SMTP should remain explicitly unavailable")
	}
}
