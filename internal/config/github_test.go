package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestGitHubConfigurationRequiresCompleteEncryptionSettings(t *testing.T) {
	for _, cfg := range []Config{{GitHubClientID: "id"}, {GitHubTokenKey: "secret-value"}, {GitHubClientID: "id", GitHubClientSecret: "secret", GitHubTokenKey: base64.StdEncoding.EncodeToString([]byte("too-short"))}} {
		err := validateGitHub(cfg)
		if err == nil || strings.Contains(err.Error(), "secret-value") {
			t.Fatal("invalid or leaked GitHub config")
		}
	}
	if err := validateGitHub(
		Config{
			GitHubClientID:     "id",
			GitHubClientSecret: "secret",
			GitHubTokenKey:     base64.StdEncoding.EncodeToString(make([]byte, 32)),
		},
	); err != nil {
		t.Fatal(err)
	}
}
