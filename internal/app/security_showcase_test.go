package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

// A reviewed organisation card is a read grant only. Use actual signed final
// results, publication and consent owners; do not fabricate a public DTO.
func TestSecurity121ShowcaseReadGrantCannotBecomeSourceOrPublicationAuthority(t *testing.T) {
	s := newShowcaseFixture(t, false)
	f, r := s.results, s.results.runtime
	request := qualificationHTTPRequest(t)
	public := "/api/v1/projects/" + s.winnerProject.String()
	private := "/api/v1/outcomes/" + s.winnerProject.String()
	for _, surface := range []rehearsalSurface{f.public, f.admin} {
		status, raw := request(surface, s.outsider, "GET", public, nil, 0, true)
		if status != 200 {
			t.Fatal("organisation read positive control failed", status)
		}
		project := resultAPIDecode[contract.OutcomeProject](t, "OutcomeProject", raw)
		if project.SourceReadable || len(project.Awards) != 0 || len(project.Links) != 1 {
			t.Fatal("organisation card widened source/result/media authority")
		}
		for _, field := range []string{"PRIVATE-", "actor_id", "owner_id", "recording_url", "source_binding", "ciphertext", "confirmation", "votes"} {
			if strings.Contains(string(raw), field) {
				t.Fatal("organisation card disclosed restricted source field", field)
			}
		}
		for _, path := range []string{private, private + "/publication", "/api/v1/projects/" + s.draftProject.String(), "/api/v1/projects/" + s.restrictedProject.String()} {
			status, _ = request(surface, s.outsider, "GET", path, nil, 0, true)
			if status != 403 && status != 404 {
				t.Fatal("read-only project reader acquired restricted object", status)
			}
		}
		anonymous, err := http.NewRequestWithContext(t.Context(), http.MethodGet, surface.server.URL+public, nil)
		resultsSourceRequire(t, err)
		response, err := surface.server.Client().Do(anonymous)
		resultsSourceRequire(t, err)
		if err := response.Body.Close(); err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != 401 {
			t.Fatal("anonymous reader acquired organisation card", response.StatusCode)
		}
		status, raw = request(surface, s.outsider, "GET", "/api/v1/projects?search="+url.QueryEscape("' OR true; -- PRIVATE-"), nil, 0, true)
		if status != 200 {
			t.Fatal("literal adversarial search failed safely", status)
		}
		page := resultAPIDecode[contract.OutcomeProjectPage](t, "OutcomeProjectPage", raw)
		if len(page.Items) != 0 || page.NextCursor != nil {
			t.Fatal("search text escaped its literal reviewed-field scope")
		}
		for _, query := range []string{"limit=101", "status=unpublished", "event_id=00000000-0000-0000-0000-000000000000"} {
			status, _ = request(surface, s.outsider, "GET", "/api/v1/projects?"+query, nil, 0, true)
			if status != 400 {
				t.Fatal("unsupported discovery control crossed schema/owner boundary", query, status)
			}
		}
	}
	view, err := f.admin.deps.Outcomes.Publication(t.Context(), r.owner.ID, s.winnerProject)
	resultsSourceRequire(t, err)
	command := map[string]any{
		"id": uuid.New(), "action": "withdraw", "source_version": view.SourceVersion, "reason": "Synthetic read-only actor attempts privileged publication mutation",
	}
	status, _ := request(f.admin, s.outsider, "PUT", private+"/publication", command, view.CurrentVersion, true)
	if status != 403 && status != 404 {
		t.Fatal("organisation showcase reader withdrew a publication", status)
	}
	after, err := f.admin.deps.Outcomes.Publication(t.Context(), r.owner.ID, s.winnerProject)
	resultsSourceRequire(t, err)
	if after.CurrentVersion != view.CurrentVersion || after.PublishedVersion == nil || view.PublishedVersion == nil || *after.PublishedVersion != *view.PublishedVersion {
		t.Fatal("denied reader mutation changed reviewed history")
	}
	// The same known current IDs and version are readable by the organiser.
	status, raw := request(f.admin, r.owner, "GET", private+"/publication", nil, 0, true)
	if status != 200 {
		t.Fatal("organiser publication positive control failed", status)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields["current_version"] == nil {
		t.Fatal("positive control did not return actual private publication")
	}
	// Execute the identical command with genuine current authority: a valid
	// mutation, not a malformed route/body, was denied to the public reader.
	status, _ = request(f.admin, r.owner, "PUT", private+"/publication", command, view.CurrentVersion, true)
	if status != 200 {
		t.Fatal("organiser exact-command positive control failed", status)
	}
	status, _ = request(f.public, s.outsider, "GET", public, nil, 0, true)
	if status != 404 {
		t.Fatal("actual withdrawn publication remained discoverable", status)
	}
	status, _ = request(f.public, s.outsider, "GET", fmt.Sprintf("/api/v1/events/%s/results/announcements", r.eventID), nil, 0, true)
	if status != 403 {
		t.Fatal("project reader acquired a narrower native result audience", status)
	}
}
