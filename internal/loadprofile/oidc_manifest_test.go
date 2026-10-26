package loadprofile

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOIDCManifestIsExplicitFixtureDataNotSeededNativePeople(t *testing.T) {
	clients := map[string]string{"load-public": "https://public.load.test/auth/callback", "load-admin": "https://admin.load.test/auth/callback"}
	manifest, err := OIDCManifestForCohort(syntheticReleaseCohort(), "https://issuer.load.test", clients)
	if err != nil || len(manifest.People) != 1000 {
		t.Fatal("declared cohort did not become exact external issuer inputs", err)
	}
	clients["load-public"] = "https://changed.load.test/auth/callback"
	if manifest.Clients["load-public"] != "https://public.load.test/auth/callback" {
		t.Fatal("caller changed frozen issuer client inputs")
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := ParseOIDCFixtureManifest(raw); err != nil || len(decoded.People) != 1000 {
		t.Fatal("exact bounded manifest failed round trip", err)
	}
	for _, invalid := range [][]byte{
		append(append([]byte(nil), raw...), []byte(`{}`)...),
		[]byte(strings.Replace(string(raw), `"profile":"release"`, `"native_session_seed":true,"profile":"release"`, 1)),
	} {
		if _, err := ParseOIDCFixtureManifest(invalid); err == nil {
			t.Fatal("extra control or trailing JSON admitted to external issuer manifest")
		}
	}
	for subject := range manifest.People {
		delete(manifest.People, subject)
		break
	}
	if err := manifest.Validate(); err == nil {
		t.Fatal("999 known external subjects became the declared release cohort")
	}
}
