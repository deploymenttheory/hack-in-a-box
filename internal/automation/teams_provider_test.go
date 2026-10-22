package automation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/teamsworkflow"
)

func TestProductionTeamsOccurrenceQueuesReferencesAndRechecksCurrentPublicScope(t *testing.T) {
	f := newFixture(t)
	installProviders(t, f)
	team := installTeamsProvider(t, f)
	f.campaign("reminder", "Event: {{event.name}}", true, "teams")
	f.bind()
	id := f.maintain().Occurrences[0].ID
	f.now = f.now.Add(time.Minute)
	f.execute(id)
	page, err := f.s.Read(t.Context(), f.owner.ID, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if page.Occurrences[0].ChannelReports["teams"].Queued != 1 {
		t.Fatal("current public source did not queue one broadcast intent")
	}
	var d jobs.Delivery
	var occurrence uuid.UUID
	if err = f.pool.QueryRow(t.Context(), "SELECT operation_id,event_id,id,occurrence_id FROM teams_workflow_deliveries").
		Scan(&d.ID, &d.EventID, &d.TargetID, &occurrence); err != nil {
		t.Fatal(err)
	}
	if occurrence != id {
		t.Fatal("Teams intent omitted immutable occurrence identity")
	}
	// Eligibility decrypts/configures and rechecks the current literal source but
	// never resolves DNS or sends. This test has no approved external channel.
	eligible, err := team.Eligible(t.Context(), d)
	if err != nil || !eligible {
		t.Fatal("current queued public scope was not eligible", err)
	}
	if err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		report, queueErr := team.QueueApprovedOccurrenceReportInTransaction(
			t.Context(),
			tx,
			f.id,
			f.campaignID,
			f.campaignVersion,
			communications.DeliveryScope{OccurrenceID: id},
		)
		if queueErr != nil {
			return queueErr
		}
		if report.Queued != 0 || report.Preexisting["pending"] != 1 {
			t.Fatalf("retry invented a new queued broadcast: %+v", report)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.s.Change(
		t.Context(),
		f.owner.ID,
		f.id,
		f.revision(),
		Command{
			Action:       "review_occurrence",
			OccurrenceID: id,
			Reason:       "Cannot strand an unsettled broadcast",
		},
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("review changed scope while existing broadcast remained pending", err)
	}
	f.exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		f.id,
		f.person.ID,
	)
	if eligible, err = team.Eligible(t.Context(), d); err != nil || eligible {
		t.Fatal("withdrawn common audience retained send authority", err)
	}
	var attempts, receipts int
	if err = f.pool.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM teams_workflow_attempts),(SELECT count(*) FROM teams_workflow_receipts)").
		Scan(&attempts, &receipts); err != nil {
		t.Fatal(err)
	}
	if attempts != 0 || receipts != 0 {
		t.Fatal("queue/eligibility fabricated external delivery evidence")
	}
}

func installTeamsProvider(t *testing.T, f *fixture) *teamsworkflow.Service {
	t.Helper()
	keys, err := secretbox.Parse(
		`{"active":"synthetic","keys":{"synthetic":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	team := &teamsworkflow.Service{
		Pool:        f.pool,
		Keys:        keys,
		Jobs:        f.s.Jobs,
		ScopePolicy: f.s,
		Now:         func() time.Time { return f.now },
	}
	f.s.Composer.Channels["teams"] = team
	team.Composer = *f.s.Composer
	endpoint := "https://syntheticenv.01.environment.api.powerplatform.com/powerautomate/automations/direct/workflows/00000000000040008000000000000001/triggers/manual/paths/invoke?api-version=1&sp=%2Ftriggers%2Fmanual%2Frun&sv=1.0&sig=SYNTHETIC_NOT_A_REAL_CHANNEL"
	// Seed the encrypted configuration only: setup DNS/transport are tested by
	// the owning adapter. This orchestration test must never touch a live channel.
	connectionID := uuid.New()
	keyID, ciphertext, err := keys.Seal(
		[]byte(endpoint),
		[]byte("hackbox:teams-workflow:"+f.id.String()+":"+connectionID.String()+":1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err = dbgen.New(f.pool).
		SaveTeamsWorkflowConnection(t.Context(), dbgen.SaveTeamsWorkflowConnectionParams{ID: connectionID, EventID: f.id, Version: 1, Label: "Synthetic queue-only channel", OwnerLabel: "Synthetic owner", CoOwnerLabel: "Synthetic co-owner", KeyID: keyID, Ciphertext: ciphertext, Active: true, UpdatedAt: f.now}); err != nil {
		t.Fatal(err)
	}

	f.s.Providers["teams"] = func(ctx context.Context, tx pgx.Tx, eventID, campaignID uuid.UUID, version int64, scope communications.DeliveryScope) (ChannelReport, error) {
		report, queueErr := team.QueueApprovedOccurrenceReportInTransaction(
			ctx,
			tx,
			eventID,
			campaignID,
			version,
			scope,
		)
		return ChannelReport{Queued: report.Queued, Preexisting: report.Preexisting}, queueErr
	}
	f.s.ReviewGuards["teams"] = team.CheckOccurrenceReviewInTransaction
	return team
}

func TestProductionTeamsExpectedChallengeWaitsForGuardedCommonPublication(t *testing.T) {
	f := newFixture(t)
	installProviders(t, f)
	team := installTeamsProvider(t, f)
	cfg := event.DefaultConfiguration()
	cfg.Timezone = "Europe/London"
	cfg.MinimumAttendance = 1
	zone, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		t.Fatal(err)
	}
	cfg.LiveStart = f.now.Add(time.Minute).In(zone).AddDate(0, 0, 7)
	cfg.LiveEnd = cfg.LiveStart.Add(8 * time.Hour)
	cfg.SubmissionCutoff = cfg.LiveEnd.Add(-time.Hour)
	cfg.TeamFreeze = f.now.Add(-time.Hour)
	cfg.AdmissionCutoff = cfg.TeamFreeze.Add(-time.Hour)
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(
		"UPDATE events SET stage='preparation',configuration=$2,team_frozen_at=$3 WHERE id=$1",
		f.id,
		raw,
		cfg.TeamFreeze,
	)
	f.exec(
		"INSERT INTO event_gate_facts(event_id,attendance_ready,teams_ready,total_teams,frozen_teams,assigned_attendees) VALUES($1,true,true,1,1,1)",
		f.id,
	)
	teamID := uuid.New()
	f.exec(
		"INSERT INTO hack_teams(event_id,id,name,state,frozen) VALUES($1,$2,'Synthetic frozen team','published',true)",
		f.id,
		teamID,
	)
	f.exec(
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
		f.id,
		teamID,
		f.person.ID,
	)
	rubrics := rubric.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	definition := rubric.Default(rubric.AllTeams, f.id)
	saved, err := rubrics.Change(
		t.Context(),
		f.owner.ID,
		f.id,
		0,
		rubric.Command{
			Action:     "save",
			Definition: &definition,
			Reason:     "Configure real common criteria",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := rubrics.Change(
		t.Context(),
		f.owner.ID,
		f.id,
		saved.Revision,
		rubric.Command{
			Action:  "approve",
			Version: saved.Reference.Version,
			Reason:  "Approve exact common criteria",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	sourceService := content.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	source, err := sourceService.Save(
		t.Context(),
		f.owner.ID,
		f.id,
		uuid.New(),
		0,
		content.Input{
			Kind: "challenge",
			Document: content.Document{
				Title:    "PRIVATE-SOURCE-TITLE-CANARY",
				Markdown: "Synthetic approved common challenge",
				Rubric:   &approved.Reference,
				Audience: access.ContentAudience{
					Confirmed: true,
					Roles:     []string{},
					Teams:     []uuid.UUID{},
				},
				Attachments: []uuid.UUID{},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"review", "approve"} {
		source, err = sourceService.Action(
			t.Context(),
			f.owner.ID,
			f.id,
			source.ID,
			source.Revision,
			action,
			"Review exact challenge source",
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	f.campaign("challenge_release", "Published challenge: {{challenge.title}}", false, "teams")
	binding := f.publicationBinding(source)
	f.now = f.now.Add(time.Minute)
	if len(f.maintain().Occurrences) != 0 {
		t.Fatal("planned release time alone queued a common challenge announcement")
	}
	if err = sourceService.Dispatch(t.Context(), f.id, source.ID, source.Revision); err != nil {
		t.Fatal(err)
	}
	page := f.maintain()
	if len(page.Occurrences) != 1 {
		t.Fatal("actual guarded challenge publication did not create a durable occurrence")
	}
	if err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		_, resolveErr := f.s.Composer.ResolveApprovedScopedCampaignInTransaction(
			t.Context(),
			tx,
			f.id,
			f.campaignID,
			f.campaignVersion,
			communications.DeliveryScope{OccurrenceID: page.Occurrences[0].ID},
			f.s,
		)
		return resolveErr
	}); err != nil {
		t.Fatalf("canonical challenge scope: %v", err)
	}
	f.execute(page.Occurrences[0].ID)
	var d jobs.Delivery
	var kind, proofID string
	if err = f.pool.QueryRow(t.Context(), "SELECT operation_id,event_id,id,scope_approval->>'kind',scope_approval->>'id' FROM teams_workflow_deliveries").
		Scan(&d.ID, &d.EventID, &d.TargetID, &kind, &proofID); err != nil {
		t.Fatal(err)
	}
	if kind != "expected_publication" || binding.PublicationApprovalID == nil ||
		proofID != binding.PublicationApprovalID.String() {
		t.Fatal("Teams intent lost exact typed publication provenance")
	}
	eligible, err := team.Eligible(t.Context(), d)
	if err != nil || !eligible {
		t.Fatal("same-version actual challenge could not be revalidated by Teams worker", err)
	}
	detail, err := f.s.Composer.Campaign(t.Context(), f.owner.ID, f.id, f.campaignID)
	if err != nil || detail.Campaign.Status != "draft" {
		t.Fatal("Teams scoped dispatch forged generic campaign approval", err)
	}
	var attempts int
	if err = f.pool.QueryRow(t.Context(), "SELECT count(*) FROM teams_workflow_attempts").
		Scan(&attempts); err != nil ||
		attempts != 0 {
		t.Fatal("queue-only proof attempted a real external channel", err)
	}
}
