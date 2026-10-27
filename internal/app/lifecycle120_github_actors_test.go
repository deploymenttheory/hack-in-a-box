package app

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestLifecycleGitHubExplicitActorsPreserveDefaultAndCopyInputs(t *testing.T) {
	canonical, err := newLifecycleGitHub(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(canonical.actors) != 36 || canonical.actors["mentor-a"] != 0 || canonical.logins[37] != "" {
		t.Fatal("canonical participant provider expanded implicitly")
	}
	actors := lifecycleDefaultGitHubActors()
	actors["mentor-a"] = 37
	actors["mentor-b"] = 38
	fixture, err := newLifecycleGitHubForActors(t.TempDir(), actors)
	if err != nil {
		t.Fatal(err)
	}
	actors["mentor-a"] = 99
	delete(actors, "attendee-01")
	actors["unknown"] = 37
	if len(fixture.actors) != 38 || fixture.actors["mentor-a"] != 37 || fixture.actors["attendee-01"] != 1 || fixture.actors["unknown"] != 0 || fixture.actors["attendee-37"] != 0 || fixture.logins[37] != "lifecycle-mentor-a" || fixture.logins[38] != "lifecycle-mentor-b" {
		t.Fatal("explicit actors alias input or substitute numeric/login ownership")
	}
}

func TestLifecycleGitHubExplicitActorsRejectAmbiguousIdentity(t *testing.T) {
	for _, actors := range []map[string]int{nil, {}, {"mentor-a": 0}, {"mentor-a": 10001}, {"mentor-a": 37, "mentor-b": 37}, {"": 1}, {"../unknown": 1}, {"Mentor-a": 1}} {
		if _, err := newLifecycleGitHubForActors(t.TempDir(), actors); err == nil {
			t.Fatal("ambiguous explicit actor configuration accepted")
		}
	}
}

func TestLifecycleGitHubActorAuthorizationFailsClosed(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		actors := lifecycleDefaultGitHubActors()
		if explicit {
			actors["mentor-a"] = 37
		}
		fixture, err := newLifecycleGitHubForActors(t.TempDir(), actors)
		if err != nil {
			t.Fatal(err)
		}
		for _, actor := range []string{"unknown", "attendee-37", "mentor-b", "mentor-a"} {
			query := url.Values{"actor": {actor}, "redirect_uri": {"http://public.localhost:8080/auth/github/callback"}, "client_id": {"lifecycle-github"}, "response_type": {"code"}, "state": {"synthetic-state-at-least-thirty-two-bytes"}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(make([]byte, 32))}}
			response := httptest.NewRecorder()
			fixture.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/github/login/oauth/authorize?"+query.Encode(), nil))
			want := http.StatusBadRequest
			if explicit && actor == "mentor-a" {
				want = http.StatusFound
			}
			if response.Code != want {
				t.Fatal("explicit provider actor authority changed", actor, response.Code, want)
			}
			if want == http.StatusBadRequest && (len(fixture.codes) != 0 || len(fixture.webSessions) != 0) {
				t.Fatal("unknown identity acquired a code or external browser session")
			}
		}
	}
}
