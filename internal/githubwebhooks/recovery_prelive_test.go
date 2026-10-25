package githubwebhooks

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
	"github.com/google/uuid"
)

// Webhooks use actual receipt wall time. Keep this pre-live event separate from
// the historical durable-cutoff fixture; no future job is made artificially due.
func TestRecoveryActualPreliveReceiptAllowsWorkButNeverFormalSubmissionOrReview(t *testing.T) {
	f := setup(t)
	member := testdb.Person(t, f.pool)
	config := event.DefaultConfiguration()
	config.LiveStart = time.Now().UTC().Truncate(time.Second).Add(24 * time.Hour)
	config.LiveEnd = config.LiveStart.Add(8 * time.Hour)
	config.SubmissionCutoff = config.LiveStart.Add(7 * time.Hour)
	publication := config.LiveStart.AddDate(0, 0, -7)
	config.TeamFreeze = publication.Add(-time.Hour)
	config.AdmissionCutoff = config.TeamFreeze.Add(-time.Hour)
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE events SET stage='preparation',configuration=$2,published_at=$3,team_frozen_at=$4 WHERE id=$1", f.event, encoded, publication, config.TeamFreeze)
	f.exec("UPDATE hack_teams SET frozen=true,published_captain_id=$3 WHERE event_id=$1 AND id=$2", f.event, f.team, member.ID)
	f.exec("INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')", f.event, member.ID)
	f.exec("INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot)VALUES($1,$2,$3,'published')", f.event, f.team, member.ID)
	definition := uuid.New()
	defs := templates.EmptyDefinitions()
	defs.Deliverables = []templates.Deliverable{{ID: definition, Title: "Synthetic early repository", Required: true, EvidenceKinds: []string{"repository"}}}
	encoded, err = json.Marshal(defs)
	if err != nil {
		t.Fatal(err)
	}
	if err = dbgen.New(f.pool).SaveEventDefinitions(t.Context(), dbgen.SaveEventDefinitionsParams{EventID: f.event, Definitions: encoded}); err != nil {
		t.Fatal(err)
	}
	native := submissions.Service{Pool: f.pool} // Real wall clock, no injected domain time.
	evidence := []submissions.Evidence{{DeliverableID: definition, Kind: "repository", Label: "Synthetic work before live", URL: "https://github.com/synthetic-org/synthetic-repo"}}
	draft, err := native.SaveDraft(t.Context(), member.ID, f.event, f.team, 0, evidence)
	if err != nil || draft.Revision != 1 {
		t.Fatal("actual pre-live draft blocked", draft, err)
	}
	if _, err = native.Submit(t.Context(), member.ID, f.event, f.team, 0, submissions.Command{Evidence: evidence}); !errors.Is(err, event.ErrBlocked) {
		t.Fatal("actual pre-live formal submission accepted", err)
	}
	if _, err = native.Review(t.Context(), f.owner, f.event, f.team, 0, 1, "accepted", "Synthetic early review"); !errors.Is(err, event.ErrBlocked) {
		t.Fatal("actual pre-live review accepted", err)
	}
	raw := f.push(time.Now().UTC().Add(-time.Minute))
	id := uuid.New()
	if err = f.s.Receive(t.Context(), Headers{DeliveryID: id.String(), Event: "push", Signature: sign(raw, testSecret)}, raw); err != nil {
		t.Fatal(err)
	}
	delivery := f.delivery(id)
	if !delivery.ReceivedAt.Time.Before(config.LiveStart) || delivery.ReceivedAt.Time.Before(publication) || !delivery.ProviderAt.Valid || !delivery.ProviderAt.Time.Before(config.LiveStart) {
		t.Fatal("fixture was not a genuine pre-live webhook receipt", delivery.ReceivedAt.Time)
	}
	f.execute(id)
	page, err := native.Read(t.Context(), member.ID, f.event, f.team)
	if err != nil || page.CanSubmit || page.Final != nil || page.Outcome != "open" || page.Revision != 0 {
		t.Fatal("operational pre-live push accepted formal evidence", page, err)
	}
	retained, err := native.Draft(t.Context(), member.ID, f.event, f.team)
	if err != nil || retained.Revision != draft.Revision || len(retained.Evidence) != 1 {
		t.Fatal("push rewrote private work", retained, err)
	}
	var revisions, cards int
	if err = f.pool.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM submission_revisions WHERE event_id=$1),(SELECT count(*) FROM judge_cards WHERE event_id=$1)", f.event).Scan(&revisions, &cards); err != nil || revisions != 0 || cards != 0 {
		t.Fatal("activity invented formal/scoring records", revisions, cards, err)
	}
	status, err := f.s.Status(t.Context(), f.owner, f.event)
	if err != nil || len(status.Activity) != 1 || status.Activity[0].Pushes != 1 {
		t.Fatal("legitimate pre-live operational work missing", status, err)
	}
}
