package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"golang.org/x/oauth2"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestLifecycleSyntheticGitHubBranchRESTContract(t *testing.T) {
	_, native, key, configuration := lifecycleGitHubRESTFixture(t)
	name := "hackbox-" + strings.ReplaceAll(uuid.NewString(), "-", "") + "-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	repository, err := native.CreateRepository(t.Context(), configuration, key, lifecycleGitHubInstallationID, name, "hackbox-provisioning:"+uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	observation, err := native.ObserveBranch(t.Context(), configuration, key, lifecycleGitHubInstallationID, name, repository.ID, "main")
	if err != nil || observation.Status != "branch_missing" || observation.SHA != "" {
		t.Fatalf("actual scoped branch REST contract unavailable: status=%q sha_present=%t err=%v", observation.Status, observation.SHA != "", err)
	}
}

func lifecycleExternalBrowser(t *testing.T, actor string) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	browser := &http.Client{Jar: jar, Transport: http.DefaultTransport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	oauth := oauth2.Config{ClientID: "lifecycle-github", ClientSecret: "synthetic-github-secret", RedirectURL: "http://public:8080/auth/github/callback", Endpoint: oauth2.Endpoint{AuthURL: "https://github.com/login/oauth/authorize", TokenURL: "https://github.com/login/oauth/access_token", AuthStyle: oauth2.AuthStyleInParams}}
	verifier := oauth2.GenerateVerifier()
	authorize, _ := url.Parse(oauth.AuthCodeURL("synthetic-access-state-at-least-thirty-two-bytes", oauth2.S256ChallengeOption(verifier)))
	query := authorize.Query()
	query.Set("actor", actor)
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
	return browser
}

func TestLifecycleSyntheticGitHubExternalWorkRequiresAcceptedWrite(t *testing.T) {
	actors := lifecycleDefaultGitHubActors()
	actors["mentor-a"] = 37
	fixture, native, key, configuration := lifecycleGitHubRESTFixtureForActors(t, actors)
	name := "hackbox-" + strings.ReplaceAll(uuid.NewString(), "-", "") + "-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	repository, err := native.CreateRepository(t.Context(), configuration, key, lifecycleGitHubInstallationID, name, "hackbox-provisioning:"+uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	path := "https://github.com/repositories/" + name + "/commits"
	work := func(browser *http.Client, origin, parent string, want int) lifecycleExternalWorkReceipt {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"parent_sha": parent, "content": "Synthetic demonstration work"})
		request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Origin", origin)
		request.Header.Set("Content-Type", "application/json")
		response, err := browser.Do(request)
		if err != nil {
			t.Fatal("strict external work transport unavailable")
		}
		defer response.Body.Close()
		if response.StatusCode != want {
			t.Fatalf("external work returned%d want%d", response.StatusCode, want)
		}
		var receipt lifecycleExternalWorkReceipt
		if want == http.StatusCreated {
			if err := json.NewDecoder(response.Body).Decode(&receipt); err != nil {
				t.Fatal(err)
			}
		}
		return receipt
	}
	writer := lifecycleExternalBrowser(t, "attendee-01")
	anonymous := &http.Client{Transport: http.DefaultTransport}
	work(anonymous, "https://github.com", "", http.StatusForbidden)
	work(writer, "https://foreign.example.test", "", http.StatusForbidden)
	work(writer, "https://github.com", "", http.StatusForbidden)
	access := githubapp.AccessRequest{RepositoryRequest: githubapp.RepositoryRequest{Configuration: configuration, InstallationID: lifecycleGitHubInstallationID, Name: name, Marker: repository.Description}, RepositoryID: repository.ID}
	invitation, err := native.ChangeCollaborator(t.Context(), configuration, key, lifecycleGitHubInstallationID, access, githubapp.AccessChange{AccountID: 5001, Permission: "push"})
	if err != nil || invitation.Invitation == nil {
		t.Fatal("pending write invitation unavailable")
	}
	work(writer, "https://github.com", "", http.StatusForbidden)
	accept := func(browser *http.Client, id int64) {
		t.Helper()
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, fmt.Sprintf("https://github.com/invitations/%d/accept", id), nil)
		request.Header.Set("Origin", "https://github.com")
		response, err := browser.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusNoContent {
			t.Fatal("external invitation not accepted")
		}
	}
	accept(writer, invitation.Invitation.ID)
	reader := lifecycleExternalBrowser(t, "mentor-a")
	invitation, err = native.ChangeCollaborator(t.Context(), configuration, key, lifecycleGitHubInstallationID, access, githubapp.AccessChange{AccountID: 5037, Permission: "pull"})
	if err != nil || invitation.Invitation == nil {
		t.Fatal("pending read invitation unavailable")
	}
	accept(reader, invitation.Invitation.ID)
	work(reader, "https://github.com", "", http.StatusForbidden)
	otherName := "hackbox-" + strings.ReplaceAll(uuid.NewString(), "-", "") + "-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := native.CreateRepository(t.Context(), configuration, key, lifecycleGitHubInstallationID, otherName, "hackbox-provisioning:"+uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	originalPath := path
	path = "https://github.com/repositories/" + otherName + "/commits"
	work(writer, "https://github.com", "", http.StatusForbidden)
	path = originalPath
	first := work(writer, "https://github.com", "", http.StatusCreated)
	if first.ID == uuid.Nil || first.RepositoryID != repository.ID || first.ActorID != 5001 || len(first.SHA) != 40 || len(first.ContentSHA256) != 64 {
		t.Fatal("external work receipt lost its actual resource/actor/hash")
	}
	observed, err := native.ObserveBranch(t.Context(), configuration, key, lifecycleGitHubInstallationID, name, repository.ID, "main")
	if err != nil || observed.Status != "current_ref" || observed.SHA != first.SHA {
		t.Fatal("native current-ref read did not independently observe external work")
	}
	if _, err := native.ObserveBranch(t.Context(), configuration, key, lifecycleGitHubInstallationID, name, repository.ID+1, "main"); err == nil {
		t.Fatal("mismatched managed numeric identity was accepted")
	}
	raw, _ := json.Marshal(map[string]any{"repository": name, "failures": 1})
	control := httptest.NewRecorder()
	fixture.control(control, httptest.NewRequest(http.MethodPost, "/github/branch-fault", bytes.NewReader(raw)))
	if control.Code != http.StatusOK {
		t.Fatal("private one-read fault not configured")
	}
	if _, err := native.ObserveBranch(t.Context(), configuration, key, lifecycleGitHubInstallationID, name, repository.ID, "main"); err == nil {
		t.Fatal("scoped branch503 was reported as successful observation")
	}
	recovered, err := native.ObserveBranch(t.Context(), configuration, key, lifecycleGitHubInstallationID, name, repository.ID, "main")
	if err != nil || recovered.SHA != first.SHA {
		t.Fatal("one-shot provider failure changed branch source or persisted beyond its exact request")
	}
	second := work(writer, "https://github.com", first.SHA, http.StatusCreated)
	if second.SHA == first.SHA || second.ParentSHA != first.SHA {
		t.Fatal("later external work replaced original receipt identity")
	}
	work(writer, "https://github.com", first.SHA, http.StatusConflict)
	observed, err = native.ObserveBranch(t.Context(), configuration, key, lifecycleGitHubInstallationID, name, repository.ID, "main")
	if err != nil || observed.SHA != second.SHA || observed.SHA == first.SHA {
		t.Fatal("current-ref provider invented historical head evidence")
	}
}
