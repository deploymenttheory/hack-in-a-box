package access_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

// This contract fixture models a long-lived subscriber. #85 owns SSE transport;
// every future emission must re-evaluate the current policy before reading or
// serializing its payload, including after reconnect or a resource change.
func TestSubscriptionPolicyReevaluatesEveryDelivery(t *testing.T) {
	f := testdb.NewAuthorization(t)
	ctx := t.Context()
	f.Grant(t, f.First, f.Person.ID, "mentor", true)
	f.Grant(t, f.Second, f.Person.ID, "photographer", true)
	first := f.Resource(t, f.First, "assessment", f.Person.ID)
	second := f.Resource(t, f.Second, "assessment", f.Person.ID)
	payloadReads := 0
	emit := func(event, target uuid.UUID) error {
		if err := f.Policy.Authorize(
			ctx,
			f.Person.ID,
			event,
			target,
			access.AssessmentRead,
		); err != nil {
			return err
		}
		payloadReads++
		return nil
	}
	if err := emit(f.First, first); err != nil {
		t.Fatal(err)
	}
	if err := emit(f.Second, second); err == nil {
		t.Fatal("overlapping membership escalated assessment access")
	}
	if err := emit(f.First, second); err == nil {
		t.Fatal("subscription accepted resource from a different event")
	}
	f.Grant(t, f.First, f.Person.ID, "mentor", false)
	if err := emit(f.First, first); err == nil {
		t.Fatal("existing subscriber retained revoked grant")
	}
	f.Grant(t, f.First, f.Person.ID, "mentor", true)
	if _, err := f.Pool.Exec(
		ctx,
		"DELETE FROM resource_assignments WHERE event_id=$1",
		f.First,
	); err != nil {
		t.Fatal(err)
	}
	if err := emit(f.First, first); err == nil {
		t.Fatal("subscription retained removed assignment")
	}
	if payloadReads != 1 {
		t.Fatal("private payload read before authorization")
	}
}
