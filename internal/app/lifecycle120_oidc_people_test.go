package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestLifecycleOIDCPeopleAreImmutableExplicitFixtureInputs(t *testing.T) {
	people := map[string]lifecyclePerson{"load-e01-p001": {Subject: "load-e01-p001", Name: "Synthetic load person", Email: "load-e01-p001@example.test"}}
	clients := map[string]string{"load-public": "https://public.example.test/auth/callback"}
	provider, err := newLifecycleOIDCForPeople("https://issuer.example.test", clients, people)
	if err != nil {
		t.Fatal(err)
	}
	people["load-e01-p001"] = lifecyclePerson{Subject: "other", Name: "Changed", Email: "changed@example.test"}
	people["new-person"] = lifecyclePerson{Subject: "new-person"}
	clients["load-public"] = "https://foreign.example.test/callback"
	clients["new-client"] = "https://foreign.example.test/callback"
	if len(provider.people) != 1 || provider.people["load-e01-p001"].Subject != "load-e01-p001" || provider.people["load-e01-p001"].Email != "load-e01-p001@example.test" {
		t.Fatal("caller mutation changed the immutable known fixture identity set")
	}
	if len(provider.clients) != 1 || provider.clients["load-public"] != "https://public.example.test/auth/callback" {
		t.Fatal("caller mutation changed the exact fixture client redirect allowlist")
	}
}

func TestLifecycleDefaultPeopleRemainBounded(t *testing.T) {
	provider, err := newLifecycleOIDC("https://issuer.example.test", map[string]string{"lifecycle-public": "https://public.example.test/auth/callback"})
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.people) != 52 || provider.people["attendee-40"].Subject != "attendee-40" || provider.people["owner"].Subject != "owner" {
		t.Fatal("default40attendee+12role actor set changed")
	}
	if _, exists := provider.people["attendee-41"]; exists {
		t.Fatal("load fixture widened the lifecycle default actor set")
	}
	if _, valid := lifecycleGitHubActor("attendee-37"); valid {
		t.Fatal("OIDC factory widened the independent GitHub36-actor contract")
	}
}

func TestLifecycleOIDCExplicitCohortCannotAcquireAnotherSource(t *testing.T) {
	clients := map[string]string{"load-public": "https://public.example.test/auth/callback"}
	person := func(subject string) lifecyclePerson {
		return lifecyclePerson{Subject: subject, Name: "Synthetic load", Email: subject + "@example.test"}
	}
	first, err := newLifecycleOIDCForPeople("https://issuer.example.test", clients, map[string]lifecyclePerson{"load-e01-p001": person("load-e01-p001")})
	if err != nil {
		t.Fatal(err)
	}
	second, err := newLifecycleOIDCForPeople("https://issuer.example.test", clients, map[string]lifecyclePerson{"load-e02-p001": person("load-e02-p001")})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.people) != 1 || len(second.people) != 1 {
		t.Fatal("explicit cohort silently acquired default or other-source identities")
	}
	for _, provider := range []*lifecycleOIDC{first, second} {
		query := url.Values{"client_id": {"load-public"}, "redirect_uri": {"https://public.example.test/auth/callback"}, "response_type": {"code"}, "scope": {"openid profile email"}, "code_challenge": {strings.Repeat("A", 43)}, "code_challenge_method": {"S256"}, "state": {strings.Repeat("s", 32)}, "nonce": {strings.Repeat("n", 32)}, "actor": {"owner"}}
		denied := httptest.NewRecorder()
		provider.ServeHTTP(denied, httptest.NewRequest(http.MethodGet, "/authorize?"+query.Encode(), nil))
		if denied.Code != http.StatusBadRequest || !strings.Contains(denied.Body.String(), "unknown synthetic identity") || len(provider.codes) != 0 {
			t.Fatal("explicit source did not reject the unknown actor at its own identity boundary")
		}
		for subject := range provider.people {
			query.Set("actor", subject)
			accepted := httptest.NewRecorder()
			provider.ServeHTTP(accepted, httptest.NewRequest(http.MethodGet, "/authorize?"+query.Encode(), nil))
			target, err := url.Parse(accepted.Header().Get("Location"))
			if err != nil || accepted.Code != http.StatusFound || target.Scheme+"://"+target.Host+target.Path != "https://public.example.test/auth/callback" || target.Query().Get("state") != query.Get("state") || len(provider.codes) != 1 {
				t.Fatal("known explicit actor lost the normal exact client/state authorization path")
			}
		}
	}
	if _, exists := first.people["load-e02-p001"]; exists {
		t.Fatal("second explicit cohort changed the first identity set")
	}
	if _, exists := second.people["load-e01-p001"]; exists {
		t.Fatal("first explicit cohort leaked into the second identity set")
	}
}

func TestLifecycleOIDCPeopleRejectDuplicateOrMismatchedIdentityInputs(t *testing.T) {
	for name, people := range map[string]map[string]lifecyclePerson{
		"empty":             {},
		"subject_mismatch":  {"one": {Subject: "two", Name: "Synthetic", Email: "one@example.test"}},
		"empty_name":        {"one": {Subject: "one", Email: "one@example.test"}},
		"empty_mailbox":     {"one": {Subject: "one", Name: "Synthetic"}},
		"duplicate_mailbox": {"one": {Subject: "one", Name: "Synthetic one", Email: "shared@example.test"}, "two": {Subject: "two", Name: "Synthetic two", Email: "SHARED@example.test"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := newLifecycleOIDCForPeople("https://issuer.example.test", map[string]string{"load-public": "https://public.example.test/auth/callback"}, people); err == nil {
				t.Fatal("ambiguous explicit fixture people were accepted")
			}
		})
	}
}
