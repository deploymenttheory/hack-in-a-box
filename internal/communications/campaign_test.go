package communications

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type syntheticChannel struct{}

func (syntheticChannel) Readiness(context.Context, uuid.UUID, uuid.UUID) (ChannelReadiness, error) {
	return ChannelReadiness{Available: true, SupportsSelfTest: true, SelfTestAvailable: true}, nil
}

func campaignDefinition(f *fixture) CampaignDefinition {
	return CampaignDefinition{
		ID:         uuid.New(),
		Name:       "Synthetic scheduled reminder",
		TemplateID: f.t.ID,
		AudienceID: f.a.ID,
		Channels:   []string{"smtp"},
		Timing:     CampaignTiming{Kind: "relative", Anchor: "live_start", Days: -1},
	}
}

func campaignChange(t *testing.T, f *fixture, c Campaign, input CampaignCommand) CampaignDetail {
	t.Helper()
	input.Version = c.Version
	input.Reason = "Synthetic campaign review"
	p, err := f.s.ChangeCampaign(
		t.Context(),
		f.owner.ID,
		f.eventID,
		c.Definition.ID,
		f.revision(t),
		input,
	)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func newCampaign(t *testing.T, f *fixture) CampaignDetail {
	t.Helper()
	d := campaignDefinition(f)
	return campaignChange(
		t,
		f,
		Campaign{Definition: d},
		CampaignCommand{Action: "save", Definition: &d},
	)
}

func readyCampaign(t *testing.T, f *fixture) CampaignDetail {
	t.Helper()
	f.person(t)
	f.s.Channels = map[string]ChannelProvider{
		"smtp":   syntheticChannel{},
		"in_app": syntheticChannel{},
		"teams":  syntheticChannel{},
	}
	p := newCampaign(t, f)
	p = campaignChange(t, f, p.Campaign.Campaign, CampaignCommand{Action: "preview"})
	return campaignChange(t, f, p.Campaign.Campaign, CampaignCommand{Action: "approve"})
}

func TestCampaignApprovalBindsContentSelectorChannelsCalendarAndTiming(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	p := readyCampaign(t, f)
	if p.Campaign.Status != "approved" || p.Campaign.Campaign.Approval == nil {
		t.Fatal("reviewed campaign not approved")
	}
	original := p.Campaign.Campaign.Approval.Binding
	d := p.Campaign.Campaign.Definition
	d.Channels = []string{"smtp", "in_app"}
	d.IncludeCalendar = true
	d.Timing.Minutes = 30
	p = campaignChange(t, f, p.Campaign.Campaign, CampaignCommand{Action: "save", Definition: &d})
	if p.Campaign.Status != "draft" || p.Campaign.Campaign.Approval != nil ||
		p.Campaign.Campaign.PreviewID != nil {
		t.Fatal("channel/timing/calendar edits retained approval")
	}
	p = campaignChange(t, f, p.Campaign.Campaign, CampaignCommand{Action: "preview"})
	p = campaignChange(t, f, p.Campaign.Campaign, CampaignCommand{Action: "approve"})
	if p.Campaign.Campaign.Approval.Binding == original {
		t.Fatal("reviewed binding ignored changed channels/calendar/timing")
	}
	f.t.Body = "Changed {{person.name}} content"
	f.change(t, Command{Action: "save_template", Template: &f.t})
	f.t.Version++
	current, err := f.s.Campaign(ctx, f.owner.ID, f.eventID, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Campaign.Status != "draft" ||
		!slices.Contains(current.Campaign.Blockers, "approval_outdated") {
		t.Fatal("new template version retained effective campaign approval")
	}
	var operations int
	if err = f.pool.QueryRow(ctx, "SELECT count(*) FROM operations WHERE event_id=$1", f.eventID).
		Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if operations != 0 {
		t.Fatal("campaign approval prematurely scheduled unattended sends")
	}
	// Selector changes and event-local date changes invalidate the same binding.
	f.a.Name = "New selector version"
	f.change(t, Command{Action: "save_audience", Audience: &f.a})
	f.a.Version++
	p = campaignChange(t, f, current.Campaign.Campaign, CampaignCommand{Action: "preview"})
	p = campaignChange(t, f, p.Campaign.Campaign, CampaignCommand{Action: "approve"})
	f.configuration.LiveStart = f.configuration.LiveStart.AddDate(0, 0, 1)
	raw, _ := json.Marshal(f.configuration)
	f.exec(t, "UPDATE events SET configuration=$2 WHERE id=$1", f.eventID, raw)
	current, err = f.s.Campaign(ctx, f.owner.ID, f.eventID, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Campaign.Status != "draft" {
		t.Fatal("revised dates retained approval")
	}
}

func TestCampaignUnavailableChannelsStaySelectedAndTeamsIsNeverASelfTest(t *testing.T) {
	f := newFixture(t)
	f.person(t)
	p := newCampaign(t, f)
	d := p.Campaign.Campaign.Definition
	d.Channels = []string{"smtp", "teams"}
	p = campaignChange(t, f, p.Campaign.Campaign, CampaignCommand{Action: "save", Definition: &d})
	p = campaignChange(t, f, p.Campaign.Campaign, CampaignCommand{Action: "preview"})
	if !slices.Equal(p.Campaign.Campaign.Definition.Channels, d.Channels) ||
		!slices.Contains(p.Campaign.Blockers, "provider_unavailable_smtp") ||
		!slices.Contains(p.Campaign.Blockers, "provider_unavailable_teams") ||
		!slices.Contains(p.Campaign.Blockers, "teams_requires_broadcast_content") {
		t.Fatal("unavailable/unsafe selected channel was hidden or dropped")
	}
	_, err := f.s.ChangeCampaign(
		t.Context(),
		f.owner.ID,
		f.eventID,
		d.ID,
		f.revision(t),
		CampaignCommand{
			Action:  "approve",
			Version: p.Campaign.Campaign.Version,
			Reason:  "Must remain blocked",
		},
	)
	if !errors.Is(err, event.ErrBlocked) {
		t.Fatalf("unavailable campaign approved: %v", err)
	}
	f.s.Channels = map[string]ChannelProvider{"teams": syntheticChannel{}}
	for _, r := range f.s.channelReadiness(t.Context(), f.eventID, f.owner.ID) {
		if r.Channel == "teams" &&
			(r.SupportsSelfTest || r.SelfTestAvailable || r.SelfTestReason != "broadcast_only") {
			t.Fatal("provider advertised a shared Teams test as self-only")
		}
	}
	_, err = f.s.ChangeCampaign(
		t.Context(),
		f.owner.ID,
		f.eventID,
		d.ID,
		f.revision(t),
		CampaignCommand{
			Action:  "test_self",
			Version: p.Campaign.Campaign.Version,
			Channel: "teams",
			Reason:  "Do not broadcast a test",
		},
	)
	if !errors.Is(err, event.ErrBlocked) {
		t.Fatalf("Teams self-test accepted: %v", err)
	}
}

func TestCampaignSelfTestUsesOnlyCurrentRequestingOrganiserAndCancelsObsoleteIntent(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	competitor := f.person(t)
	f.exec(t, "UPDATE people SET display_name='Requesting organiser' WHERE id=$1", f.owner.ID)
	f.exec(t, "UPDATE people SET display_name='Different competitor' WHERE id=$1", competitor.ID)
	outbox, err := jobs.New(
		f.pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.s.Jobs = outbox
	f.s.Channels = map[string]ChannelProvider{"smtp": syntheticChannel{}}
	p := newCampaign(t, f)
	p = campaignChange(
		t,
		f,
		p.Campaign.Campaign,
		CampaignCommand{Action: "test_self", Channel: "smtp"},
	)
	if p.TestRequestID == nil {
		t.Fatal("test intent not retained")
	}
	testID := *p.TestRequestID
	var message TestMessage
	err = database.Within(ctx, f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		var e error
		message, e = f.s.ResolveTestSelfInTransaction(ctx, tx, f.eventID, testID)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	if message.ActorID != f.owner.ID || !strings.Contains(message.Subject, "[TEST]") ||
		!strings.Contains(message.PlainText, "Requesting organiser") ||
		strings.Contains(message.PlainText, "Different competitor") {
		t.Fatal("test copied another recipient or was not labelled")
	}
	var op dbgen.Operation
	if err = f.pool.QueryRow(ctx, "SELECT id,event_id,kind,target_id,approved,cancelled,state FROM operations WHERE event_id=$1 AND target_id=$2", f.eventID, testID).
		Scan(&op.ID, &op.EventID, &op.Kind, &op.TargetID, &op.Approved, &op.Cancelled, &op.State); err != nil {
		t.Fatal(err)
	}
	if op.Kind != "communication_test_smtp" || op.TargetID != testID || !op.Approved ||
		op.State != "pending" {
		t.Fatal("test did not use a typed pending outbox intent")
	}
	other := testdb.Event(t, f.pool, f.owner.ID)
	err = database.Within(ctx, f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		_, e := f.s.ResolveTestSelfInTransaction(ctx, tx, other, testID)
		return e
	})
	if !errors.Is(err, access.ErrNotFound) {
		t.Fatalf("cross-event test resolved: %v", err)
	}
	if _, err = f.pool.Exec(
		ctx,
		"UPDATE communication_test_requests SET channel='in_app' WHERE event_id=$1 AND id=$2",
		f.eventID,
		testID,
	); err == nil {
		t.Fatal("test evidence mutable")
	}
	d := p.Campaign.Campaign.Definition
	d.Name = "Explicit changed campaign"
	p = campaignChange(t, f, p.Campaign.Campaign, CampaignCommand{Action: "save", Definition: &d})
	var cancelled bool
	if err = f.pool.QueryRow(ctx, "SELECT cancelled FROM operations WHERE id=$1", op.ID).
		Scan(&cancelled); err != nil {
		t.Fatal(err)
	}
	if !cancelled {
		t.Fatal("edit retained old queued test")
	}
	err = database.Within(ctx, f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		_, e := f.s.ResolveTestSelfInTransaction(ctx, tx, f.eventID, testID)
		return e
	})
	if !errors.Is(err, ErrUnresolved) {
		t.Fatalf("obsolete test resolved: %v", err)
	}
	p = campaignChange(
		t,
		f,
		p.Campaign.Campaign,
		CampaignCommand{Action: "test_self", Channel: "smtp"},
	)
	f.exec(
		t,
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='organiser'",
		f.eventID,
		f.owner.ID,
	)
	err = database.Within(ctx, f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		_, e := f.s.ResolveTestSelfInTransaction(ctx, tx, f.eventID, *p.TestRequestID)
		return e
	})
	if !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("revoked organiser test resolved: %v", err)
	}
}

func TestCampaignCompetingApprovalAndCancellationHaveOneReviewedWinner(t *testing.T) {
	f := newFixture(t)
	f.person(t)
	f.s.Channels = map[string]ChannelProvider{"smtp": syntheticChannel{}}
	p := newCampaign(t, f)
	p = campaignChange(t, f, p.Campaign.Campaign, CampaignCommand{Action: "preview"})
	rev := f.revision(t)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, action := range []string{"approve", "cancel"} {
		go func() {
			<-start
			_, err := f.s.ChangeCampaign(
				t.Context(),
				f.owner.ID,
				f.eventID,
				p.Campaign.Campaign.Definition.ID,
				rev,
				CampaignCommand{
					Action:  action,
					Version: p.Campaign.Campaign.Version,
					Reason:  "Concurrent reviewed control",
				},
			)
			results <- err
		}()
	}
	close(start)
	success, conflict := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			success++
		} else if errors.Is(err, access.ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("approval/cancel winners: %d/%d", success, conflict)
	}
}

func TestCampaignTimingUsesExplicitDSTAndBusinessCalendar(t *testing.T) {
	c := event.DefaultConfiguration()
	c.Timezone = "Europe/London"
	_, err := campaignDue(c, CampaignTiming{Kind: "fixed", WallTime: "2027-03-28T01:30:00"})
	if !errors.Is(err, schedule.ErrLocalTime) {
		t.Fatalf("DST gap accepted: %v", err)
	}
	ambiguous := CampaignTiming{Kind: "fixed", WallTime: "2027-10-31T01:30:00"}
	_, err = campaignDue(c, ambiguous)
	if !errors.Is(err, schedule.ErrAmbiguous) {
		t.Fatalf("DST fold silently chosen: %v", err)
	}
	ambiguous.DSTChoice = "earlier"
	early, err := campaignDue(c, ambiguous)
	if err != nil {
		t.Fatal(err)
	}
	ambiguous.DSTChoice = "later"
	late, err := campaignDue(c, ambiguous)
	if err != nil {
		t.Fatal(err)
	}
	if late.Sub(early) != time.Hour {
		t.Fatal("DST choice not preserved")
	}
	c.LiveStart = time.Date(2027, 6, 1, 9, 0, 0, 0, time.UTC)
	c.Calendar.ExcludedDates = []string{"2027-05-31"}
	due, err := campaignDue(
		c,
		CampaignTiming{Kind: "relative", Anchor: "live_start", Days: -1, BusinessDays: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if due.Format(time.RFC3339) != "2027-05-28T09:00:00Z" {
		t.Fatalf("working calendar ignored: %s", due)
	}
}
