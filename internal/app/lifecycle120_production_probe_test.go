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

// The runner is a verification binary; both API targets are separately pinned
// normal images running /app/api, without test clock/control configuration.
func TestLifecycleNormalImageCalendarAPI(t *testing.T) {
	if os.Getenv("HACKBOX_LIFECYCLE_ISOLATED") != "synthetic-only" || os.Getenv("HACKBOX_LIFECYCLE_NORMAL_IMAGES") != "normal-commands" {
		t.Skip("run only against the separately configured normal-image probe")
	}
	owner, err := signInLifecycle(t.Context(), "http://admin.localhost:8080", "http://provider.localhost:8080", "owner")
	if err != nil {
		t.Fatal(err)
	}
	public, err := signInLifecycle(t.Context(), "http://public.localhost:8080", "http://provider.localhost:8080", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if owner.person != public.person || owner.csrf == public.csrf {
		t.Fatal("normal image portals did not retain independent authenticated sessions")
	}
	// Worked future example across the 2027 London DST transition. These
	// literals are independent of schedule.ChallengePublication and host tzdata.
	live := time.Date(2027, time.March, 29, 8, 0, 0, 0, time.UTC)
	expected := time.Date(2027, time.March, 22, 9, 0, 0, 0, time.UTC)
	configuration := event.DefaultConfiguration()
	configuration.Timezone = "Europe/London"
	configuration.LiveStart, configuration.LiveEnd = live, live.Add(8*time.Hour)
	configuration.SubmissionCutoff = live.Add(7 * time.Hour)
	configuration.TeamFreeze, configuration.AdmissionCutoff = expected.Add(-time.Hour), expected.Add(-2*time.Hour)
	input := event.Input{ID: uuid.New(), Name: "Synthetic normal-image calendar", Slug: "normal-calendar-" + uuid.NewString(), Configuration: configuration, AcknowledgeCompetitionRestriction: true}
	var preview struct {
		ChallengePublication *time.Time `json:"challenge_publication"`
	}
	if err := owner.call(t.Context(), http.MethodPost, "/api/v1/event-preview", -1, input, &preview, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	var created event.View
	if err := owner.call(t.Context(), http.MethodPost, "/api/v1/events", -1, input, &created, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if preview.ChallengePublication == nil || !preview.ChallengePublication.Equal(expected) ||
		created.ChallengePublication == nil || !created.ChallengePublication.Equal(expected) || created.Configuration.Timezone != "Europe/London" {
		t.Fatal("actual normal-image Go API did not preserve the independent London local-calendar publication instant")
	}
	var same event.View
	if err := public.call(t.Context(), http.MethodGet, "/api/v1/events/"+created.ID.String(), -1, nil, &same, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if same.ID != created.ID || same.ChallengePublication == nil || !same.ChallengePublication.Equal(expected) {
		t.Fatal("normal public image did not retain the native calendar projection")
	}
	receipt, err := json.MarshalIndent(map[string]any{
		"checkpoint": "normal-image-calendar-api-only", "complete_lifecycle": false,
		"event_id": created.ID, "timezone": created.Configuration.Timezone,
		"live_start": live, "publication": created.ChallengePublication, "expected_publication": expected,
		"calendar_interval_hours": live.Sub(expected).Hours(), "configured_domain_clock": false,
		"assertions": []string{"normal-api-commands", "configured-native-oidc", "independent-portal-sessions", "actual-go-preview-and-create", "normal-public-projection", "independent-167-hour-london-dst-example"},
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("/artifacts", "normal-image-calendar.json"), receipt, 0o600); err != nil {
		t.Fatal(err)
	}
}
