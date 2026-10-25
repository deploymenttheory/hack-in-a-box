package qualification

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
	"github.com/deploymenttheory/hack-in-a-box/internal/moderation"
	"github.com/deploymenttheory/hack-in-a-box/internal/observations"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissionfreeze"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestPanelMembershipDoesNotDisclosePrivateObservationReferenceOrAttachedNote(t *testing.T) {
	f := setup(t)
	cp := f.configure("mentor_final")
	f.freeze()
	f.now = f.config.SubmissionCutoff
	notes := observations.Service{Pool: f.pool, Now: func() time.Time { return f.now }}
	noteID := uuid.New()
	if _, err := notes.Change(
		t.Context(),
		f.primary.ID,
		f.event,
		f.team,
		f.revision(),
		observations.Command{
			Action: "observation",
			ID:     noteID,
			Evidence: observations.Evidence{
				Category: "Build",
				Evidence: "PRIVATE-NATIVE-OBSERVATION",
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	outsider := testdb.Person(t, f.pool)
	f.exec(
		"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mentor')",
		f.event,
		outsider.ID,
	)
	if _, err := notes.Read(
		t.Context(),
		outsider.ID,
		f.event,
		f.team,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("fixture panel-only mentor gained native notes", err)
	}
	f.submit(cp, f.primary.ID, true)
	f.submit(cp, f.second.ID, true)
	huddles := f.huddles()
	h := f.create(huddles, cp)
	h = f.change(huddles, h, f.second.ID, moderation.Command{Action: "complete_second_review"})
	h = f.change(huddles, h, f.owner.ID, moderation.Command{Action: "open"})
	f.change(
		huddles,
		h,
		f.owner.ID,
		moderation.Command{Action: "sign_off", Reason: "Actual complete independent final source"},
	)
	s := Service{
		Pool:       f.pool,
		Now:        func() time.Time { return f.now },
		Cards:      &f.s,
		Moderation: huddles,
		Freeze:     &submissionfreeze.Service{Pool: f.pool, Now: func() time.Time { return f.now }},
	}
	p, err := s.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		cp.Definition.RunID,
		f.revision(),
		Command{
			RequestID: uuid.New(),
			Action:    "configure_panel",
			Reason:    "Named result panel does not grant observation access",
			Panel: &Panel{
				MentorIDs:   []uuid.UUID{f.primary.ID, f.second.ID, outsider.ID},
				OrganiserID: f.owner.ID,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	value, version := 5, int64(1)
	scores := []scorecards.CategoryScore{
		{
			CategoryID: cp.Criteria.Categories[0].ID,
			Value:      &value,
			Evidence: []scorecards.EvidenceReference{
				{Kind: "note", Note: "Explicit panel-facing moderated conclusion"},
				{
					Kind:    "observation",
					ID:      &noteID,
					Version: &version,
					Note:    "PRIVATE-ATTACHED-OBSERVATION-NOTE",
				},
			},
		},
	}
	p, err = s.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		cp.Definition.RunID,
		f.revision(),
		Command{
			RequestID:     uuid.New(),
			Version:       p.Round.Version,
			FreezeBinding: qualificationBinding(p, f.team),
			Action:        "save_card",
			Reason:        "Explicit moderated evidence",
			Card:          &CardInput{TeamID: f.team, Scores: scores},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := s.Read(t.Context(), outsider.ID, f.event, cp.Definition.RunID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(reader)
	if strings.Contains(string(raw), noteID.String()) ||
		strings.Contains(string(raw), "PRIVATE-ATTACHED") {
		t.Fatal("panel membership disclosed private observation reference or note", string(raw))
	}
	if len(reader.Round.Cards) != 1 || len(reader.Round.Cards[0].Scores[0].Evidence) != 1 {
		t.Fatal("panel-facing moderated result was lost", reader.Round.Cards)
	}
	history, err := s.History(t.Context(), outsider.ID, f.event, cp.Definition.RunID, nil, 20)
	if err != nil {
		t.Fatal(err)
	}
	retained, _ := json.Marshal(history)
	if strings.Contains(string(retained), noteID.String()) ||
		strings.Contains(string(retained), "PRIVATE-ATTACHED") {
		t.Fatal("history bypassed recipient evidence authority", string(retained))
	}
	// Recipient filtering must never mutate the authored immutable source.
	owner, err := s.Read(t.Context(), f.owner.ID, f.event, cp.Definition.RunID)
	if err != nil || len(owner.Round.Cards) != 1 ||
		len(owner.Round.Cards[0].Scores[0].Evidence) != 2 {
		t.Fatal("recipient projection changed native authored evidence", owner.Round.Cards, err)
	}
}

func TestCurrentPanelConflictClearsWholePrivateCohortAndStoredSelection(t *testing.T) {
	values := [][]int{
		{10, 9, 10, 9, 9},
		{9, 8, 9, 8, 8},
		{8, 7, 8, 7, 7},
		{7, 6, 7, 6, 6},
		{1, 10, 1, 1, 1},
	}
	f, s, cp, p := signedQualificationCohort(t, values)
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
			Reason:       "Explicit current mentor profile",
			PersonID:     reader.ID,
			Availability: []mentors.Window{{Start: f.config.LiveStart, End: f.config.LiveEnd}},
		},
	); err != nil {
		t.Fatal(err)
	}
	panel := p.Round.Panel
	panel.MentorIDs = append(panel.MentorIDs, reader.ID)
	p, err := s.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		cp.Definition.RunID,
		f.revision(),
		Command{
			RequestID: uuid.New(),
			Version:   p.Round.Version,
			Action:    "configure_panel",
			Panel:     &panel,
			Reason:    "Separate recorded qualification result panel",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		cp.Definition.RunID,
		f.revision(),
		Command{
			RequestID: uuid.New(),
			Version:   p.Round.Version,
			Action:    "propose",
			Reason:    "Current complete source cohort",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.Read(t.Context(), reader.ID, f.event, cp.Definition.RunID)
	if err != nil || len(before.Round.Cards) != 5 || before.Round.Proposal == nil {
		t.Fatal("fixture reader did not hold complete panel projection", before, err)
	}
	if _, err = native.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		f.revision(),
		mentors.Command{
			Action:       "conflict",
			Reason:       "Actual declared reporting conflict",
			PersonID:     reader.ID,
			CompetitorID: f.teams[0].member.ID,
			Kind:         "reporting",
			Enabled:      true,
		},
	); err != nil {
		t.Fatal(err)
	}
	after, err := s.Read(t.Context(), reader.ID, f.event, cp.Definition.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Round.Cards) != 0 || len(after.Teams) != 0 || after.Round.Proposal != nil ||
		len(after.Round.Votes) != 0 ||
		len(after.Round.Decisions) != 0 ||
		after.Round.Signoff != nil ||
		after.Current {
		t.Fatal("current conflict retained a private cohort or stored selection", after)
	}
	history, err := s.History(t.Context(), reader.ID, f.event, cp.Definition.RunID, nil, 20)
	if err != nil || len(history.Items) != 0 {
		t.Fatal("history bypassed current cohort conflict", history, err)
	}
}
