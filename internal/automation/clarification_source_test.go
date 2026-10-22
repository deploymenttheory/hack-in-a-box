package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/inbox"
)

// These adapters delegate to the same native owner and local inbox contracts as
// app composition; no synthetic publication row or recipient grant is supplied.
type nativeAnswerInbox struct {
	local   *inbox.Service
	answers *clarification.Service
}

func (s nativeAnswerInbox) RecordClarificationInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, answerID uuid.UUID,
	version int64,
	people []uuid.UUID,
) error {
	for _, person := range people {
		if err := s.local.RecordResourceInTransaction(
			ctx,
			tx,
			eventID,
			person,
			inbox.Source{Kind: "clarification", ID: answerID, Version: version},
		); err != nil {
			return fmt.Errorf("record native answer inbox: %w", err)
		}
	}
	return nil
}

func (s nativeAnswerInbox) AuthorizeNotification(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	source inbox.Source,
) (inbox.Destination, error) {
	if err := s.answers.AuthorizeNotificationInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		source.ID,
		source.Version,
	); err != nil {
		return inbox.Destination{}, fmt.Errorf("authorize native answer inbox: %w", err)
	}
	return inbox.Destination{Kind: "clarification", ResourceID: source.ID}, nil
}

func TestNativeClarificationBroadSubscriptionAndPinnedVersionUseActualCurrentPublication(
	t *testing.T,
) {
	f := newFixture(t)
	_, relay := installProviders(t, f)
	_, content, challenge, team := prepareNativeCommonChallenge(t, f)
	f.now = f.now.Add(time.Minute)
	if err := content.Dispatch(t.Context(), f.id, challenge.ID, challenge.Revision); err != nil {
		t.Fatal(err)
	}
	local := f.s.Composer.Channels["in_app"].(*inbox.Service)
	answers := &clarification.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	bridge := nativeAnswerInbox{local: local, answers: answers}
	answers.Notifications = bridge
	local.Owners = map[string]inbox.ResourceOwner{"clarification": bridge}
	adapter := ClarificationSource{Answers: answers}
	f.s.Sources = map[string]SourcePolicy{"clarification.published": adapter}
	question, err := answers.Ask(
		t.Context(),
		f.person.ID,
		f.id,
		clarification.QuestionInput{
			TeamID:    team,
			CommandID: uuid.New(),
			Topic:     "challenge",
			Body:      "PRIVATE-QUESTION-DO-NOT-COPY",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := answers.SaveAnswer(
		t.Context(),
		f.owner.ID,
		f.id,
		question.ID,
		0,
		clarification.AnswerDocument{
			Title: "Common answer",
			Body:  "Public clarified requirement",
			Kind:  "clarification",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.campaign("reminder", "An approved common answer is available.", true, "in_app")
	bind := func(version int64) Binding {
		t.Helper()
		page, _ := f.change(
			Command{
				Action: "save",
				Definition: &Definition{
					ID:                     uuid.New(),
					CampaignID:             f.campaignID,
					CampaignVersion:        f.campaignVersion,
					Trigger:                "clarification.published",
					SourceID:               &answer.ID,
					SourceVersion:          version,
					MaximumLatenessSeconds: 3600,
				},
				Reason: "Review literal official-answer subscription",
			},
		)
		var saved Binding
		for _, b := range page.Bindings {
			if b.Definition.SourceVersion == version && b.ApprovedAt == nil {
				saved = b
			}
		}
		page, _ = f.change(
			Command{
				Action:    "approve",
				BindingID: saved.Definition.ID,
				Version:   saved.Version,
				Reason:    "Approve exact template, selector and native publication journey",
			},
		)
		for _, b := range page.Bindings {
			if b.Definition.ID == saved.Definition.ID {
				return b
			}
		}
		t.Fatal("binding missing")
		return Binding{}
	}
	broad, pinned := bind(0), bind(1)
	if got := f.maintain(); len(got.Occurrences) != 0 {
		t.Fatal("draft answer fabricated a publication occurrence")
	}
	publish := func(a clarification.AnswerConfiguration) clarification.AnswerConfiguration {
		t.Helper()
		for _, action := range []string{"approve", "publish"} {
			var e error
			a, e = answers.ActAnswer(
				t.Context(),
				f.owner.ID,
				f.id,
				a.ID,
				a.Revision,
				clarification.AnswerCommand{Action: action, Version: a.Version},
			)
			if e != nil {
				t.Fatal(e)
			}
		}
		return a
	}
	f.now = f.now.Add(time.Minute)
	answer = publish(answer)
	page := f.maintain()
	if len(page.Occurrences) != 2 {
		t.Fatalf("actual version1 not discovered for both reviewed bindings: %+v", page.Occurrences)
	}
	var oldPinned uuid.UUID
	for _, o := range page.Occurrences {
		if o.BindingID == broad.Definition.ID {
			f.execute(o.ID)
		} else if o.BindingID == pinned.Definition.ID {
			oldPinned = o.ID
		}
	}
	// Correct the native answer while the pinned version1 occurrence is pending.
	answer, err = answers.SaveAnswer(
		t.Context(),
		f.owner.ID,
		f.id,
		question.ID,
		answer.Revision,
		clarification.AnswerDocument{
			Title: "Corrected common answer",
			Body:  "Public clarified correction",
			Kind:  "clarification",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Minute)
	answer = publish(answer)
	page = f.maintain()
	newCount := 0
	for _, o := range page.Occurrences {
		if o.SourceRevision == 2 {
			newCount++
			if o.BindingID != broad.Definition.ID {
				t.Fatal("pinned subscription auto-advanced")
			}
			f.execute(o.ID)
		}
	}
	if newCount != 1 {
		t.Fatalf(
			"broad valid subscription did not consume reviewed correction once: %+v",
			page.Occurrences,
		)
	}
	f.execute(oldPinned)
	if err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		items, _, e := answers.DiscoverNotificationSourcesPageInTransaction(
			t.Context(),
			tx,
			f.id,
			&answer.ID,
			uuid.Nil,
			20,
		)
		if e != nil {
			return e
		}
		raw, _ := json.Marshal(items)
		if strings.Contains(string(raw), question.ID.String()) ||
			strings.Contains(string(raw), "PRIVATE") {
			return fmt.Errorf("private question entered source metadata")
		}
		if _, e = answers.ResolveNotificationAudienceInTransaction(
			t.Context(),
			tx,
			f.id,
			answer.ID,
			1,
		); e == nil {
			return fmt.Errorf("old pending version retained current dispatch authority")
		}
		return answers.AuthorizeNotificationInTransaction(
			t.Context(),
			tx,
			f.person.ID,
			f.id,
			answer.ID,
			1,
		)
	}); err != nil {
		t.Fatal(err)
	}
	// Editing the campaign clears approval; native source approval cannot replace it.
	current, err := f.s.Composer.Campaign(t.Context(), f.owner.ID, f.id, f.campaignID)
	if err != nil {
		t.Fatal(err)
	}
	definition := current.Campaign.Campaign.Definition
	definition.Name = "Explicitly edited campaign"
	if _, err = f.s.Composer.ChangeCampaign(
		t.Context(),
		f.owner.ID,
		f.id,
		f.campaignID,
		f.revision(),
		communications.CampaignCommand{
			Action:     "save",
			Version:    f.campaignVersion,
			Definition: &definition,
			Reason:     "Re-review changed campaign",
		},
	); err != nil {
		t.Fatal(err)
	}
	answer, err = answers.SaveAnswer(
		t.Context(),
		f.owner.ID,
		f.id,
		question.ID,
		answer.Revision,
		clarification.AnswerDocument{
			Title: "Third common answer",
			Body:  "Another public correction",
			Kind:  "clarification",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Minute)
	answer = publish(answer)
	page = f.maintain()
	for _, o := range page.Occurrences {
		if o.SourceRevision == 3 {
			f.execute(o.ID)
		}
	}
	var delivered int
	var body string
	if err = f.pool.QueryRow(t.Context(), `SELECT count(*),coalesce(min(body),'') FROM inbox_notifications WHERE event_id=$1 AND source_kind='campaign'`, f.id).
		Scan(&delivered, &body); err != nil {
		t.Fatal(err)
	}
	if delivered != 4 || strings.Contains(body, "PRIVATE") || relay.calls.Load() != 0 {
		t.Fatalf(
			"approved current local receipt truth changed: count%d body%q relay%d",
			delivered,
			body,
			relay.calls.Load(),
		)
	}
}
