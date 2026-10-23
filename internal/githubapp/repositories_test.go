package githubapp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRepositoryTransportIsPrivateAndOperationScoped(t *testing.T) {
	key := syntheticKey(t)
	creates := 0
	tokens := 0
	marker := "hackbox-provisioning:synthetic"
	name := "hackbox-synthetic-team"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/app/installations/55/access_tokens":
			tokens++
			var input struct {
				Repositories []string          `json:"repositories"`
				Permissions  map[string]string `json:"permissions"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error(err)
			}
			expected := "template"
			if tokens == 2 {
				expected = name
			}
			if len(input.Repositories) != 1 || input.Repositories[0] != expected {
				t.Errorf("unsafe repositories %+v", input)
			}
			if input.Permissions["metadata"] != "read" {
				t.Errorf("missing metadata %+v", input)
			}
			if tokens == 1 &&
				(input.Permissions["contents"] != "read" || input.Permissions["administration"] != "write") {
				t.Errorf("wrong create scope %+v", input)
			}
			if tokens == 2 && len(input.Permissions) != 1 {
				t.Errorf("reconciliation has write permissions %+v", input)
			}
			fmt.Fprintf(
				w,
				`{"token":"synthetic-installation-token","expires_at":%q}`,
				time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			)
		case "/repos/synthetic-org/template/generate":
			creates++
			if r.Method != "POST" {
				t.Errorf("wrong method %s", r.Method)
			}
			var body struct {
				Name, Owner, Description    string
				Private, IncludeAllBranches bool
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if !body.Private || body.Owner != "synthetic-org" || body.Name != name ||
				body.Description != marker ||
				body.IncludeAllBranches {
				t.Errorf("unsafe create %+v", body)
			}
			w.WriteHeader(201)
			fmt.Fprintf(
				w,
				`{"id":123,"name":%q,"private":true,"description":%q,"owner":{"id":99},"template_repository":{"id":700}}`,
				name,
				marker,
			)
		case "/repos/synthetic-org/" + name:
			fmt.Fprintf(
				w,
				`{"id":123,"name":%q,"private":true,"description":%q,"owner":{"id":99},"template_repository":{"id":700}}`,
				name,
				marker,
			)
		default:
			t.Errorf("unexpected provider request %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	p := &RESTProvider{transport: server.Client().Transport, baseURL: server.URL + "/"}
	c := Configuration{
		AppID:              42,
		OrganizationID:     99,
		Organization:       "synthetic-org",
		TemplateRepository: "template",
	}
	created, err := p.CreateRepository(t.Context(), c, key, 55, name, marker)
	if err != nil || !created.Private || created.ID != 123 || created.TemplateID != 700 {
		t.Fatalf("create result %+v %v", created, err)
	}
	found, err := p.LookupRepository(t.Context(), c, key, 55, name)
	if err != nil || found != created {
		t.Fatalf("lookup %+v %v", found, err)
	}
	if creates != 1 || tokens != 2 {
		t.Fatalf("unexpected effect counts %d %d", creates, tokens)
	}
}
