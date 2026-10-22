package identity_test

import (
	"context"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
)

func TestDeploymentSessionsUseDistinctCookies(t *testing.T) {
	t.Parallel()
	for _, prefix := range []string{"hackbox_public", "hackbox_admin"} {
		auth, err := identity.New(context.Background(), nil, identity.Settings{CookiePrefix: prefix, Secure: true})
		if err != nil {
			t.Fatal(err)
		}
		if auth.Sessions.Cookie.Name != prefix+"_session" || !auth.Sessions.Cookie.Secure || !auth.Sessions.Cookie.HttpOnly || auth.Sessions.Cookie.Domain != "" {
			t.Fatalf("unsafe or shared cookie for %s", prefix)
		}
	}
}
