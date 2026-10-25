package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/automation"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/deploymenttheory/hack-in-a-box/internal/presentation"
	"github.com/deploymenttheory/hack-in-a-box/internal/results"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

const resultsConsumerPrivateReason = "PRIVATE-RESULT-PREPARATION-AND-APPROVAL-PROSE"

func resultsConsumerSafe(t *testing.T, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	resultsSourceRequire(t, err)
	for _, private := range []string{resultsMentorPrivateNote, resultsConsumerPrivateReason, "PRIVATE-INDEPENDENT-JUDGE-PROSE", `"reason"`, `"votes"`, `"evidence"`, `"source_binding"`, `"tallies"`, `"confirmation"`} {
		if strings.Contains(string(raw), private) {
			t.Fatal("private result source escaped consumer projection", private, string(raw))
		}
	}
}

func resultsConsumerSources(t *testing.T, f *resultsSourceFixture) []results.NotificationSource {
	t.Helper()
	out := []results.NotificationSource{}
	err := database.Within(t.Context(), f.runtime.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		after := uuid.Nil
		for {
			page, next, err := f.service.DiscoverNotificationSourcesPageInTransaction(
				t.Context(),
				tx,
				f.runtime.eventID,
				nil,
				after,
				1,
			)
			if err != nil {
				return err
			}
			out = append(out, page...)
			if next == nil {
				return nil
			}
			if len(page) != 1 || *next != page[0].ID {
				t.Fatal("notification source cursor did not retain actual metadata")
			}
			after = *next
		}
	})
	resultsSourceRequire(t, err)
	for _, source := range out {
		raw, err := json.Marshal(source)
		resultsSourceRequire(t, err)
		var fields map[string]json.RawMessage
		resultsSourceRequire(t, json.Unmarshal(raw, &fields))
		if len(fields) != 3 || fields["ID"] == nil || fields["Version"] == nil ||
			fields["PublishedAt"] == nil {
			t.Fatal("notification source includes more than release metadata", string(raw))
		}
	}
	resultsConsumerSafe(t, out)
	return out
}

func resultsConsumerCampaign(
	t *testing.T,
	f *resultsSourceFixture,
	publicationID uuid.UUID,
) time.Time {
	t.Helper()
	ctx := t.Context()
	actor, eventID := f.runtime.owner.ID, f.runtime.eventID
	revision := func() int64 {
		t.Helper()
		row, err := dbgen.New(f.runtime.pool).Event(ctx, eventID)
		resultsSourceRequire(t, err)
		return row.Revision
	}
	composer := f.admin.deps.Communications
	template := communications.Template{
		ID:      uuid.New(),
		Name:    "Reviewed release notice",
		Purpose: "results",
		FactID:  &publicationID,
		Format:  "text",
		Subject: "An explicit result announcement",
		Body:    "The event has deliberately released results revision {{results.revision}}. Open the event results.",
	}
	audience := communications.Audience{
		ID:                 uuid.New(),
		Name:               "Complete native result announcement audience",
		Roles:              []string{"attendee", "organiser", "mentor", "judge", "mc"},
		RegistrationStates: []string{},
		Teams:              []uuid.UUID{},
	}
	for _, command := range []communications.Command{{Action: "save_template", Template: &template, Reason: "Reviewed safe literal release notice"}, {Action: "save_audience", Audience: &audience, Reason: "Review the complete native current result audience"}} {
		_, err := composer.Change(ctx, actor, eventID, revision(), command)
		resultsSourceRequire(t, err)
	}
	// Positive native rendering control before removing the owning adapter.
	visible, err := composer.Change(
		ctx,
		actor,
		eventID,
		revision(),
		communications.Command{
			Action:     "preview",
			TemplateID: template.ID,
			AudienceID: audience.ID,
			Reason:     "Review actual native result revision rendering",
		},
	)
	resultsSourceRequire(t, err)
	if visible.Preview == nil || len(visible.Preview.Recipients) == 0 {
		t.Fatal("actual registered owner did not render the published result fact", visible)
	}
	for _, recipient := range visible.Preview.Recipients {
		if len(recipient.Unresolved) != 0 || strings.Contains(recipient.Body, "{{") {
			t.Fatal("native result revision remained unresolved", recipient)
		}
	}
	// An unregistered owner must not render even an organiser's published fact.
	withoutOwner := composer
	withoutOwner.FactOwners = map[string]communications.FactOwner{}
	denied, err := withoutOwner.Change(
		ctx,
		actor,
		eventID,
		revision(),
		communications.Command{
			Action:     "preview",
			TemplateID: template.ID,
			AudienceID: audience.ID,
			Reason:     "Unknown native owner cannot render result facts",
		},
	)
	resultsSourceRequire(t, err)
	if denied.Preview == nil || len(denied.Preview.Recipients) != 0 ||
		!slices.Contains(denied.Preview.Blockers, "empty_audience") {
		t.Fatal("missing result owner fell back to a generic fact grant", denied)
	}
	page, err := f.admin.deps.Events.Read(ctx, actor, eventID)
	resultsSourceRequire(t, err)
	due := f.runtime.clock.read().Add(time.Minute)
	wall, choice := testdb.WallTimeForInstant(t, due, page.Configuration.Timezone)
	definition := communications.CampaignDefinition{
		ID:         uuid.New(),
		Name:       "Native result publication notice",
		TemplateID: template.ID,
		AudienceID: audience.ID,
		Channels:   []string{"in_app"},
		Timing:     communications.CampaignTiming{Kind: "fixed", WallTime: wall, DSTChoice: choice},
	}
	version := int64(0)
	for _, action := range []string{"save", "preview", "approve"} {
		command := communications.CampaignCommand{
			Action:  action,
			Version: version,
			Reason:  "Explicitly review the safe in-app announcement",
		}
		if action == "save" {
			command.Definition = &definition
		}
		detail, err := composer.ChangeCampaign(
			ctx,
			actor,
			eventID,
			definition.ID,
			revision(),
			command,
		)
		resultsSourceRequire(t, err)
		version = detail.Campaign.Campaign.Version
	}
	binding := automation.Definition{
		ID:                     uuid.New(),
		CampaignID:             definition.ID,
		CampaignVersion:        version,
		Trigger:                "results.published",
		SourceID:               &publicationID,
		MaximumLatenessSeconds: 3600,
	}
	auto := f.admin.deps.Automation
	saved, _, err := auto.Change(
		ctx,
		actor,
		eventID,
		revision(),
		automation.Command{
			Action:     "save",
			Definition: &binding,
			Reason:     "Bind only actual native result publication events",
		},
	)
	resultsSourceRequire(t, err)
	at := slices.IndexFunc(
		saved.Bindings,
		func(b automation.Binding) bool { return b.Definition.ID == binding.ID },
	)
	if at < 0 {
		t.Fatal("native binding not retained")
	}
	_, _, err = auto.Change(
		ctx,
		actor,
		eventID,
		revision(),
		automation.Command{
			Action:    "approve",
			BindingID: binding.ID,
			Version:   saved.Bindings[at].Version,
			Reason:    "Approve actual reviewed publication-triggered campaign",
		},
	)
	resultsSourceRequire(t, err)
	return due
}

func resultsConsumerDispatch(
	t *testing.T,
	f *resultsSourceFixture,
	expected int,
) []automation.Occurrence {
	t.Helper()
	auto := f.admin.deps.Automation
	resultsSourceRequire(t, auto.Maintain(t.Context()))
	resultsSourceRequire(t, auto.Maintain(t.Context()))
	page, err := auto.Read(t.Context(), f.runtime.owner.ID, f.runtime.eventID)
	resultsSourceRequire(t, err)
	if len(page.Occurrences) != expected {
		t.Fatal(
			"native publications omitted/duplicated automation occurrences",
			page.Occurrences,
			expected,
		)
	}
	for _, occurrence := range page.Occurrences {
		if occurrence.SourceKind != "results.published" {
			t.Fatal("unexpected source in result occurrence", occurrence)
		}
		stored, err := dbgen.New(f.runtime.pool).
			AutomationOccurrence(t.Context(), dbgen.AutomationOccurrenceParams{EventID: f.runtime.eventID, ID: occurrence.ID})
		resultsSourceRequire(t, err)
		if stored.OperationID == nil {
			t.Fatal("native occurrence has no durable operation", stored)
		}
		operation, err := dbgen.New(f.runtime.pool).Operation(t.Context(), *stored.OperationID)
		resultsSourceRequire(t, err)
		if operation.Kind != automation.HandlerKind || operation.TargetID != occurrence.ID ||
			!operation.Approved {
			t.Fatal("wrong native delivery operation", operation)
		}
		delivery := jobs.Delivery{
			ID:       operation.ID,
			EventID:  operation.EventID,
			TargetID: operation.TargetID,
			Key:      operation.ID.String(),
		}
		eligible, err := auto.Eligible(t.Context(), delivery)
		resultsSourceRequire(t, err)
		if !eligible {
			t.Fatal("native approved result occurrence ineligible", occurrence)
		}
		// The fixture's domain time is2027. Invoke the actual registered handler;
		// this proves approved pipeline effects, not a future wall-clock worker lease.
		for range 2 {
			if outcome := auto.Deliver(t.Context(), delivery); outcome != jobs.Delivered {
				t.Fatal("actual result dispatch did not persist/reuse local delivery", outcome)
			}
		}
	}
	return page.Occurrences
}

func TestResultsPublicationConsumersKeepPrivatePreparationAndIndependentReleaseBoundaries(
	t *testing.T,
) {
	overall, categoryPrize := uuid.New(), uuid.New()
	var category uuid.UUID
	f := newResultsSourceFixtureWithSetup(
		t,
		rubric.MentorQualification,
		func(d *rubric.Definition) {
			category = d.Categories[0].ID
			d.Prizes = []rubric.Prize{
				{ID: overall, Name: "Overall result"},
				{ID: categoryPrize, Name: "Demo result", CategoryID: &category},
			}
		},
		nil,
	)
	ctx := t.Context()
	actor, eventID := f.runtime.owner.ID, f.runtime.eventID
	actualSource, ok := f.admin.deps.Automation.Sources["results.published"].(automation.ResultSource)
	if !ok || actualSource.Results == nil {
		t.Fatal("production communication graph lacks the actual results source")
	}
	// Factory presentation wiring is owned by the parallel API integration. Here
	// the actual native result owner is explicitly composed into the real service.
	room := f.admin.deps.Presentation
	room.Results = &f.service
	display, err := room.Read(ctx, actor, eventID)
	resultsSourceRequire(t, err)
	paired, err := room.Pair(
		ctx,
		actor,
		eventID,
		display.Revision,
		presentation.PairInput{
			Label:   "Synthetic result proof display",
			Minutes: 60,
			Reason:  "Explicit result-room pairing",
		},
	)
	resultsSourceRequire(t, err)
	for _, token := range []string{"", strings.Repeat("wrong", 8)} {
		if _, err = room.Snapshot(ctx, eventID, token); !errors.Is(err, access.ErrUnauthenticated) {
			t.Fatal("unpaired room read result preparation", err)
		}
	}
	snapshot, err := room.Snapshot(ctx, eventID, paired.Token)
	resultsSourceRequire(t, err)
	if len(snapshot.Results) != 0 || len(resultsConsumerSources(t, f)) != 0 {
		t.Fatal("unreleased result became a consumer source", snapshot.Results)
	}
	resultsConsumerOfficialFacts(t, f, 0)
	p, err := f.service.Read(ctx, actor, eventID)
	resultsSourceRequire(t, err)
	p, err = f.service.Change(
		ctx,
		actor,
		eventID,
		p.Revision,
		results.Command{
			RequestID:     uuid.New(),
			Version:       p.Round.Version,
			Action:        "prepare",
			SourceBinding: p.Preview.FinalistsBinding,
			Reason:        resultsConsumerPrivateReason,
		},
	)
	resultsSourceRequire(t, err)
	if len(resultsConsumerSources(t, f)) != 0 {
		t.Fatal("private prepared finalist selection became a notification source")
	}
	resultsConsumerOfficialFacts(t, f, 0)
	snapshot, err = room.Snapshot(ctx, eventID, paired.Token)
	resultsSourceRequire(t, err)
	if len(snapshot.Results) != 0 {
		t.Fatal("paired display received private preparation", snapshot.Results)
	}
	resultsConsumerDispatch(t, f, 0)
	firstCommand := results.Command{
		RequestID:     uuid.New(),
		Version:       p.Round.Version,
		Action:        "publish_finalists",
		SourceBinding: p.Preview.FinalistsBinding,
		Reason:        resultsConsumerPrivateReason,
	}
	firstRevision := p.Revision
	p, err = f.service.Change(ctx, actor, eventID, firstRevision, firstCommand)
	resultsSourceRequire(t, err)
	firstReceipt := *p.Receipt
	repeated, err := f.service.Change(ctx, actor, eventID, firstRevision, firstCommand)
	resultsSourceRequire(t, err)
	if !reflect.DeepEqual(repeated.Receipt, &firstReceipt) || repeated.Revision != p.Revision ||
		repeated.Round.Version != p.Round.Version {
		t.Fatal("exact finalist retry changed original acknowledgement")
	}
	resultsConsumerOfficialFacts(t, f, 1)
	first := resultsConsumerSources(t, f)
	if len(first) != 1 || firstReceipt.PublicationID == nil ||
		first[0].ID != *firstReceipt.PublicationID ||
		first[0].Version != p.Round.Version {
		t.Fatal(
			"deliberate finalist release did not yield one exact metadata source",
			first,
			p.Receipt,
		)
	}
	snapshot, err = room.Snapshot(ctx, eventID, paired.Token)
	resultsSourceRequire(t, err)
	if len(snapshot.Results) != 1 || snapshot.Results[0].Kind != "finalists" ||
		len(snapshot.Results[0].Awards) != 0 {
		t.Fatal("finalist room release bypassed award embargo", snapshot.Results)
	}
	resultsConsumerSafe(t, snapshot.Results)
	due := resultsConsumerCampaign(t, f, first[0].ID)
	f.runtime.clock.set(due)
	firstOccurrences := resultsConsumerDispatch(t, f, 1)
	if firstOccurrences[0].SourceID != first[0].ID ||
		firstOccurrences[0].SourceRevision != first[0].Version {
		t.Fatal("automation replaced the actual publication identity", firstOccurrences)
	}
	initialInbox, err := f.admin.deps.Inbox.List(ctx, f.runtime.competitor.ID, &eventID, nil, 20)
	resultsSourceRequire(t, err)
	if len(initialInbox.Items) != 1 {
		t.Fatal("native repeated dispatch omitted/duplicated local release notice", initialInbox)
	}

	f.complete(t)
	p, err = f.service.Read(ctx, actor, eventID)
	resultsSourceRequire(t, err)
	p, err = f.service.Change(
		ctx,
		actor,
		eventID,
		p.Revision,
		results.Command{
			RequestID:     uuid.New(),
			Version:       p.Round.Version,
			Action:        "prepare",
			SourceBinding: p.Preview.Binding,
			Reason:        resultsConsumerPrivateReason,
		},
	)
	resultsSourceRequire(t, err)
	awards := []results.Award{
		{
			PrizeID:  overall,
			Name:     "Overall result",
			TeamID:   f.runtime.teamID,
			Amount:   "100.001",
			Currency: "GBP",
		},
		{
			PrizeID:    categoryPrize,
			Name:       "Demo result",
			CategoryID: &category,
			TeamID:     f.runtime.teamID,
		},
	}
	p, err = f.service.Change(
		ctx,
		actor,
		eventID,
		p.Revision,
		results.Command{
			RequestID:     uuid.New(),
			Version:       p.Round.Version,
			Action:        "confirm",
			SourceBinding: p.Preview.Binding,
			Awards:        awards,
			Reason:        resultsConsumerPrivateReason,
		},
	)
	resultsSourceRequire(t, err)
	if len(resultsConsumerSources(t, f)) != 1 {
		t.Fatal("private award confirmation emitted a source")
	}
	resultsConsumerOfficialFacts(t, f, 1)
	snapshot, err = room.Snapshot(ctx, eventID, paired.Token)
	resultsSourceRequire(t, err)
	if len(snapshot.Results) != 1 || len(snapshot.Results[0].Awards) != 0 {
		t.Fatal("confirmed private award reached the room", snapshot.Results)
	}
	finalCommand := results.Command{
		RequestID:     uuid.New(),
		Version:       p.Round.Version,
		Action:        "publish_awards",
		SourceBinding: p.Preview.Binding,
		Reason:        resultsConsumerPrivateReason,
	}
	finalRevision := p.Revision
	p, err = f.service.Change(ctx, actor, eventID, finalRevision, finalCommand)
	resultsSourceRequire(t, err)
	awardReceipt := *p.Receipt
	repeated, err = f.service.Change(ctx, actor, eventID, finalRevision, finalCommand)
	resultsSourceRequire(t, err)
	if !reflect.DeepEqual(repeated.Receipt, &awardReceipt) || repeated.Revision != p.Revision ||
		repeated.Round.Version != p.Round.Version {
		t.Fatal("exact award retry repeated/replaced release")
	}
	sources := resultsConsumerSources(t, f)
	if len(sources) != 2 || sources[0].ID == sources[1].ID || awardReceipt.PublicationID == nil ||
		!slices.ContainsFunc(sources, func(s results.NotificationSource) bool {
			return s.ID == *awardReceipt.PublicationID && s.Version == p.Round.Version
		}) {
		t.Fatal("independent release kinds did not retain exactly two safe sources", sources)
	}
	resultsConsumerOfficialFacts(t, f, 2)
	due = resultsConsumerCampaign(t, f, *awardReceipt.PublicationID)
	f.runtime.clock.set(due)
	resultsConsumerDispatch(t, f, 2)
	snapshot, err = room.Snapshot(ctx, eventID, paired.Token)
	resultsSourceRequire(t, err)
	if len(snapshot.Results) != 2 {
		t.Fatal("room did not retain independent explicit releases", snapshot.Results)
	}
	awardAt := slices.IndexFunc(
		snapshot.Results,
		func(a results.Announcement) bool { return a.Kind == "awards" },
	)
	if awardAt < 0 || len(snapshot.Results[awardAt].Awards) != 2 ||
		snapshot.Results[awardAt].Awards[0].Amount != "100.001" {
		t.Fatal("room lost exact announced prize metadata", snapshot.Results)
	}
	resultsConsumerSafe(t, snapshot.Results)
	if strings.Contains(mustResultsConsumerJSON(t, snapshot), paired.Token) {
		t.Fatal("room snapshot exposed pairing capability")
	}
	currentInbox, err := f.admin.deps.Inbox.List(ctx, f.runtime.competitor.ID, &eventID, nil, 20)
	resultsSourceRequire(t, err)
	if len(currentInbox.Items) != 2 {
		t.Fatal("second deliberate publication omitted/duplicated local messages", currentInbox)
	}
	for _, item := range currentInbox.Items {
		detail, err := f.admin.deps.Inbox.Open(ctx, f.runtime.competitor.ID, item.ID, nil)
		resultsSourceRequire(t, err)
		resultsConsumerSafe(t, detail)
		if detail.Body != fmt.Sprintf(
			"The event has deliberately released results revision %d. Open the event results.",
			first[0].Version,
		) &&
			detail.Body != fmt.Sprintf(
				"The event has deliberately released results revision %d. Open the event results.",
				awardReceipt.Version,
			) {
			t.Fatal("reviewed literal publication message changed", detail)
		}
	}
	factsBeforeSupersession := resultsConsumerOfficialFacts(t, f, 2)
	resultsConsumerFactAuthority(t, f, factsBeforeSupersession)
	// An explicitly reviewed new native judge source publishes another award of
	// the same kind. Older routing facts remain stored, but cease granting access.
	f.reopen(t, 0)
	f.card(t, 0, 7, "submit")
	f.round(t, judging.RoundCommand{Action: "open_deliberation"})
	p, err = f.service.Read(ctx, actor, eventID)
	resultsSourceRequire(t, err)
	p, err = f.service.Change(
		ctx,
		actor,
		eventID,
		p.Revision,
		results.Command{
			RequestID:     uuid.New(),
			Version:       p.Round.Version,
			Action:        "prepare",
			SourceBinding: p.Preview.Binding,
			Reason:        resultsConsumerPrivateReason,
		},
	)
	resultsSourceRequire(t, err)
	p, err = f.service.Change(
		ctx,
		actor,
		eventID,
		p.Revision,
		results.Command{
			RequestID:     uuid.New(),
			Version:       p.Round.Version,
			Action:        "confirm",
			SourceBinding: p.Preview.Binding,
			Awards:        awards,
			Reason:        resultsConsumerPrivateReason,
		},
	)
	resultsSourceRequire(t, err)
	p, err = f.service.Change(
		ctx,
		actor,
		eventID,
		p.Revision,
		results.Command{
			RequestID:     uuid.New(),
			Version:       p.Round.Version,
			Action:        "publish_awards",
			SourceBinding: p.Preview.Binding,
			Reason:        resultsConsumerPrivateReason,
		},
	)
	resultsSourceRequire(t, err)
	if p.Receipt.PublicationID == nil || *p.Receipt.PublicationID == *awardReceipt.PublicationID {
		t.Fatal("native correction reused an earlier publication")
	}
	allFacts := resultsConsumerOfficialFacts(t, f, 3)
	err = database.Within(ctx, f.runtime.pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		for _, fact := range allFacts {
			allowed, e := f.service.AuthorizeCommunicationFact(
				ctx,
				q,
				f.runtime.competitor.ID,
				fact,
			)
			if e != nil {
				return e
			}
			if allowed != (fact.ID != *awardReceipt.PublicationID) {
				t.Fatal("fact did not enforce latest publication per kind", fact, allowed)
			}
		}
		return nil
	})
	resultsSourceRequire(t, err)
	sources = resultsConsumerSources(t, f)
	if len(sources) != 2 ||
		slices.ContainsFunc(
			sources,
			func(s results.NotificationSource) bool { return s.ID == *awardReceipt.PublicationID },
		) {
		t.Fatal("superseded award remained a current notification source", sources)
	}
	stillReadable, err := f.admin.deps.Inbox.List(ctx, f.runtime.competitor.ID, &eventID, nil, 20)
	resultsSourceRequire(t, err)
	if len(stillReadable.Items) != 1 {
		t.Fatal(
			"stale results-purpose campaign remained readable or unrelated finalist notice disappeared",
			stillReadable,
		)
	}
	currentNotificationID := stillReadable.Items[0].ID
	var smtp, teams int
	resultsSourceRequire(
		t,
		f.runtime.pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM smtp_campaign_messages WHERE event_id=$1),(SELECT count(*) FROM teams_workflow_deliveries WHERE event_id=$1)", eventID).
			Scan(&smtp, &teams),
	)
	if smtp != 0 || teams != 0 {
		t.Fatal("unselected provider channels received effects", smtp, teams)
	}

	_, err = f.runtime.pool.Exec(
		ctx,
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		eventID,
		f.runtime.competitor.ID,
	)
	resultsSourceRequire(t, err)
	err = database.Within(ctx, f.runtime.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		for _, source := range sources {
			audience, err := actualSource.ResolveInTransaction(
				ctx,
				tx,
				eventID,
				automation.Source{
					Kind:     "results.published",
					ID:       source.ID,
					Revision: source.Version,
				},
			)
			if err != nil {
				return err
			}
			if slices.Contains(audience.RecipientIDs, f.runtime.competitor.ID) {
				t.Fatal("native result audience retained withdrawn competitor")
			}
		}
		return nil
	})
	resultsSourceRequire(t, err)
	facts := resultsConsumerOfficialFacts(t, f, 3)
	err = database.Within(ctx, f.runtime.pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		for _, fact := range facts {
			allowed, e := f.service.AuthorizeCommunicationFact(
				ctx,
				q,
				f.runtime.competitor.ID,
				fact,
			)
			if e != nil {
				return e
			}
			if allowed {
				t.Fatal("withdrawn recipient retained native result fact authority")
			}
		}
		return nil
	})
	resultsSourceRequire(t, err)
	hidden, err := f.admin.deps.Inbox.List(ctx, f.runtime.competitor.ID, &eventID, nil, 20)
	resultsSourceRequire(t, err)
	if len(hidden.Items) != 0 {
		t.Fatal("withdrawn recipient retained private campaign inbox", hidden)
	}
	if _, err = f.admin.deps.Inbox.Open(
		ctx,
		f.runtime.competitor.ID,
		currentNotificationID,
		nil,
	); err == nil {
		t.Fatal("withdrawn recipient opened retained notification")
	}
	display, err = room.Read(ctx, actor, eventID)
	resultsSourceRequire(t, err)
	_, err = room.Revoke(
		ctx,
		actor,
		eventID,
		paired.Pairing.ID,
		display.Revision,
		"Explicitly revoke result-room capability",
	)
	resultsSourceRequire(t, err)
	if _, err = room.Snapshot(
		ctx,
		eventID,
		paired.Token,
	); !errors.Is(
		err,
		access.ErrUnauthenticated,
	) {
		t.Fatal("revoked display pairing retained results", err)
	}
}

func mustResultsConsumerJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	resultsSourceRequire(t, err)
	return string(raw)
}

func resultsConsumerOfficialFacts(
	t *testing.T,
	f *resultsSourceFixture,
	count int,
) []dbgen.CommunicationFact {
	t.Helper()
	facts, err := dbgen.New(f.runtime.pool).CommunicationFacts(t.Context(), f.runtime.eventID)
	resultsSourceRequire(t, err)
	facts = slices.DeleteFunc(
		facts,
		func(f dbgen.CommunicationFact) bool { return f.Kind != "results" },
	)
	if len(facts) != count {
		t.Fatal(
			"deliberate native result publication did not atomically retain exactly its official facts",
			len(facts),
			count,
		)
	}
	for _, fact := range facts {
		if fact.Status != "published" || fact.Revision < 1 || fact.OwnerID != nil ||
			fact.ResourceID != nil {
			t.Fatal("official result fact includes non-routing source/grant fields", fact)
		}
	}
	return facts
}

func resultsConsumerFactAuthority(
	t *testing.T,
	f *resultsSourceFixture,
	facts []dbgen.CommunicationFact,
) {
	t.Helper()
	outsider := testdb.Person(t, f.runtime.pool)
	err := database.Within(t.Context(), f.runtime.pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		for _, fact := range facts {
			for _, person := range []uuid.UUID{f.runtime.owner.ID, f.runtime.competitor.ID, f.runtime.mentor.ID, f.runtime.mc.ID, f.judges[0].ID} {
				allowed, e := f.service.AuthorizeCommunicationFact(t.Context(), q, person, fact)
				if e != nil {
					return e
				}
				if !allowed {
					t.Fatal(
						"actual published fact denied its native current audience",
						person,
						fact,
					)
				}
			}
			allowed, e := f.service.AuthorizeCommunicationFact(t.Context(), q, outsider.ID, fact)
			if e != nil {
				return e
			}
			if allowed {
				t.Fatal("published fact granted an unknown identity access")
			}
			for _, kind := range []string{"unknown", "stale version", "wrong event", "wrong status", "wrong kind", "owner grant", "resource grant"} {
				changed := fact
				switch kind {
				case "unknown":
					changed.ID = uuid.New()
				case "stale version":
					changed.Revision++
				case "wrong event":
					changed.EventID = f.runtime.otherEventID
				case "wrong status":
					changed.Status = "open"
				case "wrong kind":
					changed.Kind = "help"
				case "owner grant":
					changed.OwnerID = &f.runtime.owner.ID
				case "resource grant":
					changed.ResourceID = &fact.ID
				}
				allowed, e = f.service.AuthorizeCommunicationFact(
					t.Context(),
					q,
					f.runtime.owner.ID,
					changed,
				)
				if e != nil {
					return e
				}
				if allowed {
					t.Fatal("unowned/stale fact became an organiser grant", kind)
				}
			}
			earlierReader := f.service
			earlierReader.Now = func() time.Time { return f.runtime.clock.read().Add(-time.Hour) }
			allowed, e = earlierReader.AuthorizeCommunicationFact(
				t.Context(),
				q,
				f.runtime.owner.ID,
				fact,
			)
			if e != nil {
				return e
			}
			if allowed {
				t.Fatal("a future publication was readable at the earlier server instant")
			}
		}
		return nil
	})
	resultsSourceRequire(t, err)
}

func TestResultPublicationAndOfficialFactRollbackTogetherOnNativeFactFailure(t *testing.T) {
	prize := uuid.New()
	f := newResultsSourceFixtureWithSetup(
		t,
		rubric.AllTeams,
		func(d *rubric.Definition) { d.Prizes = []rubric.Prize{{ID: prize, Name: "Overall result"}} },
		nil,
	)
	f.complete(t)
	ctx := t.Context()
	actor, eventID := f.runtime.owner.ID, f.runtime.eventID
	p, err := f.service.Read(ctx, actor, eventID)
	resultsSourceRequire(t, err)
	p, err = f.service.Change(
		ctx,
		actor,
		eventID,
		p.Revision,
		results.Command{
			RequestID:     uuid.New(),
			Version:       p.Round.Version,
			Action:        "prepare",
			SourceBinding: p.Preview.Binding,
			Reason:        resultsConsumerPrivateReason,
		},
	)
	resultsSourceRequire(t, err)
	p, err = f.service.Change(
		ctx,
		actor,
		eventID,
		p.Revision,
		results.Command{
			RequestID:     uuid.New(),
			Version:       p.Round.Version,
			Action:        "confirm",
			SourceBinding: p.Preview.Binding,
			Reason:        resultsConsumerPrivateReason,
			Awards: []results.Award{
				{PrizeID: prize, Name: "Overall result", TeamID: f.runtime.teamID},
			},
		},
	)
	resultsSourceRequire(t, err)
	resultsConsumerOfficialFacts(t, f, 0)
	// A test-owned database fault, never a fabricated publication or official fact.
	_, err = f.runtime.pool.Exec(
		ctx,
		`CREATE FUNCTION fail_test_result_fact() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.kind='results' THEN RAISE EXCEPTION 'synthetic official result fact failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_test_result_fact BEFORE INSERT OR UPDATE ON communication_facts FOR EACH ROW EXECUTE FUNCTION fail_test_result_fact()`,
	)
	resultsSourceRequire(t, err)
	command := results.Command{
		RequestID:     uuid.New(),
		Version:       p.Round.Version,
		Action:        "publish_awards",
		SourceBinding: p.Preview.Binding,
		Reason:        resultsConsumerPrivateReason,
	}
	if _, err = f.service.Change(ctx, actor, eventID, p.Revision, command); err == nil {
		t.Fatal("publication succeeded despite failed official fact transaction")
	}
	current, err := f.service.Read(ctx, actor, eventID)
	resultsSourceRequire(t, err)
	if current.Revision != p.Revision || current.Round.Version != p.Round.Version ||
		len(current.Announcements) != 0 {
		t.Fatal("failed fact left a partial publication/version", current)
	}
	resultsConsumerOfficialFacts(t, f, 0)
	ack, err := f.service.OriginalReceipt(ctx, actor, eventID, command.RequestID)
	resultsSourceRequire(t, err)
	if ack.Receipt != nil {
		t.Fatal("rolled-back publication created a synthetic acknowledgement")
	}
	_, err = f.runtime.pool.Exec(
		ctx,
		`DROP TRIGGER fail_test_result_fact ON communication_facts; DROP FUNCTION fail_test_result_fact()`,
	)
	resultsSourceRequire(t, err)
	command.RequestID = uuid.New()
	published, err := f.service.Change(ctx, actor, eventID, current.Revision, command)
	resultsSourceRequire(t, err)
	facts := resultsConsumerOfficialFacts(t, f, 1)
	if published.Receipt.PublicationID == nil || facts[0].ID != *published.Receipt.PublicationID ||
		facts[0].Revision != published.Round.Version {
		t.Fatal("real successful publication did not own its exact official fact", published, facts)
	}
}
