package loadprofile

import (
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/presentation"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

// This is real native owner/transaction/query compatibility, with synthetic
// identities and a native display pairing. It is not HTTP/OIDC/SSE transport,
// production-image throughput or release latency evidence.
func TestNativeCueOwnerRevisionAndImmutableEffects(t *testing.T) {
	pool := testdb.New(t, true)
	owner := testdb.Person(t, pool)
	event := testdb.Event(t, pool, owner.ID)
	native, err := NewNativeDatabase(pool, []uuid.UUID{event})
	if err != nil {
		t.Fatal(err)
	}
	service := presentation.Service{Pool: pool}
	ctx := t.Context()
	base, err := service.Read(ctx, owner.ID, event)
	if err != nil {
		t.Fatal(err)
	}
	paired, err := service.Pair(ctx, owner.ID, event, base.Revision,
		presentation.PairInput{Label: "Load component display", Minutes: 30, Reason: "Synthetic native query proof"})
	if err != nil {
		t.Fatal(err)
	}
	cue := presentation.Cue{ID: uuid.New(), Title: "Native capacity cue", Text: "Public component source", DurationSeconds: 60}
	fingerprint, err := payloadFingerprint(cue)
	if err != nil {
		t.Fatal(err)
	}
	marker, candidate := uuid.New(), uuid.New()
	spec := WriteSpec{Kind: CueEffect, Action: "override", Event: event, Actor: owner.ID,
		Resource: event, Command: marker, Version: paired.Revision + 1, Fingerprint: fingerprint}
	changed, err := service.Change(ctx, owner.ID, event, paired.Revision,
		presentation.Command{Action: "override", Override: &cue, Reason: "load-cue:" + marker.String() + ":" + fingerprint})
	if err != nil {
		t.Fatal(err)
	}
	if changed.Revision != spec.Version || changed.Override == nil || *changed.Override != cue {
		t.Fatal("native override did not return its exact committed source")
	}
	anchor, err := native.CuePublication(ctx, event, owner.ID, spec.Version, SustainedPhase)
	if err != nil || anchor.AuditAt.IsZero() {
		t.Fatal("native publication audit missing", err)
	}
	snapshot, err := service.Snapshot(ctx, event, paired.Token)
	if err != nil || snapshot.Sequence != spec.Version || snapshot.Current == nil ||
		snapshot.Current.ID != cue.ID || snapshot.Current.Text != cue.Text {
		t.Fatal("scoped native display projection did not match committed cue", err)
	}
	// Replace the current cue using a NEW deliberately authored command. The
	// original immutable decision/audit identity must survive that replacement.
	next := cue
	next.ID, next.Text = uuid.New(), "Different later source"
	if _, err := service.Change(ctx, owner.ID, event, changed.Revision,
		presentation.Command{Action: "override", Override: &next, Reason: "Separate later reviewed cue"}); err != nil {
		t.Fatal(err)
	}
	facts, commits, err := native.Effects(ctx, map[uuid.UUID]WriteSpec{candidate: spec})
	if err != nil {
		t.Fatal(err)
	}
	fact := facts[candidate]
	if !fact.Present || fact.Effects != 1 || fact.Fingerprint != fingerprint || len(commits) != 1 {
		t.Fatal("current replacement erased or changed original native cue effect", fact, commits)
	}
}
