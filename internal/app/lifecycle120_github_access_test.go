package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

func TestLifecycleSyntheticGitHubAccessRESTContract(t *testing.T) {
	fixture, native, key, configuration := lifecycleGitHubRESTFixture(t)
	name := "hackbox-" + strings.ReplaceAll(uuid.NewString(), "-", "") + "-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	marker := "hackbox-provisioning:" + uuid.NewString()
	repository, err := native.CreateRepository(t.Context(), configuration, key, lifecycleGitHubInstallationID, name, marker)
	if err != nil {
		t.Fatal("native template creation failed")
	}
	request := githubapp.AccessRequest{RepositoryRequest: githubapp.RepositoryRequest{Configuration: configuration, InstallationID: lifecycleGitHubInstallationID, Name: name, Marker: marker}, RepositoryID: repository.ID}
	snapshot, err := native.InspectCollaborators(t.Context(), configuration, key, lifecycleGitHubInstallationID, request)
	if err != nil || len(snapshot.Collaborators) != 0 || len(snapshot.Invitations) != 0 {
		t.Fatal("native scoped access snapshot failed")
	}
	result, err := native.ChangeCollaborator(t.Context(), configuration, key, lifecycleGitHubInstallationID, request, githubapp.AccessChange{AccountID: 5001, Permission: "push"})
	if err != nil || result.Accepted || result.Invitation == nil || result.Invitation.AccountID != 5001 || result.Invitation.Permission != "write" {
		t.Fatal("native invitation was not a distinct pending external effect")
	}
	snapshot, err = native.InspectCollaborators(t.Context(), configuration, key, lifecycleGitHubInstallationID, request)
	if err != nil || len(snapshot.Collaborators) != 0 || len(snapshot.Invitations) != 1 || snapshot.Invitations[0].ID != result.Invitation.ID {
		t.Fatal("pending invitation was mistaken for accepted collaborator")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	browser := &http.Client{Jar: jar, Transport: http.DefaultTransport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	oauth := oauth2.Config{ClientID: "lifecycle-github", ClientSecret: "synthetic-github-secret", RedirectURL: "http://public:8080/auth/github/callback", Endpoint: oauth2.Endpoint{AuthURL: "https://github.com/login/oauth/authorize", TokenURL: "https://github.com/login/oauth/access_token", AuthStyle: oauth2.AuthStyleInParams}}
	verifier := oauth2.GenerateVerifier()
	authorize, _ := url.Parse(oauth.AuthCodeURL("synthetic-access-state-at-least-thirty-two-bytes", oauth2.S256ChallengeOption(verifier)))
	query := authorize.Query()
	query.Set("actor", "attendee-01")
	authorize.RawQuery = query.Encode()
	response, err := browser.Get(authorize.String())
	if err != nil {
		t.Fatal("external browser login unavailable")
	}
	callback, locationErr := response.Location()
	response.Body.Close()
	if response.StatusCode != 302 || locationErr != nil {
		t.Fatal("external browser login did not bind OAuth authorization")
	}
	if _, err := oauth.Exchange(context.WithValue(t.Context(), oauth2.HTTPClient, browser), callback.Query().Get("code"), oauth2.VerifierOption(verifier)); err != nil {
		t.Fatal("external browser PKCE exchange rejected")
	}
	accept := fmt.Sprintf("https://github.com/invitations/%d/accept", result.Invitation.ID)
	for _, attempt := range []struct {
		client *http.Client
		origin string
		status int
	}{{&http.Client{Transport: http.DefaultTransport}, "https://github.com", 403}, {browser, "https://attacker.invalid", 403}, {browser, "https://github.com", 204}} {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, accept, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Origin", attempt.origin)
		response, err := attempt.client.Do(request)
		if err != nil {
			t.Fatal("external invitation acceptance unavailable")
		}
		response.Body.Close()
		if response.StatusCode != attempt.status {
			t.Fatal("external invitation ignored browser ownership/origin", response.StatusCode)
		}
	}
	snapshot, err = native.InspectCollaborators(t.Context(), configuration, key, lifecycleGitHubInstallationID, request)
	if err != nil || len(snapshot.Collaborators) != 1 || len(snapshot.Invitations) != 0 || snapshot.Collaborators[0].ID != 5001 || snapshot.Collaborators[0].Permission != "write" {
		t.Fatal("native observation did not independently verify accepted external access")
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.proof.InvitationCreates != 1 || fixture.proof.InvitationAcceptances != 1 {
		t.Fatal("external access effects duplicated")
	}
}
