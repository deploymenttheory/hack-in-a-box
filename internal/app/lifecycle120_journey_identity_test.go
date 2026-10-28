package app

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/githubidentity"
)

// Linking is global to a person, not an event grant. Use each participant's real
// SSO session and verify that a second event/session sees the same owned numeric
// identity without creating or replacing it.
func (r *lifecycleJourney) linkGitHubParticipants(t *testing.T) {
	t.Helper()
	type identity struct {
		client, other *lifecycleClient
		subject       string
		numericID     int64
	}
	identities := []identity{}
	for index, client := range r.events[0].cohort.clients {
		// The extra reserve never confirms or receives a repository grant. Keep
		// the strict36-person GitHub fixture unchanged and do not link that user.
		if client.person == r.events[0].cohort.reservePerson {
			continue
		}
		identities = append(identities, identity{client: client, other: r.events[1].cohort.clients[index], subject: fmt.Sprintf("attendee-%02d", index+1), numericID: int64(5001 + index)})
	}
	for index, actor := range []string{"mentor-a", "mentor-b"} {
		client, other := r.events[0].cohort.staff[actor], r.events[1].cohort.staff[actor]
		if client == nil || other == nil || client.person != other.person {
			t.Fatal("selected mentor did not retain one normal OIDC identity across both events")
		}
		identities = append(identities, identity{client: client, other: other, subject: actor, numericID: int64(5037 + index)})
	}
	for index, current := range identities {
		client, expectedID := current.client, current.numericID
		var before githubidentity.Status
		if err := client.call(t.Context(), http.MethodGet, "/api/v1/account/github", -1, nil, &before, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if !before.Enabled || before.Account != nil {
			t.Fatal("fresh configured GitHub linking did not require verification")
		}
		var authorization struct {
			URL string `json:"url"`
		}
		if err := client.call(t.Context(), http.MethodPost, "/api/v1/account/github/authorize", before.Revision, map[string]bool{"acknowledge_invitation_impact": true}, &authorization, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		target, err := url.Parse(authorization.URL)
		if err != nil || target.Scheme != "https" || target.Host != "github.com" || target.Path != "/login/oauth/authorize" || target.Query().Get("code_challenge_method") != "S256" || len(target.Query().Get("state")) < 32 {
			t.Fatal("native participant owner did not create bound PKCE authorization")
		}
		query := target.Query()
		query.Set("actor", current.subject)
		target.RawQuery = query.Encode()
		transport, err := newLifecycleProviderTransport()
		if err != nil {
			t.Fatal(err)
		}
		previous := client.http.Transport
		client.http.Transport = transport
		callback, err := client.redirect(t.Context(), target.String(), http.StatusFound)
		client.http.Transport = previous
		transport.base.CloseIdleConnections()
		if err != nil {
			t.Fatal("external GitHub authorization unavailable")
		}
		parsed, err := url.Parse(callback)
		if err != nil || parsed.Scheme+"://"+parsed.Host+parsed.Path != client.origin+"/auth/github/callback" || parsed.Query().Get("state") != query.Get("state") || parsed.Query().Get("code") == "" {
			t.Fatal("external GitHub callback lost exact native state/origin")
		}
		if index == 0 {
			other := current.other
			result, err := other.redirect(t.Context(), callback, http.StatusSeeOther)
			if err != nil || result != other.origin+"/account?github=failed" {
				t.Fatal("another authenticated browser session consumed participant linking authority")
			}
		}
		result, err := client.redirect(t.Context(), callback, http.StatusSeeOther)
		if err != nil || result != client.origin+"/account?github=linked" {
			t.Fatal("native GitHub ownership callback failed")
		}
		result, err = client.redirect(t.Context(), callback, http.StatusSeeOther)
		if err != nil || result != client.origin+"/account?github=failed" {
			t.Fatal("native participant linking callback was replayable")
		}
		for _, reader := range []*lifecycleClient{client, current.other} {
			var status githubidentity.Status
			if err := reader.call(t.Context(), http.MethodGet, "/api/v1/account/github", -1, nil, &status, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if status.Account == nil || status.Account.GitHubID != expectedID || status.Account.Login != "lifecycle-"+current.subject || status.Revision != before.Revision+1 || status.Account.VerifiedAt.IsZero() {
				t.Fatal("native numeric GitHub ownership was not stable across event sessions")
			}
		}
	}
}
