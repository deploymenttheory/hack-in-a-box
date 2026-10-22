package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestInAppRuntimeSharesProducerAndDurableConsumerWithSMTP(t *testing.T) {
	pool := testdb.New(t, true)
	provisionalSQL, readErr := os.ReadFile("../automation/schema.provisional.sql")
	if readErr != nil {
		t.Fatal(readErr)
	}
	if _, applyErr := pool.Exec(t.Context(), string(provisionalSQL)); applyErr != nil {
		t.Fatal(applyErr)
	}
	ctx := t.Context()
	cfg, err := config.Load(func(key string) string {
		if key == "HACKBOX_ENV" {
			return "test"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := newCommunicationRuntime(
		pool,
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Inbox.Communications != runtime.Communications ||
		runtime.Communications.Jobs != runtime.Jobs ||
		runtime.SMTP == nil {
		t.Fatal("producer and consumer do not share the complete runtime")
	}
	owner := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	configuration := event.DefaultConfiguration()
	configuration.LiveStart = time.Now().UTC().Add(24 * time.Hour)
	configuration.LiveEnd = configuration.LiveStart.Add(8 * time.Hour)
	configuration.SubmissionCutoff = configuration.LiveEnd.Add(-time.Hour)
	raw, _ := json.Marshal(configuration)
	if _, err = pool.Exec(
		ctx,
		"UPDATE events SET configuration=$2 WHERE id=$1",
		eventID,
		raw,
	); err != nil {
		t.Fatal(err)
	}
	revision := func() int64 {
		t.Helper()
		row, readErr := dbgen.New(pool).Event(ctx, eventID)
		if readErr != nil {
			t.Fatal(readErr)
		}
		return row.Revision
	}
	template := communications.Template{
		ID:      uuid.New(),
		Name:    "Runtime synthetic message",
		Purpose: "reminder",
		Format:  "text",
		Subject: "Hello {{person.name}}",
		Body:    "Local synthetic test for {{person.name}}",
	}
	audience := communications.Audience{
		ID:                 uuid.New(),
		Name:               "Synthetic attendee selector",
		Roles:              []string{"attendee"},
		RegistrationStates: []string{},
		Teams:              []uuid.UUID{},
	}
	for _, command := range []communications.Command{{Action: "save_template", Template: &template}, {Action: "save_audience", Audience: &audience}} {
		command.Reason = "Verify shared production composition"
		if _, err = runtime.Communications.Change(
			ctx,
			owner.ID,
			eventID,
			revision(),
			command,
		); err != nil {
			t.Fatal(err)
		}
	}
	definition := communications.CampaignDefinition{
		ID:         uuid.New(),
		Name:       "Runtime local test",
		TemplateID: template.ID,
		AudienceID: audience.ID,
		Channels:   []string{"in_app"},
		Timing:     communications.CampaignTiming{Kind: "relative", Anchor: "live_start"},
	}
	campaign, err := runtime.Communications.ChangeCampaign(
		ctx,
		owner.ID,
		eventID,
		definition.ID,
		revision(),
		communications.CampaignCommand{
			Action:     "save",
			Definition: &definition,
			Reason:     "Verify source-bound local intent",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	testIntent, err := runtime.Communications.ChangeCampaign(
		ctx,
		owner.ID,
		eventID,
		definition.ID,
		revision(),
		communications.CampaignCommand{
			Action:  "test_self",
			Version: campaign.Campaign.Campaign.Version,
			Channel: "in_app",
			Reason:  "Verify actual fixed-recipient handler",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if testIntent.TestRequestID == nil {
		t.Fatal("no durable local intent")
	}
	var operation uuid.UUID
	if err = pool.QueryRow(ctx, "SELECT id FROM operations WHERE event_id=$1 AND target_id=$2", eventID, *testIntent.TestRequestID).
		Scan(&operation); err != nil {
		t.Fatal(err)
	}
	if err = runtime.Jobs.Execute(ctx, operation); err != nil {
		t.Fatal(err)
	}
	page, err := runtime.Inbox.List(ctx, owner.ID, nil, nil, 10)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("runtime handler failed local delivery: %+v %v", page, err)
	}
	detail, err := runtime.Inbox.Open(ctx, owner.ID, page.Items[0].ID, nil)
	if err != nil || !strings.HasPrefix(detail.Subject, "[TEST]") ||
		!strings.Contains(detail.Body, owner.DisplayName) {
		t.Fatal("runtime self-test was not the requesting organiser's message")
	}
}
