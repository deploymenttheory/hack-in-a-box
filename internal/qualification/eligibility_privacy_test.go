package qualification

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestCurrentConflictWithExcludedTeamScrubsEligibilityAndRoundProseWithoutChangingHistory(
	t *testing.T,
) {
	f := setupPool(t, testdb.New(t, true), 2)
	cp := f.configure("mentor_final")
	reader := testdb.Person(t, f.pool)
	f.exec(
		"INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'mentor')",
		f.event,
		reader.ID,
	)
	native := mentors.Service{Pool: f.pool}
	if _, err := native.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		f.revision(),
		mentors.Command{
			Action:       "profile",
			Reason:       "Separate current qualification panel reader",
			PersonID:     reader.ID,
			Availability: []mentors.Window{{Start: f.config.LiveStart, End: f.config.LiveEnd}},
		},
	); err != nil {
		t.Fatal(err)
	}
	// Submit the other team's real URL before the irreversible shared cutoff.
	deliverable := uuid.New()
	defs := templates.EmptyDefinitions()
	defs.Deliverables = []templates.Deliverable{
		{
			ID:            deliverable,
			Title:         "Independent hosted final",
			Required:      true,
			EvidenceKinds: []string{"url"},
			Audience:      []string{"mentor", "judge", "organiser"},
		},
	}
	rawDefs, _ := json.Marshal(defs)
	if err := dbgen.New(f.pool).
		SaveEventDefinitions(t.Context(), dbgen.SaveEventDefinitionsParams{EventID: f.event, Definitions: rawDefs}); err != nil {
		t.Fatal(err)
	}
	nativeSubmissions := submissions.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	second, err := nativeSubmissions.Read(
		t.Context(),
		f.teams[1].member.ID,
		f.event,
		f.teams[1].team,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = nativeSubmissions.Submit(
		t.Context(),
		f.teams[1].member.ID,
		f.event,
		f.teams[1].team,
		second.Revision,
		submissions.Command{
			Evidence: []submissions.Evidence{
				{
					DeliverableID: deliverable,
					Kind:          "url",
					Label:         "Real formal remaining-team build",
					URL:           "https://synthetic.invalid/remaining-final",
				},
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	freeze := nativeManagedFinal(f)
	f.useTeam(f.teams[1])
	nativeFinalProof(f, cp, f.huddles())
	f.useTeam(f.teams[0])
	s := Service{
		Pool:       f.pool,
		Now:        func() time.Time { return f.now },
		Cards:      &f.s,
		Moderation: f.huddles(),
		Freeze:     freeze,
	}
	panel := panelCommand(f)
	panel.Panel.MentorIDs = append(panel.Panel.MentorIDs, reader.ID)
	p, err := s.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		cp.Definition.RunID,
		f.revision(),
		panel,
	)
	if err != nil {
		t.Fatal(err)
	}
	f.useTeam(f.teams[1])
	p = reviewedCard(f, s, cp.Definition.RunID, p, 7)
	f.useTeam(f.teams[0])
	const canary = "PRIVATE-EXCLUDED-TEAM-ELIGIBILITY-REASON"
	p, err = s.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		cp.Definition.RunID,
		f.revision(),
		Command{
			RequestID:     uuid.New(),
			Version:       p.Round.Version,
			Action:        "decide_eligibility",
			Reason:        canary,
			TeamID:        f.team,
			FreezeBinding: qualificationBinding(p, f.team),
			Decision:      "ineligible",
			Evidence: []scorecards.EvidenceReference{
				{Kind: "note", Note: "PRIVATE-EXCLUDED-EVIDENCE"},
			},
		},
	)
	excludedAt := slices.IndexFunc(
		p.Teams,
		func(team TeamStatus) bool { return team.TeamID == f.teams[0].team },
	)
	if err != nil || len(p.Round.Eligibility) != 1 || excludedAt < 0 ||
		p.Teams[excludedAt].Eligible {
		t.Fatal("actual explicit exclusion missing", p, err)
	}
	snapshot := func() string {
		t.Helper()
		var raw string
		if e := f.pool.QueryRow(t.Context(), "SELECT jsonb_build_object('rounds',(SELECT jsonb_agg(to_jsonb(r) ORDER BY version) FROM mentor_qualification_rounds r WHERE event_id=$1),'commands',(SELECT jsonb_agg(to_jsonb(c) ORDER BY request_id) FROM mentor_qualification_commands c WHERE event_id=$1))::text", f.event).
			Scan(&raw); e != nil {
			t.Fatal(e)
		}
		return raw
	}
	retained := snapshot()
	before, err := s.Read(t.Context(), reader.ID, f.event, cp.Definition.RunID)
	if err != nil || before.Round.Reason != canary || len(before.Round.Eligibility) != 1 ||
		before.Round.Eligibility[0].Reason != canary {
		t.Fatal("intended panel did not initially own the private exclusion", before, err)
	}
	if _, err = native.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		f.revision(),
		mentors.Command{
			Action:       "conflict",
			Reason:       "Actual reporting conflict with excluded competitor",
			PersonID:     reader.ID,
			CompetitorID: f.member.ID,
			Kind:         "reporting",
			Enabled:      true,
		},
	); err != nil {
		t.Fatal(err)
	}
	current, err := s.Read(t.Context(), reader.ID, f.event, cp.Definition.RunID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(current)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), canary) ||
		strings.Contains(string(raw), "PRIVATE-EXCLUDED-EVIDENCE") {
		t.Error(
			"current excluded-team conflict retained private eligibility/top-level prose",
			string(raw),
		)
	}
	if len(current.Round.Cards) != 1 || current.Round.Cards[0].TeamID != f.teams[1].team ||
		*current.Round.Cards[0].Scores[0].Value != 7 ||
		current.Preview == nil ||
		len(current.Preview.Rankings) != 1 ||
		current.Preview.Rankings[0].TeamID != f.teams[1].team {
		t.Error(
			"excluded-team conflict erased legitimate remaining-team card/selection preview",
			current,
		)
	}
	history, err := s.History(t.Context(), reader.ID, f.event, cp.Definition.RunID, nil, 20)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), canary) ||
		strings.Contains(string(raw), "PRIVATE-EXCLUDED-EVIDENCE") {
		t.Error(
			"historical excluded-team conflict retained private eligibility/top-level prose",
			string(raw),
		)
	}
	owner, err := s.Read(t.Context(), f.owner.ID, f.event, cp.Definition.RunID)
	if err != nil || len(owner.Round.Eligibility) != 1 ||
		owner.Round.Eligibility[0].Reason != canary ||
		owner.Round.Reason != canary ||
		owner.Teams[excludedAt].Eligible {
		t.Error("recipient projection erased legitimate exclusion or organiser record", owner, err)
	}
	if snapshot() != retained {
		t.Error("recipient conflict rewrote immutable original history/ACK bytes")
	}
	if _, err = native.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		f.revision(),
		mentors.Command{
			Action:       "conflict",
			Reason:       "Actual reporting conflict is explicitly removed",
			PersonID:     reader.ID,
			CompetitorID: f.member.ID,
			Kind:         "reporting",
			Enabled:      false,
		},
	); err != nil {
		t.Fatal(err)
	}
	restored, err := s.Read(t.Context(), reader.ID, f.event, cp.Definition.RunID)
	if err != nil || len(restored.Round.Eligibility) != 1 ||
		restored.Round.Eligibility[0].Reason != canary ||
		restored.Round.Reason != canary {
		t.Fatal("restored current authority did not recover its legitimate record", restored, err)
	}
	if snapshot() != retained {
		t.Fatal("scope restoration rewrote original history")
	}
}
