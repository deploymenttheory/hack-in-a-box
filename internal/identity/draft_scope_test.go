package identity_test

import (
	"testing"

	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
)

func TestDraftScopeSeparatesSessionsForTheSamePersonWithoutExposingTokens(t *testing.T) {
	sessions := scs.New()
	auth := identity.Auth{Sessions: sessions}
	person := uuid.NewString()
	first, err := sessions.Load(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	sessions.Put(first, "person_id", person)
	scope := auth.DraftScope(first)
	token, _, err := sessions.Commit(first)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := sessions.Load(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	if scope == uuid.Nil || auth.DraftScope(loaded) != scope || scope.String() == token {
		t.Fatal("draft scope was not stable isolated metadata")
	}
	second, err := sessions.Load(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	sessions.Put(second, "person_id", person)
	if auth.DraftScope(second) == scope {
		t.Fatal("independent sessions for one person shared a draft scope")
	}
}
