package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/automation"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestCommunicationRuntimeProducesAndConsumesApprovedInAppOccurrence(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	owner := testdb.Person(t, pool)
	person := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	eventConfig := event.DefaultConfiguration()
	eventConfig.Timezone = "UTC"
	raw, err := json.Marshal(eventConfig)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(
		ctx,
		"UPDATE events SET configuration=$2 WHERE id=$1",
		eventID,
		raw,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(
		ctx,
		"INSERT INTO competition_claims VALUES($1,$2,'confirmed')",
		eventID,
		person.ID,
	); err != nil {
		t.Fatal(err)
	}
	disabledKeys, err := secretbox.Parse("")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{PublicOrigin: "https://hack.example.test", CredentialKeys: disabledKeys}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	producer, err := newCommunicationRuntime(pool, cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := newCommunicationRuntime(pool, cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	for _, runtime := range []communicationRuntime{producer, worker} {
		runtime.Communications.Now = func() time.Time { return now }
		runtime.Automation.Now = func() time.Time { return now }
		runtime.Inbox.Now = func() time.Time { return now }
	}
	q := dbgen.New(pool)
	revision := func() int64 {
		t.Helper()
		row, readErr := q.Event(ctx, eventID)
		if readErr != nil {
			t.Fatal(readErr)
		}
		return row.Revision
	}
	template := communications.Template{
		ID:      uuid.New(),
		Name:    "Runtime message",
		Purpose: "reminder",
		Format:  "text",
		Subject: "Runtime reminder",
		Body:    "Current competitor reminder",
	}
	audience := communications.Audience{
		ID:                 uuid.New(),
		Name:               "Current confirmed competitors",
		Roles:              []string{"attendee"},
		RegistrationStates: []string{},
		Teams:              []uuid.UUID{},
	}
	for _, command := range []communications.Command{{Action: "save_template", Template: &template, Reason: "Review current message"}, {Action: "save_audience", Audience: &audience, Reason: "Review current audience"}} {
		if _, err = producer.Communications.Change(
			ctx,
			owner.ID,
			eventID,
			revision(),
			command,
		); err != nil {
			t.Fatal(err)
		}
	}
	campaignID := uuid.New()
	definition := communications.CampaignDefinition{
		ID:         campaignID,
		Name:       "Runtime campaign",
		TemplateID: template.ID,
		AudienceID: audience.ID,
		Channels:   []string{"in_app"},
		Timing: communications.CampaignTiming{
			Kind:     "fixed",
			WallTime: now.Add(time.Minute).Format("2006-01-02T15:04:05"),
		},
	}
	version := int64(0)
	for _, action := range []string{"save", "preview", "approve"} {
		command := communications.CampaignCommand{
			Action:  action,
			Version: version,
			Reason:  "Explicit runtime review",
		}
		if action == "save" {
			command.Definition = &definition
		}
		detail, changeErr := producer.Communications.ChangeCampaign(
			ctx,
			owner.ID,
			eventID,
			campaignID,
			revision(),
			command,
		)
		if changeErr != nil {
			t.Fatal(changeErr)
		}
		version = detail.Campaign.Campaign.Version
	}
	bindingID := uuid.New()
	binding := automation.Definition{
		ID:                     bindingID,
		CampaignID:             campaignID,
		CampaignVersion:        version,
		Trigger:                "scheduled",
		MaximumLatenessSeconds: 60,
	}
	page, _, err := producer.Automation.Change(
		ctx,
		owner.ID,
		eventID,
		revision(),
		automation.Command{Action: "save", Definition: &binding, Reason: "Bind runtime schedule"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = producer.Automation.Change(
		ctx,
		owner.ID,
		eventID,
		revision(),
		automation.Command{
			Action:    "approve",
			BindingID: bindingID,
			Version:   page.Bindings[0].Version,
			Reason:    "Approve current runtime schedule",
		},
	); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	if err = worker.Automation.Maintain(ctx); err != nil {
		t.Fatal(err)
	}
	current, err := worker.Automation.Read(ctx, owner.ID, eventID)
	if err != nil || len(current.Occurrences) != 1 {
		t.Fatal("separate worker did not discover durable source", err)
	}
	occurrence, err := q.AutomationOccurrence(
		ctx,
		dbgen.AutomationOccurrenceParams{EventID: eventID, ID: current.Occurrences[0].ID},
	)
	if err != nil || occurrence.OperationID == nil {
		t.Fatal("runtime failed to enqueue durable orchestration", err)
	}
	if err = worker.Jobs.Execute(ctx, *occurrence.OperationID); err != nil {
		t.Fatal(err)
	}
	inbox, err := producer.Inbox.List(ctx, person.ID, nil, nil, 20)
	if err != nil || len(inbox.Items) != 1 {
		t.Fatal("actual configured worker did not persist private participant inbox entry", err)
	}
	if err = worker.Jobs.Execute(ctx, *occurrence.OperationID); err != nil {
		t.Fatal(err)
	}
	again, err := producer.Inbox.List(ctx, person.ID, nil, nil, 20)
	if err != nil || len(again.Items) != 1 {
		t.Fatal("runtime retry duplicated participant notification", err)
	}
	var smtpIntents, teamsIntents int
	if err = pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM smtp_campaign_messages),(SELECT count(*) FROM teams_workflow_deliveries)").
		Scan(&smtpIntents, &teamsIntents); err != nil {
		t.Fatal(err)
	}
	if smtpIntents != 0 || teamsIntents != 0 {
		t.Fatal("unselected unconfigured channels received work")
	}
}
