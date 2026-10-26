package app

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/go-github/v89/github"
	"golang.org/x/oauth2"
)

// Exercise real OAuth/REST clients against the strict external fixture. Native
// state/session/encryption ownership is a separate connected HTTP checkpoint.
func TestLifecycleSyntheticGitHubOAuthPKCEContract(t *testing.T) {
	_, _, _, _ = lifecycleGitHubRESTFixture(t)
	configuration := oauth2.Config{ClientID: "lifecycle-github", ClientSecret: "synthetic-github-secret", RedirectURL: "http://public:8080/auth/github/callback", Endpoint: oauth2.Endpoint{AuthURL: "https://github.com/login/oauth/authorize", TokenURL: "https://github.com/login/oauth/access_token", AuthStyle: oauth2.AuthStyleInParams}}
	browser := &http.Client{Transport: http.DefaultTransport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	authorize := func(verifier string) string {
		t.Helper()
		target, err := url.Parse(configuration.AuthCodeURL("synthetic-state-at-least-thirty-two-bytes", oauth2.S256ChallengeOption(verifier)))
		if err != nil {
			t.Fatal(err)
		}
		query := target.Query()
		query.Set("actor", "attendee-01")
		target.RawQuery = query.Encode()
		response, err := browser.Get(target.String())
		if err != nil {
			t.Fatal("external authorization unavailable")
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusFound {
			t.Fatal("strict external authorization rejected native PKCE input", response.StatusCode)
		}
		callback, err := url.Parse(response.Header.Get("Location"))
		if err != nil || callback.Scheme+"://"+callback.Host+callback.Path != configuration.RedirectURL || callback.Query().Get("state") != "synthetic-state-at-least-thirty-two-bytes" {
			t.Fatal("external authorization lost exact callback/state")
		}
		return callback.Query().Get("code")
	}
	ctx := context.WithValue(t.Context(), oauth2.HTTPClient, browser)
	verifier := oauth2.GenerateVerifier()
	if _, err := configuration.Exchange(ctx, authorize(verifier), oauth2.VerifierOption(oauth2.GenerateVerifier())); err == nil {
		t.Fatal("wrong PKCE verifier accepted")
	}
	code := authorize(verifier)
	token, err := configuration.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		t.Fatal("native OAuth exchange failed")
	}
	if _, err := configuration.Exchange(ctx, code, oauth2.VerifierOption(verifier)); err == nil {
		t.Fatal("external authorization code reused")
	}
	client, err := github.NewClient(github.WithHTTPClient(browser), github.WithAuthToken(token.AccessToken))
	if err != nil {
		t.Fatal(err)
	}
	user, _, err := client.Users.Get(t.Context(), "")
	if err != nil || user.GetID() != 5001 || user.GetLogin() != "lifecycle-attendee-01" || user.GetType() != "User" {
		t.Fatal("real authenticated user lookup did not retain numeric ownership")
	}
}
