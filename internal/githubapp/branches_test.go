package githubapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestExactManagedBranchObservationUsesReadOnlyScopedToken(t *testing.T) {
	key := syntheticKey(t)
	c := Configuration{AppID: 42, OrganizationID: 99, Organization: "synthetic-org", TemplateRepository: "template"}
	for _, tc := range []struct {
		name                 string
		status               int
		repo                 int64
		ref, sha, kind, want string
		reject               bool
	}{
		{name: "exact branch with slash", repo: 700, ref: "refs/heads/release/build", sha: strings.Repeat("a", 40), kind: "commit", want: "current_ref"},
		{name: "branch missing", repo: 700, status: 404, want: "branch_missing"},
		{name: "empty repository", repo: 700, status: 409, want: "empty_repository"},
		{name: "provider outage", repo: 700, status: 503, reject: true},
		{name: "recreated name", repo: 701, reject: true},
		{name: "matching prefix is not exact", repo: 700, ref: "refs/heads/release/build-other", sha: strings.Repeat("a", 40), kind: "commit", reject: true},
		{name: "tag object is not commit", repo: 700, ref: "refs/heads/release/build", sha: strings.Repeat("a", 40), kind: "tag", reject: true},
		{name: "invalid sha", repo: 700, ref: "refs/heads/release/build", sha: "untrusted", kind: "commit", reject: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gets := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/app/installations/55/access_tokens":
					if r.Method != "POST" {
						t.Errorf("token method %s", r.Method)
					}
					var body struct {
						Repositories []string          `json:"repositories"`
						Permissions  map[string]string `json:"permissions"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if len(body.Repositories) != 1 || body.Repositories[0] != "managed" || body.Permissions["contents"] != "read" || body.Permissions["administration"] != "" {
						t.Errorf("excess token scope %+v", body)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"token": "synthetic-test-token", "expires_at": time.Now().Add(time.Hour).Format(time.RFC3339)})
				case "/repos/synthetic-org/managed":
					if r.Method != "GET" {
						t.Error("repository write")
					}
					gets++
					_ = json.NewEncoder(w).Encode(map[string]any{"id": tc.repo, "name": "managed", "private": true, "owner": map[string]any{"id": 99}})
				case "/repos/synthetic-org/managed/git/ref/heads/release/build":
					if r.Method != "GET" {
						t.Error("ref write")
					}
					gets++
					if tc.status != 0 {
						w.WriteHeader(tc.status)
						_, _ = w.Write([]byte(`{"message":"PRIVATE PROVIDER BODY MUST NOT ESCAPE"}`))
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"ref": tc.ref, "object": map[string]any{"type": tc.kind, "sha": tc.sha}})
				default:
					t.Errorf("unexpected provider path %s", r.URL.Path)
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			p := &RESTProvider{transport: server.Client().Transport, baseURL: server.URL + "/"}
			o, err := p.ObserveBranch(t.Context(), c, key, 55, "managed", 700, "release/build")
			if tc.reject {
				if err == nil || strings.Contains(err.Error(), "PRIVATE") {
					t.Fatal(o, err)
				}
			} else if err != nil || o.Status != tc.want {
				t.Fatal(o, err)
			}
			if gets < 1 {
				t.Fatal("no provider observation")
			}
		})
	}
}
