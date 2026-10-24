package submissions_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/archives"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

// Cancellation before a hack followed by archival is a real lifecycle path:
// there need not ever have been a formal submission outcome to preserve.
func TestNeverHeldArchiveDoesNotInventSubmissionOutcomes(t *testing.T) {
	pool := testdb.New(t, true)
	f := newFixture(t, pool)
	f.now = f.c.LiveStart.Add(-time.Hour)
	events := event.Service{
		Pool:              pool,
		Clock:             func() time.Time { return f.now },
		SettleSubmissions: f.s.Settle,
	}
	row, err := f.q.Event(t.Context(), f.id)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := events.Transition(
		t.Context(),
		f.owner.ID,
		f.id,
		row.Revision,
		"cancelled",
		"Synthetic never-held cancellation",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = events.Transition(
		t.Context(),
		f.owner.ID,
		f.id,
		cancelled.Revision,
		"archived",
		"",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var result string
		err := pool.QueryRow(t.Context(), `SELECT json_build_object(
   'states',(SELECT json_agg(s ORDER BY s.team_id) FROM submission_states s WHERE event_id=$1),
   'changes',(SELECT json_agg(c ORDER BY c.team_id,c.revision) FROM submission_changes c WHERE event_id=$1),
   'facts',(SELECT row_to_json(f) FROM event_gate_facts f WHERE event_id=$1),
   'event',(SELECT row_to_json(e) FROM events e WHERE id=$1))::text`, f.id).Scan(&result)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	before := snapshot()
	f.now = f.c.SubmissionCutoff.Add(time.Hour)
	if err = f.s.Maintain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(); after != before {
		t.Fatalf("maintenance mutated archived competition: before=%s after=%s", before, after)
	}
	if _, err = f.s.Read(
		t.Context(),
		f.owner.ID,
		f.id,
		f.team,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatalf("missing archived outcome must remain unavailable: %v", err)
	}
	if _, err = f.s.History(
		t.Context(),
		f.owner.ID,
		f.id,
		f.team,
		0,
		10,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatalf("history invented archived outcome: %v", err)
	}
	if after := snapshot(); after != before {
		t.Fatalf("archive reads mutated competition: before=%s after=%s", before, after)
	}
}

func TestArchiveDeliverableExportKeepsFrozenRevisionAndSelectedAudience(t *testing.T) {
	pool := testdb.New(t, true)
	f := newFixture(t, pool)
	f.defs.Deliverables[2].EvidenceKinds = []string{"url"}
	f.saveDefs()
	f.grant(f.mentor.ID, "mentor", f.team)
	items := append(
		f.evidence("formal-repository"),
		submissions.Evidence{
			DeliverableID: f.secret,
			Kind:          "url",
			Label:         "ORGANISER-EVIDENCE-CANARY",
			URL:           "https://example.test/organiser-only",
		},
	)
	submitted := f.submit(f.member.ID, 0, items, "")
	f.now = f.c.SubmissionCutoff
	if err := f.s.Maintain(t.Context()); err != nil {
		t.Fatal(err)
	}
	row, err := f.q.Event(t.Context(), f.id)
	if err != nil {
		t.Fatal(err)
	}
	lifecycle := event.Service{
		Pool:              pool,
		Clock:             func() time.Time { return f.now },
		SettleSubmissions: f.s.Settle,
	}
	cancelled, err := lifecycle.Transition(
		t.Context(),
		f.owner.ID,
		f.id,
		row.Revision,
		"cancelled",
		"Synthetic archive after native settlement",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	archived, err := lifecycle.Transition(
		t.Context(),
		f.owner.ID,
		f.id,
		cancelled.Revision,
		"archived",
		"",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	service := archives.Service{Pool: pool, Clock: func() time.Time { return f.now }}
	request := archives.ExportRequest{
		Source:     "deliverables",
		ResourceID: f.team,
		Audience:   "mentor",
		Limit:      10,
	}
	result, err := service.Export(t.Context(), f.mentor.ID, f.id, request)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	if result.Submission == nil || result.Submission.Outcome != "frozen" ||
		result.Submission.Final == nil ||
		result.Submission.Final.Revision != submitted.Final.Revision ||
		strings.Contains(string(raw), "ORGANISER-EVIDENCE-CANARY") {
		t.Fatal("frozen audience projection changed", string(raw))
	}
	request.Audience = "organiser"
	result, err = service.Export(t.Context(), f.owner.ID, f.id, request)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(result)
	if !strings.Contains(string(raw), "ORGANISER-EVIDENCE-CANARY") {
		t.Fatal("actual organiser lost owned projection")
	}
	if _, err = service.Export(
		t.Context(),
		f.mentor.ID,
		f.id,
		request,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("mentor promoted export audience", err)
	}
	request.Audience = "participant"
	request.ResourceID = f.otherTeam
	if _, err = service.Export(
		t.Context(),
		f.member.ID,
		f.id,
		request,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("team export crossed resource", err)
	}
	current, err := f.q.Event(t.Context(), f.id)
	if err != nil || current.Revision != archived.Revision || current.WritesAllowed {
		t.Fatal("export mutated archived competition", current, err)
	}
}
