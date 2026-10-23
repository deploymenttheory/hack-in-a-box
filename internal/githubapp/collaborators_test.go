package githubapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCollaboratorTransportUsesScopedTokensAndCurrentNumericIdentity(t *testing.T) {
	key := syntheticKey(t)
	writes, tokens := 0, 0
	mode := "invite"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/app":
			fmt.Fprint(w, `{"id":42,"slug":"synthetic-app"}`)
		case "/app/installations/55":
			// No contents permission or template visibility is required for an
			// existing native repository's collaborator cleanup.
			organizationID, suspended := 99, "null"
			if mode == "installation-mismatch" {
				organizationID = 100
			}
			if mode == "suspended" {
				suspended = `"2026-10-23T12:00:00Z"`
			}
			fmt.Fprintf(
				w,
				`{"id":55,"app_id":42,"account":{"id":%d,"login":"synthetic-org","type":"Organization"},"suspended_at":%s,"permissions":{"metadata":"read","administration":"write"}}`,
				organizationID,
				suspended,
			)
		case "/app/installations/55/access_tokens":
			tokens++
			var input struct {
				Repositories []string          `json:"repositories"`
				Permissions  map[string]string `json:"permissions"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error(err)
			}
			expected := "write"
			if tokens == 1 {
				expected = "read"
			}
			if len(input.Repositories) != 1 || input.Repositories[0] != "hackbox-team" ||
				len(input.Permissions) != 2 ||
				input.Permissions["metadata"] != "read" ||
				input.Permissions["administration"] != expected {
				t.Errorf("unsafe token scope %+v", input)
			}
			fmt.Fprintf(
				w,
				`{"token":"synthetic-private-token","expires_at":%q}`,
				time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			)
		case "/repos/synthetic-org/hackbox-team":
			fmt.Fprint(
				w,
				`{"id":900,"name":"hackbox-team","private":true,"description":"hackbox-provisioning:synthetic","owner":{"id":99}}`,
			)
		case "/repos/synthetic-org/hackbox-team/collaborators":
			if r.URL.Query().Get("affiliation") != "all" {
				t.Error("collaborator observation omitted visible inherited access")
			}
			fmt.Fprint(w, `[{"id":101,"login":"renamed-user","role_name":"write"}]`)
		case "/repos/synthetic-org/hackbox-team/invitations":
			fmt.Fprint(
				w,
				`[{"id":501,"invitee":{"id":102},"permissions":"read","expired":true,"created_at":"2027-01-01T00:00:00Z"}]`,
			)
		case "/user/101":
			id := 101
			if mode == "identity-mismatch" {
				id = 102
			}
			fmt.Fprintf(w, `{"id":%d,"login":"renamed-user"}`, id)
		case "/repos/synthetic-org/hackbox-team/collaborators/renamed-user":
			writes++
			if mode == "policy" {
				w.WriteHeader(403)
				fmt.Fprint(w, `{"message":"PRIVATE PROVIDER DETAIL"}`)
				return
			}
			if r.Method == "DELETE" {
				w.WriteHeader(204)
				return
			}
			var body struct {
				Permission string `json:"permission"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Permission != "push" {
				t.Error("unexpected privilege", body.Permission)
			}
			if mode == "accepted" {
				w.WriteHeader(204)
				return
			}
			w.WriteHeader(201)
			fmt.Fprint(
				w,
				`{"id":502,"invitee":{"id":101},"repository":{"id":900},"permissions":"write","created_at":"2027-01-01T00:00:00Z"}`,
			)
		case "/repos/synthetic-org/hackbox-team/invitations/502":
			if r.Method != "DELETE" {
				t.Error("wrong invitation removal method")
			}
			writes++
			w.WriteHeader(204)
		default:
			t.Error("unexpected provider request", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	provider := &RESTProvider{transport: server.Client().Transport, baseURL: server.URL + "/"}
	configuration := Configuration{
		AppID:              42,
		AppSlug:            "synthetic-app",
		OrganizationID:     99,
		Organization:       "synthetic-org",
		TemplateRepository: "template",
	}
	request := AccessRequest{
		RepositoryRequest: RepositoryRequest{
			Configuration:  configuration,
			InstallationID: 55,
			Name:           "hackbox-team",
			Marker:         "hackbox-provisioning:synthetic",
		},
		RepositoryID: 900,
	}
	snapshot, err := provider.InspectCollaborators(t.Context(), configuration, key, 55, request)
	if err != nil || len(snapshot.Collaborators) != 1 || snapshot.Collaborators[0].ID != 101 ||
		len(snapshot.Invitations) != 1 ||
		!snapshot.Invitations[0].Expired {
		t.Fatal("lost current identity/status", snapshot, err)
	}
	change := AccessChange{AccountID: 101, Permission: "push"}
	result, err := provider.ChangeCollaborator(t.Context(), configuration, key, 55, request, change)
	if err != nil || result.Invitation == nil || result.Invitation.ID != 502 ||
		result.Invitation.AccountID != 101 {
		t.Fatal(result, err)
	}
	mode = "accepted"
	result, err = provider.ChangeCollaborator(t.Context(), configuration, key, 55, request, change)
	if err != nil || !result.Accepted || result.Invitation != nil {
		t.Fatal("204 was not distinct", result, err)
	}
	mode = "identity-mismatch"
	if _, err = provider.ChangeCollaborator(
		t.Context(),
		configuration,
		key,
		55,
		request,
		change,
	); !errors.Is(err, ErrIntegration) ||
		writes != 2 {
		t.Fatal("stale numeric identity sent a write", writes, err)
	}
	mode = "policy"
	if _, err = provider.ChangeCollaborator(
		t.Context(),
		configuration,
		key,
		55,
		request,
		change,
	); !errors.Is(err, ErrAccessRejected) ||
		strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal("unsafe policy diagnostics", err)
	}
	beforeWrites := writes
	for _, invalid := range []string{"installation-mismatch", "suspended"} {
		mode = invalid
		if _, err = provider.ChangeCollaborator(
			t.Context(),
			configuration,
			key,
			55,
			request,
			change,
		); !errors.Is(
			err,
			ErrAccessNotSent,
		) {
			t.Fatal("stale installation reached dispatch", invalid, err)
		}
	}
	if writes != beforeWrites {
		t.Fatal("invalid current installation mutated access")
	}
	mode = "remove"
	change.Remove = true
	if _, err = provider.ChangeCollaborator(
		t.Context(),
		configuration,
		key,
		55,
		request,
		change,
	); err != nil {
		t.Fatal(err)
	}
	change.InvitationID = 502
	if _, err = provider.ChangeCollaborator(
		t.Context(),
		configuration,
		key,
		55,
		request,
		change,
	); err != nil {
		t.Fatal(err)
	}
	if writes != 5 {
		t.Fatal("unexpected mutation count", writes)
	}
}
