package app

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

// This is an explicitly partial infrastructure/authentication checkpoint. It
// cannot emit the complete L01-L18 lifecycle acceptance receipt.
func TestLifecycleComposeBootstrap(t *testing.T) {
	if os.Getenv("HACKBOX_LIFECYCLE_ISOLATED") != "synthetic-only" {
		t.Skip("run only in the isolated lifecycle Compose project")
	}
	ctx := t.Context()
	admin, err := signInLifecycle(ctx, "http://admin.localhost:8080", "http://provider.localhost:8080", "owner")
	if err != nil {
		t.Fatal(err)
	}
	public, err := signInLifecycle(ctx, "http://public.localhost:8080", "http://provider.localhost:8080", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if public.person != admin.person || public.csrf == admin.csrf {
		t.Fatal("independent portals must resolve one native person with separate session contexts")
	}
	stranger, err := signInLifecycle(ctx, "http://admin.localhost:8080", "http://provider.localhost:8080", "stranger")
	if err != nil {
		t.Fatal(err)
	}
	control, err := newLifecycleControl(os.Getenv("HACKBOX_LIFECYCLE_CONTROL"))
	if err != nil {
		t.Fatal(err)
	}
	defer control.close()
	timeline, err := control.initialTimeline(ctx)
	if err != nil {
		t.Fatal(err)
	}
	events := make([]event.View, 0, 2)
	for _, name := range []string{"Harbour", "Orchard"} {
		input := lifecycleEventInput(name, timeline)
		var created event.View
		if err := admin.call(ctx, http.MethodPost, "/api/v1/events", -1, input, &created, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if created.ID != input.ID || created.Stage != "draft" || created.OwnerID == nil || *created.OwnerID != admin.person {
			t.Fatal("normal event creation did not establish the authenticated native owner")
		}
		if err := stranger.call(ctx, http.MethodGet, "/api/v1/events/"+created.ID.String(), -1, nil, nil, http.StatusForbidden); err != nil {
			t.Fatal(err)
		}
		events = append(events, created)
	}
	denied := lifecycleEventInput("Denied", timeline)
	if err := stranger.call(ctx, http.MethodPost, "/api/v1/events", -1, denied, nil, http.StatusForbidden); err != nil {
		t.Fatal(err)
	}
	// Log out only the public session, then prove the independent admin process
	// still accepts its own session. Never persist tokens or cookies in receipts.
	if err := public.call(ctx, http.MethodPost, "/api/v1/logout", -1, nil, nil, http.StatusNoContent); err != nil {
		t.Fatal(err)
	}
	if err := public.call(ctx, http.MethodGet, "/api/v1/session", -1, nil, nil, http.StatusUnauthorized); err != nil {
		t.Fatal(err)
	}
	if err := admin.call(ctx, http.MethodGet, "/api/v1/events/"+events[0].ID.String(), -1, nil, nil, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	// A separate partial receipt prevents an infrastructure checkpoint from
	// accidentally being mistaken for completion of the issue's lifecycle.
	receipt, err := json.MarshalIndent(map[string]any{
		"checkpoint": "native-compose-bootstrap-only", "complete_lifecycle": false,
		"assertions": []string{"normal-config", "real-oidc-callback", "native-bootstrap-owner", "separate-portal-sessions", "private-draft-denied", "unprivileged-creation-denied"},
		"event_ids":  []uuid.UUID{events[0].ID, events[1].ID},
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("/artifacts", "bootstrap-only.json"), receipt, 0o600); err != nil {
		t.Fatal(err)
	}
}

func lifecycleEventInput(name string, timeline lifecycleTimeline) event.Input {
	configuration := event.DefaultConfiguration()
	configuration.Description = "Synthetic " + name + " lifecycle verification"
	configuration.Venue = "Synthetic " + name + " test venue"
	configuration.Capacity = 35
	configuration.MinimumAttendance = 30
	configuration.LiveStart = timeline.Live
	configuration.LiveEnd = timeline.Completed
	configuration.SubmissionCutoff = timeline.Cutoff
	configuration.TeamFreeze = timeline.Publication.Add(-time.Hour)
	configuration.AdmissionCutoff = configuration.TeamFreeze.Add(-time.Hour)
	return event.Input{
		ID: uuid.New(), Name: "Synthetic " + name, Slug: "lifecycle-" + uuid.NewString(),
		Configuration: configuration, AcknowledgeCompetitionRestriction: true,
	}
}
