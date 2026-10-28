package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

func installLifecycleChallenge(ctx context.Context, owner *lifecycleClient, eventID uuid.UUID, mode string) (content.Page, rubric.Configuration, error) {
	return installLifecycleChallengeWithConfiguration(ctx, owner, eventID, mode, nil)
}

func installLifecycleChallengeWithConfiguration(ctx context.Context, owner *lifecycleClient, eventID uuid.UUID, mode string, configure func(rubric.Definition) (rubric.Configuration, error)) (content.Page, rubric.Configuration, error) {
	return installLifecycleChallengeWithActions(ctx, owner, eventID, mode, configure, nil)
}

func installLifecycleChallengeWithActions(ctx context.Context, owner *lifecycleClient, eventID uuid.UUID, mode string, configure func(rubric.Definition) (rubric.Configuration, error), author func(content.Input) (content.Page, error)) (content.Page, rubric.Configuration, error) {
	base := "/api/v1/events/" + eventID.String()
	definition := rubric.Default(mode, eventID)
	definition.RequiredJudgeCount = 3
	demoCategory := definition.Categories[0].ID
	definition.Prizes = []rubric.Prize{
		{ID: uuid.NewSHA1(eventID, []byte("synthetic-overall-prize")), Name: "Synthetic overall award", Description: "Explicit fixture recognition; no payment is executed"},
		{ID: uuid.NewSHA1(eventID, []byte("synthetic-demo-prize")), Name: "Synthetic demo award", Description: "Separate declared demo-category recognition", CategoryID: &demoCategory},
	}
	definition.PrivateJudgeGuidance = "PRIVATE synthetic judge guidance must never reach attendee responses"
	var approved rubric.Configuration
	if configure != nil {
		var err error
		approved, err = configure(definition)
		if err != nil {
			return content.Page{}, approved, err
		}
	} else {
		var configured rubric.Configuration
		if err := owner.call(ctx, http.MethodPost, base+"/rubric/configuration", 0,
			map[string]any{"action": "save", "definition": definition, "reason": "Synthetic exact common assessment contract"}, &configured, http.StatusOK); err != nil {
			return content.Page{}, configured, err
		}
		if err := owner.call(ctx, http.MethodPost, base+"/rubric/configuration", configured.Revision,
			map[string]any{"action": "approve", "version": configured.Reference.Version, "reason": "Explicit exact common rubric approval"}, &approved, http.StatusOK); err != nil {
			return content.Page{}, approved, err
		}
		if !approved.Approved || approved.Reference != configured.Reference {
			return content.Page{}, approved, fmt.Errorf("native rubric approval changed its exact public reference")
		}
	}
	if !approved.Approved || approved.Reference.Version < 1 {
		return content.Page{}, approved, fmt.Errorf("challenge requires an exact approved rubric")
	}
	input := content.Input{Kind: "challenge", Document: content.Document{
		Title: "Synthetic common lifecycle challenge", Markdown: "Build a synthetic working service and demonstrate its outcome evidence.",
		Audience:    access.ContentAudience{Confirmed: true, Roles: []string{}, Teams: []uuid.UUID{}},
		Attachments: []uuid.UUID{}, Rubric: &approved.Reference, RequireAcknowledgement: true,
	}}
	if author != nil {
		page, err := author(input)
		return page, approved, err
	}
	var page content.Page
	path := base + "/content/" + uuid.NewString()
	if err := owner.call(ctx, http.MethodPut, path, 0, input, &page, http.StatusOK); err != nil {
		return page, approved, err
	}
	for _, action := range []string{"review", "approve"} {
		var next content.Page
		if err := owner.call(ctx, http.MethodPost, path+"/actions", page.Revision,
			map[string]string{"action": action, "reason": "Synthetic guarded common challenge publication"}, &next, http.StatusOK); err != nil {
			return page, approved, err
		}
		page = next
	}
	return page, approved, nil
}

// This partial Compose checkpoint exercises the native worker's real common
// publication maintenance. It does not complete the full lifecycle or prove a
// normal production-image deployment, browser journey or provider delivery.
func TestLifecycleComposePublication(t *testing.T) {
	if os.Getenv("HACKBOX_LIFECYCLE_ISOLATED") != "synthetic-only" {
		t.Skip("run only in the isolated lifecycle Compose project")
	}
	ctx := t.Context()
	control, err := newLifecycleControl(os.Getenv("HACKBOX_LIFECYCLE_CONTROL"))
	if err != nil {
		t.Fatal(err)
	}
	defer control.close()
	timeline, err := control.initialTimeline(ctx)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := signInLifecycle(ctx, "http://admin.localhost:8080", "http://provider.localhost:8080", "owner")
	if err != nil {
		t.Fatal(err)
	}
	var current event.View
	if err := owner.call(ctx, http.MethodPost, "/api/v1/events", -1, lifecycleEventInput("Publication", timeline), &current, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/events/" + current.ID.String()
	var policy privacy.Settings
	if err := owner.call(ctx, http.MethodPut, base+"/privacy", current.Revision, privacy.Settings{RetentionDays: 120, DraftDays: 7}, &policy, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if policy.RetentionDays != 120 {
		t.Fatal("native HTTP policy did not preserve the explicit ninety-day review plus thirty-day response window")
	}
	cohort, err := admitLifecycleCohort(ctx, owner, current)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := allocateLifecycleTeams(ctx, owner, current.ID, &cohort, control, nil); err != nil {
		t.Fatal(err)
	}
	challenge, approved, err := installLifecycleChallenge(ctx, owner, current.ID, rubric.AllTeams)
	if err != nil {
		t.Fatal(err)
	}
	reader := cohort.clients[3] // A genuinely confirmed member of the reassigned pair.
	challengePath := base + "/content/" + challenge.ID.String()
	if err := control.advance(ctx, timeline.Publication.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{challengePath, base + "/rubric"} {
		if err := reader.call(ctx, http.MethodGet, path, -1, nil, nil, http.StatusNotFound); err != nil {
			t.Fatal(err)
		}
	}
	var hidden content.List
	if err := reader.call(ctx, http.MethodGet, base+"/content?q=Synthetic", -1, nil, &hidden, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if len(hidden.Items) != 0 || hidden.NextCursor != nil {
		t.Fatal("unpublished common challenge leaked through native search metadata")
	}
	wallAdvance := time.Now()
	if err := control.advance(ctx, timeline.Publication); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if err := owner.call(ctx, http.MethodGet, base, -1, nil, &current, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if current.PublishedAt != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if current.ChallengePublication == nil || !current.ChallengePublication.Equal(timeline.Publication) || current.PublishedAt == nil ||
		current.PublishedAt.Before(timeline.Publication) || current.PublishedAt.After(timeline.Publication.Add(time.Minute)) ||
		current.Stage != "preparation" || current.ConfirmedAttendees != 32 || timeline.Live.Sub(timeline.Publication) == 168*time.Hour {
		t.Fatal("real native worker did not publish at the independent local-calendar DST boundary")
	}
	var common json.RawMessage
	var acknowledged []uuid.UUID
	for _, actor := range []*lifecycleClient{reader, cohort.clients[5]} {
		var raw json.RawMessage
		if err := actor.call(ctx, http.MethodGet, base+"/rubric", -1, nil, &raw, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(raw, []byte("PRIVATE")) || common != nil && !bytes.Equal(common, raw) {
			t.Fatal("attendees received private guidance or different common rubric bytes")
		}
		common = raw
		var public rubric.PublicPage
		if err := json.Unmarshal(raw, &public); err != nil || public.Reference != approved.Reference {
			t.Fatal("common rubric did not retain its exact approved reference", err)
		}
		var released content.Page
		if err := actor.call(ctx, http.MethodGet, challengePath, -1, nil, &released, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if released.ReleasedVersion == nil || *released.ReleasedVersion != challenge.Version || released.Document.Rubric == nil || *released.Document.Rubric != approved.Reference {
			t.Fatal("common challenge/rubric publication was not atomic and exact-version bound")
		}
		var ack content.Page
		if err := actor.call(ctx, http.MethodPost, challengePath+"/acknowledgements", *released.ReleasedVersion, nil, &ack, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if ack.AcknowledgedAt == nil {
			t.Fatal("native exact-version acknowledgement was not retained")
		}
		acknowledged = append(acknowledged, actor.person)
	}
	digest := sha256.Sum256(common)
	receipt, err := json.MarshalIndent(map[string]any{
		"checkpoint": "native-compose-publication-only", "complete_lifecycle": false,
		"event_id": current.ID, "challenge_id": challenge.ID, "published_at": current.PublishedAt,
		"expected_local_calendar_publication": timeline.Publication, "live_start": timeline.Live,
		"calendar_interval_hours":                   timeline.Live.Sub(timeline.Publication).Hours(),
		"observed_wall_seconds_after_clock_advance": time.Since(wallAdvance).Seconds(),
		"rubric_reference":                          approved.Reference, "common_response_sha256": hex.EncodeToString(digest[:]),
		"acknowledged_people": acknowledged, "explicit_retention_days": policy.RetentionDays,
		"native_worker_publication_maintenance": true, "provider_delivery_exercised": false,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("/artifacts", "publication-only.json"), receipt, 0o600); err != nil {
		t.Fatal(err)
	}
}
