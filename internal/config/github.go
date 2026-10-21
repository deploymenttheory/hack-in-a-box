package config

import (
	"encoding/base64"
	"errors"
)

var errGitHub = errors.New(
	"HACKBOX_GITHUB_CLIENT_ID, HACKBOX_GITHUB_CLIENT_SECRET and HACKBOX_GITHUB_TOKEN_KEY must be configured together; the token key must be base64-encoded 32 bytes",
)

func validateGitHub(c Config) error {
	if c.GitHubClientID == "" && c.GitHubClientSecret == "" && c.GitHubTokenKey == "" {
		return nil
	}
	key, err := base64.StdEncoding.DecodeString(c.GitHubTokenKey)
	if err != nil || len(key) != 32 || c.GitHubClientID == "" || c.GitHubClientSecret == "" {
		return errGitHub
	}
	return nil
}
