package app

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/google/uuid"
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
	browser := lifecycleExternalBrowser(t, "attendee-01")
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
