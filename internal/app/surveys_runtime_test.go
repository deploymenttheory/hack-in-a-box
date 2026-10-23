package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/automation"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/inbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/surveys"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestActualApprovedSurveyCampaignKeepsBodyAndNativeReaderAfterAnswerAndClose(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	owner, person, other := testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	now := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	dates := event.DefaultConfiguration()
	dates.LiveStart = now.Add(-12 * time.Hour)
	dates.LiveEnd = now.Add(-4 * time.Hour)
	dates.SubmissionCutoff = now.Add(-5 * time.Hour)
	dates.TeamFreeze = now.AddDate(0, 0, -8)
	dates.AdmissionCutoff = dates.TeamFreeze.Add(-time.Hour)
	raw, _ := json.Marshal(dates)
	if _, err := pool.Exec(
		ctx,
		"UPDATE events SET stage='completed',configuration=$2,completed_at=$3 WHERE id=$1",
		eventID,
		raw,
		now.Add(-3*time.Hour),
	); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{person.ID, other.ID} {
		if _, err := pool.Exec(
			ctx,
			"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
			eventID,
			id,
		); err != nil {
			t.Fatal(err)
		}
	}
	keys, err := secretbox.Parse("")
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	producer, err := newCommunicationRuntime(
		pool,
		config.Config{
			PublicOrigin:    "https://hack.example.test",
			WorkspaceOrigin: "https://hack.example.test",
			CredentialKeys:  keys,
		},
		logger,
	)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := newCommunicationRuntime(
		pool,
		config.Config{
			PublicOrigin:    "https://hack.example.test",
			WorkspaceOrigin: "https://hack.example.test",
			CredentialKeys:  keys,
		},
		logger,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, runtime := range []communicationRuntime{producer, worker} {
		runtime.Communications.Now = func() time.Time { return now }
		runtime.Automation.Now = func() time.Time { return now }
		runtime.Surveys.Now = func() time.Time { return now }
		runtime.Inbox.Now = func() time.Time { return now }
	}
	q := dbgen.New(pool)
	revision := func() int64 {
		t.Helper()
		row, e := q.Event(ctx, eventID)
		if e != nil {
			t.Fatal(e)
		}
		return row.Revision
	}
	id, question := uuid.New(), uuid.New()
	definition := surveys.Definition{
		Form: forms.Definition{
			ID:       id,
			Name:     "Identifiable post-event feedback",
			Audience: "attendee",
			Fields:   []forms.Field{{ID: question, Label: "Learning rating", Type: "rating"}},
		},
		OpensAt:       now.Add(time.Minute),
		ClosesAt:      now.Add(10 * time.Minute),
		MinimumCohort: 5,
		Measures:      []surveys.QuestionMeasure{},
	}
	cfg, err := producer.Surveys.Change(
		ctx,
		owner.ID,
		eventID,
		id,
		0,
		surveys.Command{Action: "save", Definition: &definition},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"approve", "publish"} {
		cfg, err = producer.Surveys.Change(
			ctx,
			owner.ID,
			eventID,
			id,
			cfg.Version,
			surveys.Command{Action: action},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	var notificationCount int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM inbox_notifications").
		Scan(&notificationCount); err != nil ||
		notificationCount != 0 {
		t.Fatal("survey publication bypassed approved messaging", err)
	}
	template := communications.Template{
		ID:      uuid.New(),
		Name:    "Reviewed survey invitation",
		Purpose: "feedback",
		Format:  "text",
		Subject: "Share post-event feedback",
		Body:    "Please share feedback. Your response is identifiable to current organisers, not anonymous.",
	}
	audience := communications.Audience{
		ID:                 uuid.New(),
		Name:               "Current invited competitors",
		Roles:              []string{"attendee"},
		RegistrationStates: []string{},
		Teams:              []uuid.UUID{},
	}
	for _, command := range []communications.Command{{Action: "save_template", Template: &template, Reason: "Review invitation wording"}, {Action: "save_audience", Audience: &audience, Reason: "Review role audience"}} {
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
	zone, err := time.LoadLocation(dates.Timezone)
	if err != nil {
		t.Fatal(err)
	}
	campaignID := uuid.New()
	campaign := communications.CampaignDefinition{
		ID:         campaignID,
		Name:       "Post-event questionnaire invitation",
		TemplateID: template.ID,
		AudienceID: audience.ID,
		Channels:   []string{"in_app"},
		Timing: communications.CampaignTiming{
			Kind:     "fixed",
			WallTime: now.Add(time.Minute).In(zone).Format("2006-01-02T15:04:05"),
		},
	}
	version := int64(0)
	for _, action := range []string{"save", "preview", "approve"} {
		command := communications.CampaignCommand{
			Action:  action,
			Version: version,
			Reason:  "Explicit invitation message/audience/timing review",
		}
		if action == "save" {
			command.Definition = &campaign
		}
		result, e := producer.Communications.ChangeCampaign(
			ctx,
			owner.ID,
			eventID,
			campaignID,
			revision(),
			command,
		)
		if e != nil {
			t.Fatal(e)
		}
		version = result.Campaign.Campaign.Version
	}
	binding := automation.Definition{
		ID:                     uuid.New(),
		CampaignID:             campaignID,
		CampaignVersion:        version,
		Trigger:                "survey.open",
		SourceID:               &id,
		SourceVersion:          cfg.Version,
		MaximumLatenessSeconds: 3600,
	}
	page, _, err := producer.Automation.Change(
		ctx,
		owner.ID,
		eventID,
		revision(),
		automation.Command{
			Action:     "save",
			Definition: &binding,
			Reason:     "Bind actual open native questionnaire",
		},
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
			BindingID: binding.ID,
			Version:   page.Bindings[0].Version,
			Reason:    "Approve native invitation trigger",
		},
	); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	if err = worker.Automation.Maintain(ctx); err != nil {
		t.Fatal(err)
	}
	page, err = worker.Automation.Read(ctx, owner.ID, eventID)
	if err != nil || len(page.Occurrences) != 1 {
		t.Fatal("actual survey source not discovered", err)
	}
	occurrence, err := q.AutomationOccurrence(
		ctx,
		dbgen.AutomationOccurrenceParams{EventID: eventID, ID: page.Occurrences[0].ID},
	)
	if err != nil || occurrence.OperationID == nil {
		t.Fatalf("native occurrence not queued: %+v error%v", page.Occurrences, err)
	}
	for range 2 {
		if err = worker.Jobs.Execute(ctx, *occurrence.OperationID); err != nil {
			t.Fatal(err)
		}
	}
	items, err := producer.Inbox.List(ctx, person.ID, nil, nil, 10)
	if err != nil || len(items.Items) != 1 {
		t.Fatal("approved invitation did not create one private inbox item", err)
	}
	detail, err := producer.Inbox.Open(ctx, person.ID, items.Items[0].ID, nil)
	if err != nil || detail.Body != template.Body || detail.Destination == nil ||
		detail.Destination.Kind != "survey" ||
		detail.Destination.ResourceID != id {
		t.Fatal("approved body or canonical native destination changed", err)
	}
	reader, err := producer.Surveys.ReadOwn(ctx, person.ID, eventID, detail.Destination.ResourceID)
	if err != nil || reader.Version != cfg.Version ||
		reader.ResponseVisibility != surveys.ResponseVisibility {
		t.Fatal("destination did not reach actual current native survey", err)
	}
	if _, err = producer.Surveys.Respond(
		ctx,
		person.ID,
		eventID,
		id,
		0,
		surveys.ResponseCommand{
			CommandID:   uuid.New(),
			FormVersion: cfg.Version,
			Answers:     forms.Answers{question.String(): json.RawMessage(`8`)},
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err = producer.Inbox.Open(ctx, person.ID, items.Items[0].ID, nil); err != nil {
		t.Fatal("answering erased legitimate retained invitation history", err)
	}
	now = definition.ClosesAt
	err = database.Within(ctx, pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		_, e := worker.Inbox.QueueApprovedOccurrenceInTransaction(
			ctx,
			tx,
			eventID,
			campaignID,
			version,
			communications.DeliveryScope{OccurrenceID: occurrence.ID},
		)
		return e
	})
	if err == nil {
		t.Fatal("closed survey permitted another invitation queue")
	}
	if _, err = producer.Inbox.Open(ctx, person.ID, items.Items[0].ID, nil); err != nil {
		t.Fatal("close destroyed current own historical reader scope", err)
	}
	reader, err = producer.Surveys.ReadOwn(ctx, person.ID, eventID, id)
	if err != nil || reader.CanRespond || reader.Response == nil {
		t.Fatal("native closed survey reader incorrect", err)
	}
	// Malformed metadata cannot turn retained campaign approval into a different
	// native resource, version or self-test route. Each synthetic corrupt row is
	// immutable and separate; the real approved invitation is left untouched.
	var originalProvenance []byte
	if err = pool.QueryRow(ctx, "SELECT provenance FROM inbox_notifications WHERE id=$1", items.Items[0].ID).
		Scan(&originalProvenance); err != nil {
		t.Fatal(err)
	}
	foreignEvent := testdb.Event(t, pool, owner.ID)
	if _, err = pool.Exec(
		ctx,
		"UPDATE events SET stage='completed',configuration=$2,completed_at=$3 WHERE id=$1",
		foreignEvent,
		raw,
		now.Add(-time.Hour),
	); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(
		ctx,
		"INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')",
		foreignEvent,
		person.ID,
	); err != nil {
		t.Fatal(err)
	}
	foreignID := uuid.New()
	foreignDefinition := definition
	foreignDefinition.Form.ID = foreignID
	foreignDefinition.OpensAt = now.Add(-time.Minute)
	foreignDefinition.ClosesAt = now.Add(time.Hour)
	foreign, err := producer.Surveys.Change(
		ctx,
		owner.ID,
		foreignEvent,
		foreignID,
		0,
		surveys.Command{Action: "save", Definition: &foreignDefinition},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"approve", "publish"} {
		foreign, err = producer.Surveys.Change(
			ctx,
			owner.ID,
			foreignEvent,
			foreignID,
			foreign.Version,
			surveys.Command{Action: action},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name string
		ref  *communications.ResourceReference
		self bool
	}{
		{
			"cross-event",
			&communications.ResourceReference{
				Kind:    "survey",
				ID:      foreignID,
				Version: foreign.Version,
			},
			false,
		},
		{
			"wrong-owner",
			&communications.ResourceReference{Kind: "help", ID: id, Version: cfg.Version},
			false,
		},
		{
			"missing-resource",
			&communications.ResourceReference{Kind: "survey", ID: uuid.New(), Version: cfg.Version},
			false,
		},
		{
			"missing-version",
			&communications.ResourceReference{Kind: "survey", ID: id, Version: 0},
			false,
		},
		{
			"wrong-version",
			&communications.ResourceReference{Kind: "survey", ID: id, Version: cfg.Version + 1},
			false,
		},
		{
			"self-test",
			&communications.ResourceReference{Kind: "survey", ID: id, Version: cfg.Version},
			true,
		},
	}
	for _, test := range cases {
		var provenance communications.InboxProvenance
		if err = json.Unmarshal(originalProvenance, &provenance); err != nil {
			t.Fatal(err)
		}
		provenance.Resource = test.ref
		provenance.SelfOnly = test.self
		raw, e := json.Marshal(provenance)
		if e != nil {
			t.Fatal(e)
		}
		badID := uuid.New()
		kind := "campaign"
		if test.self {
			kind = "test_self"
		}
		if _, err = pool.Exec(
			ctx,
			`INSERT INTO inbox_notifications(id,event_id,recipient_id,source_kind,source_id,source_version,provenance,subject,body,format,occurrence_id,scope_binding) SELECT $2,event_id,recipient_id,$3,source_id,source_version,$4,subject,body,format,$5,scope_binding FROM inbox_notifications WHERE id=$1`,
			items.Items[0].ID,
			badID,
			kind,
			raw,
			uuid.New(),
		); err != nil {
			t.Fatal(err)
		}
		if _, err = producer.Inbox.Open(
			ctx,
			person.ID,
			badID,
			nil,
		); !errors.Is(
			err,
			access.ErrNotFound,
		) {
			t.Fatal("malformed native reference exposed a notification", test.name, err)
		}
	}
	ownerBefore := producer.Inbox.Owners["survey"]
	producer.Inbox.Owners["survey"] = wrongSurveyOwner{}
	if _, err = producer.Inbox.Open(
		ctx,
		person.ID,
		items.Items[0].ID,
		nil,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("wrong native destination kind escaped allowlist", err)
	}
	producer.Inbox.Owners["survey"] = ownerBefore
	if _, err = pool.Exec(
		ctx,
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		eventID,
		person.ID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = producer.Inbox.Open(
		ctx,
		person.ID,
		items.Items[0].ID,
		nil,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("withdrawn recipient retained native notification scope", err)
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM inbox_notifications").
		Scan(&notificationCount); err != nil ||
		notificationCount != 2+len(cases) {
		t.Fatal(
			"approved campaign emitted second native-notification bypass",
			notificationCount,
			err,
		)
	}
}

type wrongSurveyOwner struct{}

func (wrongSurveyOwner) AuthorizeNotification(
	context.Context,
	pgx.Tx,
	uuid.UUID,
	uuid.UUID,
	inbox.Source,
) (inbox.Destination, error) {
	return inbox.Destination{Kind: "help", ResourceID: uuid.New()}, nil
}
