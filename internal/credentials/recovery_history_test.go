package credentials

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/deploymenttheory/hack-in-a-box/internal/inbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
	"github.com/deploymenttheory/hack-in-a-box/internal/recovery"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type recoveryChannel struct{}

func (recoveryChannel) Readiness(
	context.Context,
	uuid.UUID,
	uuid.UUID,
) (communications.ChannelReadiness, error) {
	return communications.ChannelReadiness{
		Available:      true,
		ConfigRevision: "synthetic-recovery-configuration",
	}, nil
}

// Delegate to the same native answer/inbox transaction contracts as app wiring.
type recoveryAnswerInbox struct {
	answers *clarification.Service
	local   *inbox.Service
}

func (s recoveryAnswerInbox) RecordClarificationInTransaction(
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
			return fmt.Errorf("record native restored answer reference: %w", err)
		}
	}
	return nil
}

func (s recoveryAnswerInbox) AuthorizeNotification(
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
		return inbox.Destination{}, fmt.Errorf("authorize native recovered answer: %w", err)
	}
	return inbox.Destination{Kind: "clarification", ResourceID: source.ID}, nil
}

// All meaningful history is created through its native owner. The assertions
// compare the exact immutable rows and read restored objects with current native
// permissions; merely restoring empty tables is not recovery evidence.
func seedRecoveryHistories(t *testing.T, f *fixture) func(*pgxpool.Pool) {
	t.Helper()
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Second)
	cfg := event.DefaultConfiguration()
	cfg.Timezone = "UTC"
	cfg.MinimumAttendance = 2
	cfg.LiveStart = now.Add(-time.Hour)
	cfg.LiveEnd = now.Add(4 * time.Hour)
	cfg.SubmissionCutoff = cfg.LiveEnd.Add(-time.Hour)
	due := cfg.LiveStart.Add(-7 * 24 * time.Hour)
	cfg.TeamFreeze = due.Add(-time.Hour)
	cfg.AdmissionCutoff = cfg.TeamFreeze.Add(-time.Hour)
	encoded, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(
		"UPDATE events SET configuration=$2,stage='preparation',team_frozen_at=$3,published_at=NULL WHERE id=$1",
		f.event,
		encoded,
		cfg.TeamFreeze,
	)
	f.exec(
		"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
		f.event,
		f.other.ID,
	)
	f.exec(
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
		f.event,
		f.team,
		f.other.ID,
	)
	f.exec(
		"INSERT INTO event_gate_facts(event_id,attendance_ready,teams_ready,total_teams,frozen_teams,assigned_attendees) VALUES($1,true,true,1,1,2)",
		f.event,
	)
	ref := testdb.ApprovedRubric(t, f.pool, f.owner.ID, f.event)
	clock := due.Add(-time.Minute)
	contents := content.Service{Pool: f.pool, Clock: func() time.Time { return clock }}
	challenge, err := contents.Save(
		ctx,
		f.owner.ID,
		f.event,
		uuid.New(),
		0,
		content.Input{
			Kind: "challenge",
			Document: content.Document{
				Title:    "Synthetic released rules",
				Markdown: "Build the published challenge. Cite your sources. Exact approved rubric applies.",
				Audience: access.ContentAudience{Confirmed: true},
				Rubric:   &ref,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"review", "approve"} {
		challenge, err = contents.Action(
			ctx,
			f.owner.ID,
			f.event,
			challenge.ID,
			challenge.Revision,
			action,
			"Synthetic exact recovery rules approval",
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	clock = due
	if err = contents.Dispatch(ctx, f.event, challenge.ID, challenge.Revision); err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE events SET stage='live' WHERE id=$1", f.event)

	clarifications := clarification.Service{Pool: f.pool, Clock: func() time.Time { return now }}
	local := &inbox.Service{Pool: f.pool, Now: func() time.Time { return now }}
	answerInbox := recoveryAnswerInbox{answers: &clarifications, local: local}
	local.Owners = map[string]inbox.ResourceOwner{"clarification": answerInbox}
	clarifications.Notifications = answerInbox
	question, err := clarifications.Ask(
		ctx,
		f.member.ID,
		f.event,
		clarification.QuestionInput{
			TeamID:    f.team,
			CommandID: uuid.New(),
			Topic:     "challenge",
			Body:      "SYNTHETIC-PRIVATE-RECOVERY-QUESTION May we use the documented public dataset?",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := clarifications.SaveAnswer(
		ctx,
		f.owner.ID,
		f.event,
		question.ID,
		0,
		clarification.AnswerDocument{
			Title: "Public dataset clarification",
			Body:  "The published rules permit that public dataset with attribution.",
			Kind:  "clarification",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"approve", "publish"} {
		answer, err = clarifications.ActAnswer(
			ctx,
			f.owner.ID,
			f.event,
			answer.ID,
			answer.Revision,
			clarification.AnswerCommand{Action: action, Version: answer.Version},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	support := help.Service{Pool: f.pool, Clock: func() time.Time { return now }}
	request, err := support.Create(
		ctx,
		f.member.ID,
		f.event,
		uuid.New(),
		help.Input{
			TeamID:      f.team,
			Category:    "setup",
			Description: "SYNTHETIC-PRIVATE-RECOVERY-HELP",
			Location:    "Synthetic table",
			CommandID:   uuid.New(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	request, err = support.Change(
		ctx,
		f.owner.ID,
		f.event,
		request.Request.ID,
		request.Request.Revision,
		help.Command{Action: "claim", CommandID: uuid.New()},
	)
	if err != nil {
		t.Fatal(err)
	}
	request, err = support.Change(
		ctx,
		f.owner.ID,
		f.event,
		request.Request.ID,
		request.Request.Revision,
		help.Command{Action: "resolve", CommandID: uuid.New()},
	)
	if err != nil {
		t.Fatal(err)
	}

	reviewEvent := testdb.Event(t, f.pool, f.owner.ID)
	end := now.Add(-time.Hour)
	f.exec(
		`UPDATE events SET configuration=jsonb_build_object('live_start',$2::text,'live_end',$3::text,'timezone','UTC'),stage='completed',completed_at=$3::timestamptz,published_at=$4::timestamptz WHERE id=$1`,
		reviewEvent,
		end.Add(-time.Hour).Format(time.RFC3339),
		end.Format(time.RFC3339),
		end.Add(-8*24*time.Hour),
	)
	composer := &communications.Service{
		Pool:     f.pool,
		Channels: map[string]communications.ChannelProvider{"in_app": recoveryChannel{}},
	}
	followthrough := outcomes.Service{
		Pool:     f.pool,
		Composer: composer,
		Now:      func() time.Time { return now },
	}
	outcomeID := uuid.New()
	item, err := followthrough.Change(
		ctx,
		f.member.ID,
		outcomeID,
		0,
		outcomes.Command{
			ID:     uuid.New(),
			Action: "propose",
			Reason: "Synthetic recovery idea",
			Definition: &outcomes.Definition{
				Kind:          "pitch",
				Problem:       "Synthetic observed problem",
				Audience:      "Synthetic service team",
				ProposedValue: "Proposed benefit, not demonstrated adoption",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []outcomes.Command{
		{ID: uuid.New(), Action: "adopt", Reason: "Synthetic event provenance", Provenance: &outcomes.Provenance{EventID: &reviewEvent}},
		{ID: uuid.New(), Action: "assign", Reason: "Synthetic current owner", OwnerID: &f.member.ID},
		{ID: uuid.New(), Action: "moderate", Reason: "Synthetic accepted follow-through", Status: "accepted"},
		{ID: uuid.New(), Action: "evidence", Reason: "Synthetic dated evidence", Evidence: &outcomes.Evidence{ID: uuid.New(), Basis: "demonstrated_adoption", Claim: "One synthetic team used the prototype", Source: "https://example.test/synthetic-recovery-evidence", ObservedAt: &now}},
	} {
		item, err = followthrough.Change(ctx, f.owner.ID, outcomeID, item.Version.Version, command)
		if err != nil {
			t.Fatal(err)
		}
	}
	reviewID := uuid.New()
	schedule := outcomes.ReviewSchedule{
		Days:     30,
		WallTime: end.AddDate(0, 0, 30).Format("2006-01-02T15:04:05"),
		Timezone: "UTC",
		Channels: []string{"in_app"},
	}
	message := outcomes.ReviewMessage{
		Subject: "Synthetic follow-through review",
		Body:    "Hello {{recipient.name}}. Review {{outcome.url}} at {{review.date}}.",
	}
	plan, err := followthrough.ChangePlan(
		ctx,
		f.owner.ID,
		outcomeID,
		reviewID,
		0,
		outcomes.PlanCommand{
			ID:            uuid.New(),
			Action:        "save",
			SourceVersion: item.Version.Version,
			Schedule:      &schedule,
			Message:       &message,
			Reason:        "Synthetic approved future reminder",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	plan, err = followthrough.ChangePlan(
		ctx,
		f.owner.ID,
		outcomeID,
		reviewID,
		plan.Version,
		outcomes.PlanCommand{
			ID:            uuid.New(),
			Action:        "approve",
			SourceVersion: item.Version.Version,
			Reason:        "Synthetic exact future reminder approval",
		},
	)
	if err != nil || plan.EffectiveState != "approved" {
		t.Fatalf("native approved follow-up absent: %v", err)
	}
	// Include realistic account-volume data without inventing extra attendance,
	// competition entries or completed work for those unassociated people.
	f.exec(
		`INSERT INTO people(issuer,subject,display_name) SELECT 'synthetic-recovery',gen_random_uuid()::text,'Synthetic retained account' FROM generate_series(1,997)`,
	)
	f.exec(
		`INSERT INTO operations(event_id,operation_key,kind,target_id,due_at,approved,state,attempts) VALUES($1,'synthetic-original-pending','synthetic_recovery',$1,now(),true,'pending',0),($1,'synthetic-original-unknown','synthetic_recovery',$1,now(),true,'ambiguous',1)`,
		reviewEvent,
	)
	f.exec(
		`INSERT INTO delivery_attempts(operation_id,lease_token,outcome,finished_at) SELECT id,gen_random_uuid(),'ambiguous',now() FROM operations WHERE operation_key='synthetic-original-unknown'`,
	)
	tables := []string{
		"operations", "delivery_attempts",
		"rubric_versions",
		"rubric_approvals",
		"rubric_publications",
		"content_versions",
		"content_changes",
		"help_requests",
		"help_transitions",
		"clarification_questions",
		"clarification_versions",
		"clarification_approvals",
		"clarification_publications",
		"clarification_changes",
		"outcome_versions",
		"outcome_evidence",
		"outcome_review_versions",
	}
	snapshots := map[string]string{}
	for _, table := range tables {
		snapshots[table] = historySnapshot(t, f.pool, table)
	}
	return func(pool *pgxpool.Pool) {
		t.Helper()
		for _, table := range tables {
			if actual := historySnapshot(t, pool, table); actual != snapshots[table] {
				t.Fatalf("native immutable history differs after recovery: %s", table)
			}
		}
		native := clarification.Service{Pool: pool, Clock: func() time.Time { return now }}
		original, e := native.Question(ctx, f.member.ID, f.event, question.ID)
		if e != nil || original.Body != question.Body {
			t.Fatal("restored private question unavailable to its current team")
		}
		if _, e = native.Question(ctx, testdb.Person(t, pool).ID, f.event, question.ID); e == nil {
			t.Fatal("restore granted stranger private question access")
		}
		recoveredSupport := help.Service{Pool: pool, Clock: func() time.Time { return now }}
		page, e := recoveredSupport.Read(ctx, f.member.ID, f.event, request.Request.ID)
		if e != nil || page.Request.Status != "resolved" || len(page.History) != 3 {
			t.Fatalf("native help history unreadable after restore: %v", e)
		}
		recoveredOutcomes := outcomes.Service{
			Pool: pool,
			Composer: &communications.Service{
				Pool:     pool,
				Channels: map[string]communications.ChannelProvider{"in_app": recoveryChannel{}},
			},
			Now: func() time.Time { return now },
		}
		evidence, e := recoveredOutcomes.Evidence(ctx, f.member.ID, outcomeID, uuid.Nil, 25)
		if e != nil || len(evidence.Items) != 1 {
			t.Fatalf("native outcome evidence unavailable after restore: %v", e)
		}
		plans, e := recoveredOutcomes.Plans(ctx, f.member.ID, outcomeID)
		if e != nil || len(plans.Items) != 1 || plans.Items[0].EffectiveState != "approved" {
			t.Fatalf("exact approved follow-up not restored: %v", e)
		}
		status, e := recovery.Read(ctx, pool)
		if e != nil || status.HoldID == nil {
			t.Fatal("history restore not held")
		}
		found := false
		for _, intent := range status.Intents {
			if intent.Kind == "outcome_review" && intent.ID == reviewID {
				found = true
			}
		}
		if !found {
			t.Fatal("approved follow-up missing from deliberate recovery review")
		}
		encoded, e := json.Marshal(status)
		if e != nil || strings.Contains(string(encoded), "SYNTHETIC-PRIVATE") {
			t.Fatal("private history entered safe operator review")
		}
	}
}

func historySnapshot(t *testing.T, pool *pgxpool.Pool, table string) string {
	t.Helper()
	// Caller list is fixed in this synthetic fixture, never request/CLI input.
	query := "SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text),'[]'::jsonb)::text FROM " + table + " r"
	var snapshot string
	if err := pool.QueryRow(t.Context(), query).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot == "[]" {
		t.Fatalf("empty history cannot prove recovery: %s", table)
	}
	return snapshot
}
