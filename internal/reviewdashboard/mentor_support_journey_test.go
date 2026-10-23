package reviewdashboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/observations"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

// This composed scenario uses the real owning services, their actual published
// rubric and one shared clock. SQL only bootstraps synthetic people/coverage and
// the separate team owner's already-disbanded state; it never fabricates scores,
// huddle authority, frozen submissions or judging grants.
func TestMultiTeamQuietEvidenceLivePendingAndFreshFrozenFinal(t *testing.T) {
	f := setup(t)
	board := dashboard(f)
	output := submissions.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	notes := observations.Service{Pool: f.pool, Now: func() time.Time { return f.now }}
	repo := uuid.New()
	definitions := templates.EmptyDefinitions()
	definitions.Deliverables = []templates.Deliverable{
		{
			ID:            repo,
			Title:         "Formal repository",
			Required:      true,
			EvidenceKinds: []string{"repository"},
		},
	}
	raw, _ := json.Marshal(definitions)
	f.exec(
		`INSERT INTO event_definitions(event_id,definitions)VALUES($1,$2)ON CONFLICT(event_id)DO UPDATE SET definitions=EXCLUDED.definitions`,
		f.event,
		raw,
	)
	otherTeam, otherMember := f.journeyTeam("Working draft only")
	discardedTeam, discardedMember := f.journeyTeam("Unused disbanded repository")
	evidence := func(label string) []submissions.Evidence {
		return []submissions.Evidence{
			{
				DeliverableID: repo,
				Kind:          "repository",
				Label:         label,
				URL:           "https://example.test/synthetic/" + label,
			},
		}
	}
	noteCommand := observations.Command{
		Action: "observation",
		ID:     uuid.New(),
		Evidence: observations.Evidence{
			Category: "collaboration",
			Evidence: "QUIET-CONTRIBUTOR-FACT: paired design explanation and demonstrated error recovery by " + f.other.ID.String() + "; no repository activity measure",
			Outcome:  "Factual demonstrated contribution, independent of who typed the code",
		},
	}
	f.now = f.config.LiveStart.Add(-time.Nanosecond)
	if _, err := notes.Change(
		t.Context(),
		f.primary.ID,
		f.event,
		f.team,
		f.revision(),
		noteCommand,
	); !errors.Is(
		err,
		event.ErrBlocked,
	) {
		t.Fatal("pre-live private competition observation accepted", err)
	}
	if _, err := f.s.Change(
		t.Context(),
		f.primary.ID,
		f.event,
		f.team,
		f.revision(),
		f.command(f.checkpoint, "save", false),
	); err == nil {
		t.Fatal("pre-live provisional score accepted")
	}
	before, err := output.Read(t.Context(), f.member.ID, f.event, f.team)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = output.Submit(
		t.Context(),
		f.member.ID,
		f.event,
		f.team,
		before.Revision,
		submissions.Command{
			Evidence: evidence("prelive"),
			Reason:   "Must remain private early work",
		},
	); err == nil {
		t.Fatal("pre-live formal output accepted")
	}
	f.now = f.config.LiveStart
	for _, pair := range []struct {
		team, member uuid.UUID
		label        string
	}{{f.team, f.member.ID, "quiet-demonstration"}, {otherTeam, otherMember.ID, "unused-working-only"}, {discardedTeam, discardedMember.ID, "unused-disbanded"}} {
		if _, err = output.SaveDraft(
			t.Context(),
			pair.member,
			f.event,
			pair.team,
			0,
			evidence(pair.label),
		); err != nil {
			t.Fatal("permitted private live draft rejected", err)
		}
	}
	observed, err := notes.Change(
		t.Context(),
		f.primary.ID,
		f.event,
		f.team,
		f.revision(),
		noteCommand,
	)
	if err != nil || len(observed.Observations) != 1 {
		t.Fatal("quiet factual contribution not retained by native owner", observed, err)
	}
	note := observed.Observations[0]
	score := f.command(f.checkpoint, "save", false)
	observedValue := f.checkpoint.Criteria.Categories[0].MinScore
	score.Scores = []scorecards.CategoryScore{
		{
			CategoryID: f.checkpoint.Criteria.Categories[0].ID,
			Value:      &observedValue,
			Evidence: []scorecards.EvidenceReference{
				{Kind: "observation", ID: &note.ID, Version: &note.Version},
			},
		},
	}
	live, err := f.s.Change(t.Context(), f.primary.ID, f.event, f.team, f.revision(), score)
	if err != nil || live.Scores[0].Pending || live.Scores[0].Value == nil ||
		*live.Scores[0].Value != observedValue ||
		!live.Scores[1].Pending ||
		live.Scores[1].Value != nil {
		t.Fatal("explicit observed rubric value conflated with unobserved categories", live, err)
	}
	score.Action = "submit"
	score.Version = live.Reference.Version
	live, err = f.s.Change(t.Context(), f.primary.ID, f.event, f.team, f.revision(), score)
	if err != nil {
		t.Fatal(err)
	}
	// A second team has its own independent provisional card; it is not a ranking.
	if _, err = f.s.Change(
		t.Context(),
		f.primary.ID,
		f.event,
		otherTeam,
		f.revision(),
		f.command(f.checkpoint, "submit", false),
	); err != nil {
		t.Fatal(err)
	}
	final := f.configure("mentor_final")
	if _, err = f.s.Change(
		t.Context(),
		f.primary.ID,
		f.event,
		f.team,
		f.revision(),
		f.command(final, "save", false),
	); err == nil {
		t.Fatal("final assessment accepted before cutoff/frozen output")
	}
	current, err := output.Read(t.Context(), f.member.ID, f.event, f.team)
	if err != nil {
		t.Fatal(err)
	}
	formal, err := output.Submit(
		t.Context(),
		f.member.ID,
		f.event,
		f.team,
		current.Revision,
		submissions.Command{
			Evidence: evidence("quiet-demonstration"),
			Reason:   "Explicit team formal output; working drafts stay separate",
		},
	)
	if err != nil || formal.Final == nil {
		t.Fatal("actual formal submission failed", formal, err)
	}
	// The team owner has disbanded this separate team. Its retained private repo
	// must never become a score merely because another team reaches assessment.
	f.exec(
		`UPDATE hack_teams SET state='disbanded' WHERE event_id=$1 AND id=$2`,
		f.event,
		discardedTeam,
	)
	f.exec(
		`UPDATE competition_entries SET active=false WHERE event_id=$1 AND team_id=$2`,
		f.event,
		discardedTeam,
	)
	f.now = f.config.SubmissionCutoff
	frozen, err := output.Read(t.Context(), f.member.ID, f.event, f.team)
	if err != nil || frozen.Outcome != "frozen" || frozen.Final == nil ||
		frozen.Final.Revision != formal.Final.Revision ||
		frozen.FrozenAt == nil {
		t.Fatal("actual native common cutoff did not freeze exact formal output", frozen, err)
	}
	fresh, err := f.s.Change(
		t.Context(),
		f.primary.ID,
		f.event,
		f.team,
		f.revision(),
		f.command(final, "save", false),
	)
	if err != nil || fresh.Reference.ID == live.Reference.ID || fresh.SubmissionRevision == nil ||
		*fresh.SubmissionRevision != frozen.Final.Revision ||
		fresh.Purpose != "mentor_final" {
		t.Fatal("fresh native final source binding failed", fresh, err)
	}
	for _, category := range fresh.Scores {
		if !category.Pending || category.Value != nil || len(category.Evidence) != 0 {
			t.Fatal(
				"provisional score/evidence carried into final without fresh independent review",
				category,
			)
		}
	}
	for _, unused := range []uuid.UUID{otherTeam, discardedTeam} {
		if _, err = f.s.Change(
			t.Context(),
			f.primary.ID,
			f.event,
			unused,
			f.revision(),
			f.command(final, "save", false),
		); err == nil {
			t.Fatal("unused/unsubmitted repository became final assessment", unused)
		}
	}
	page, err := board.Read(t.Context(), f.owner.ID, f.event, uuid.Nil, nil, 25)
	if err != nil || len(page.Teams) != 3 {
		t.Fatal("multi-team coordination missing", page, err)
	}
	states := map[uuid.UUID]Team{}
	for _, team := range page.Teams {
		states[team.ID] = team
	}
	if states[otherTeam].Assessment.OutputState != "not_submitted" ||
		states[otherTeam].Assessment.Availability != "not_applicable" ||
		states[discardedTeam].Assessment.OutputState != "disbanded" ||
		states[discardedTeam].Assessment.Availability != "not_applicable" ||
		states[discardedTeam].Assessment.Final.RequiredReviews != 0 {
		t.Fatal("unused outputs displayed as awaiting qualification", states)
	}
	encoded, _ := json.Marshal(page)
	for _, private := range []string{"QUIET-CONTRIBUTOR-FACT", `"scores":`, `"value":`, `"rank":`, `"commit_count":`, `"comparison":`} {
		if strings.Contains(string(encoded), private) {
			t.Fatal("private evidence or activity comparison entered coordination", private)
		}
	}
	judge := testdb.Person(t, f.pool)
	f.exec(
		`INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'judge')`,
		f.event,
		judge.ID,
	)
	for _, actor := range []uuid.UUID{f.member.ID, otherMember.ID, judge.ID} {
		if _, err = f.s.Read(
			t.Context(),
			actor,
			f.event,
			f.team,
			uuid.Nil,
			50,
		); !errors.Is(
			err,
			access.ErrForbidden,
		) {
			t.Fatal("attendee/unassigned judge gained independent assessment", actor, err)
		}
		if _, err = notes.Read(
			t.Context(),
			actor,
			f.event,
			f.team,
		); !errors.Is(
			err,
			access.ErrForbidden,
		) {
			t.Fatal("attendee/unassigned judge gained private quiet-contributor notes", actor, err)
		}
	}
}

func (f *fixture) journeyTeam(name string) (uuid.UUID, dbgen.Person) {
	f.t.Helper()
	id, entry := uuid.New(), uuid.New()
	member := testdb.Person(f.t, f.pool)
	f.exec(
		`INSERT INTO hack_teams(event_id,id,name,state,frozen)VALUES($1,$2,$3,'published',true)`,
		f.event,
		id,
		name,
	)
	f.exec(
		`INSERT INTO competition_entries(id,event_id,team_id)VALUES($1,$2,$3)`,
		entry,
		f.event,
		id,
	)
	f.exec(
		`INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')`,
		f.event,
		member.ID,
	)
	f.exec(
		`INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot)VALUES($1,$2,$3,'published')`,
		f.event,
		id,
		member.ID,
	)
	f.exec(
		`INSERT INTO policy_resources(event_id,id,kind)VALUES($1,$2,'team'),($1,$3,'assessment')`,
		f.event,
		id,
		entry,
	)
	for _, assignment := range []struct {
		person uuid.UUID
		slot   string
	}{{f.primary.ID, "primary"}, {f.second.ID, "second"}} {
		f.exec(
			`INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at)VALUES($1,$2,$3,$4,'published',$5,$6)`,
			f.event,
			id,
			assignment.person,
			assignment.slot,
			f.config.LiveStart,
			f.config.LiveEnd,
		)
	}
	f.exec(
		`UPDATE event_gate_facts SET total_teams=total_teams+1,frozen_teams=frozen_teams+1,assigned_attendees=assigned_attendees+1 WHERE event_id=$1`,
		f.event,
	)
	return id, member
}

func (f *fixture) assessmentSnapshot() string {
	f.t.Helper()
	// These are actual persisted owning records, not a recreated policy oracle.
	tables := []string{
		"mentor_observation_versions",
		"mentor_scorecard_versions",
		"mentor_scorecard_signoffs",
		"moderation_huddle_versions",
		"rubric_assessment_runs",
	}
	values := map[string]json.RawMessage{}
	for _, table := range tables {
		var raw []byte
		query := fmt.Sprintf(
			`SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text),'[]'::jsonb) FROM %s r WHERE event_id=$1`,
			table,
		)
		if err := f.pool.QueryRow(f.t.Context(), query, f.event).Scan(&raw); err != nil {
			f.t.Fatal(err)
		}
		values[table] = raw
	}
	raw, err := json.Marshal(values)
	if err != nil {
		f.t.Fatal(err)
	}
	return string(raw)
}
