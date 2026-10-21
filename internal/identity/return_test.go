package identity_test

import (
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
)

func TestLoginReturnDestination(t *testing.T) {
	for _, path := range []string{"https://evil.example/path", "//evil.example", "/%2fevil.example", "/\\evil.example", "/%5cevil.example", "/auth/login", "/api/v1/logout", "/ops/metrics", "/x%0d%0aLocation:evil", ""} {
		if got := identity.SafeReturnTo(path); got != "/" {
			t.Errorf("unsafe destination %q accepted as %q", path, got)
		}
	}
	for _, path := range []string{"/", "/events/123", "/events/123?tab=staff"} {
		if got := identity.SafeReturnTo(path); got != path {
			t.Errorf("safe destination %q changed to %q", path, got)
		}
	}
}
