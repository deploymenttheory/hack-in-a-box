package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestCommunicationFactoryUsesCanonicalWorkspaceAcrossSeparateSurfaces(t *testing.T) {
	pool := testdb.New(t, true)
	owner := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	const workspace = "http://127.0.0.1:8080"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	revisions := map[string]string{}
	runtimes := map[string]communicationRuntime{}
	for _, surface := range []struct{ name, origin string }{{"public", workspace}, {"admin", "http://127.0.0.1:8081"}, {"worker", workspace}} {
		t.Run(surface.name, func(t *testing.T) {
			values := map[string]string{
				"HACKBOX_SURFACE":             "public",
				"HACKBOX_ENV":                 "test",
				"HACKBOX_PUBLIC_ORIGIN":       surface.origin,
				"HACKBOX_WORKSPACE_ORIGIN":    workspace,
				"HACKBOX_SMTP_HOST":           "relay.example.test",
				"HACKBOX_SMTP_USERNAME":       "synthetic-user",
				"HACKBOX_SMTP_PASSWORD":       "synthetic-test-only",
				"HACKBOX_SMTP_FROM":           "sender@example.test",
				"HACKBOX_SMTP_MESSAGE_DOMAIN": "example.test",
			}
			if surface.name == "admin" {
				values["HACKBOX_SURFACE"] = "admin"
			}
			cfg, err := config.Load(func(key string) string { return values[key] })
			if err != nil {
				t.Fatal(err)
			}
			runtime, err := newCommunicationRuntime(pool, cfg, logger)
			if err != nil {
				t.Fatal(err)
			}
			ready, err := runtime.SMTP.Readiness(t.Context(), eventID, owner.ID)
			if err != nil || !ready.Available || ready.ConfigRevision == "" {
				t.Fatal("configured factory did not provide real SMTP readiness")
			}
			revisions[surface.name] = ready.ConfigRevision
			runtimes[surface.name] = runtime
			if runtime.SMTP.Origin != workspace {
				t.Errorf(
					"attendee workspace links use per-surface origin %q instead of %q",
					runtime.SMTP.Origin,
					workspace,
				)
			}
			if cfg.PublicOrigin != surface.origin {
				t.Fatal("canonical workspace changed the per-surface auth/session origin")
			}
		})
	}
	if revisions["admin"] != revisions["worker"] || revisions["public"] != revisions["worker"] {
		t.Fatal("admin-approved SMTP provider revision differs from worker/public runtime")
	}
	assertCanonicalCampaignCalendar(
		t,
		runtimes["admin"],
		runtimes["worker"],
		eventID,
		owner.ID,
		workspace,
	)
}

func assertCanonicalCampaignCalendar(
	t *testing.T,
	admin, worker communicationRuntime,
	eventID, ownerID uuid.UUID,
	workspace string,
) {
	t.Helper()
	pool := admin.SMTP.Pool
	now := time.Now().UTC().Truncate(time.Microsecond)
	configuration := event.DefaultConfiguration()
	configuration.LiveStart = now.AddDate(0, 1, 0)
	configuration.LiveEnd = configuration.LiveStart.Add(8 * time.Hour)
	configuration.SubmissionCutoff = configuration.LiveEnd.Add(-time.Hour)
	configuration.TeamFreeze = configuration.LiveStart.AddDate(0, 0, -8)
	configuration.AdmissionCutoff = configuration.TeamFreeze.Add(-time.Hour)
	data, _ := json.Marshal(configuration)
	if _, err := pool.Exec(
		t.Context(),
		"UPDATE events SET configuration=$2,stage='registration' WHERE id=$1",
		eventID,
		data,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(
		t.Context(),
		"INSERT INTO event_gate_facts(event_id) VALUES($1)",
		eventID,
	); err != nil {
		t.Fatal(err)
	}
	q := dbgen.New(pool)
	if err := q.ConfirmPersonEmail(
		t.Context(),
		dbgen.ConfirmPersonEmailParams{PersonID: ownerID, Mailbox: "organiser@example.test"},
	); err != nil {
		t.Fatal(err)
	}
	for _, runtime := range []communicationRuntime{admin, worker} {
		runtime.Communications.Now = func() time.Time { return now }
		runtime.SMTP.Now = func() time.Time { return now }
	}
	revision := func() int64 {
		row, err := q.Event(t.Context(), eventID)
		if err != nil {
			t.Fatal(err)
		}
		return row.Revision
	}
	template := communications.Template{
		ID:      uuid.New(),
		Name:    "Canonical workspace",
		Purpose: "reminder",
		Format:  "text",
		Subject: "Event reminder",
		Body:    "{{event.name}}",
	}
	audience := communications.Audience{
		ID:                 uuid.New(),
		Name:               "Organisers",
		Roles:              []string{"organiser"},
		RegistrationStates: []string{},
		Teams:              []uuid.UUID{},
	}
	for _, command := range []communications.Command{{Action: "save_template", Template: &template}, {Action: "save_audience", Audience: &audience}} {
		command.Reason = "Synthetic canonical workspace regression"
		if _, err := admin.Communications.Change(
			t.Context(),
			ownerID,
			eventID,
			revision(),
			command,
		); err != nil {
			t.Fatal(err)
		}
	}
	definition := communications.CampaignDefinition{
		ID:              uuid.New(),
		Name:            "Calendar origin",
		TemplateID:      template.ID,
		AudienceID:      audience.ID,
		Channels:        []string{"smtp"},
		IncludeCalendar: true,
		Timing: communications.CampaignTiming{
			Kind:   "relative",
			Anchor: "live_start",
			Days:   -1,
		},
	}
	var campaign communications.Campaign
	for _, action := range []string{"save", "preview", "approve"} {
		command := communications.CampaignCommand{
			Action:  action,
			Version: campaign.Version,
			Reason:  "Synthetic reviewed origin",
		}
		if action == "save" {
			command.Definition = &definition
		}
		result, err := admin.Communications.ChangeCampaign(
			t.Context(),
			ownerID,
			eventID,
			definition.ID,
			revision(),
			command,
		)
		if err != nil {
			t.Fatal(err)
		}
		campaign = result.Campaign.Campaign
	}
	now = campaign.Approval.DueAt
	// Queue through the worker factory using the admin's exact approved revision.
	// No worker is started and no SMTP network effect occurs.
	err := database.Within(t.Context(), pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		count, err := worker.SMTP.QueueApprovedCampaignInTransaction(
			t.Context(),
			tx,
			eventID,
			definition.ID,
			campaign.Version,
		)
		if err == nil && count != 1 {
			t.Fatalf("queued=%d want1", count)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var snapshot []byte
	if err = pool.QueryRow(t.Context(), "SELECT snapshot FROM smtp_campaign_messages WHERE event_id=$1", eventID).
		Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	var retained struct {
		Message maildelivery.Message `json:"message"`
	}
	if err = json.Unmarshal(snapshot, &retained); err != nil {
		t.Fatal(err)
	}
	if retained.Message.Calendar == nil ||
		retained.Message.Calendar.WorkspaceURL != workspace+"/events/"+eventID.String() {
		t.Fatal("retained attendee calendar does not point to canonical public workspace")
	}
}
