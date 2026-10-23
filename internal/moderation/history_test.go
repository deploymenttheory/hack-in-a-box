package moderation

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
	"github.com/deploymenttheory/hack-in-a-box/internal/observations"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestHistoryDoesNotGrantNewModeratorAccessToFormerPanelSnapshots(t *testing.T) {
	f := setup(t)
	s := f.huddles()
	h := f.create(s, f.checkpoint)
	newModerator := testdb.Person(t, f.pool)
	f.exec(
		`INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'organiser')`,
		f.event,
		newModerator.ID,
	)
	definition := h.Definition
	definition.ModeratorID = newModerator.ID
	next := f.change(
		s,
		h,
		f.owner.ID,
		Command{
			Action:     "configure",
			Definition: &definition,
			Reason:     "Explicitly replace intended panel moderator",
		},
	)
	page, err := s.History(t.Context(), newModerator.ID, f.event, f.team, h.Definition.ID, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Version != next.Version {
		t.Fatal("current moderator lost current history", page)
	}
	if page.NextCursor != nil {
		older, err := s.History(
			t.Context(),
			newModerator.ID,
			f.event,
			f.team,
			h.Definition.ID,
			page.NextCursor,
			1,
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(older.Items) != 0 {
			t.Fatal("latest panel granted access to former private panel", older)
		}
	}
	primary, err := s.History(t.Context(), f.primary.ID, f.event, f.team, h.Definition.ID, nil, 1)
	if err != nil || len(primary.Items) != 1 || primary.NextCursor == nil {
		t.Fatal("continuing intended panel member lost legitimate cursor", primary, err)
	}
	older, err := s.History(
		t.Context(),
		f.primary.ID,
		f.event,
		f.team,
		h.Definition.ID,
		primary.NextCursor,
		1,
	)
	if err != nil || len(older.Items) != 1 || older.Items[0].Version != h.Version {
		t.Fatal("continuing panel member lost legitimate history", older, err)
	}
}

func TestPlannedReplacementFiltersRetainedSecondReviewAfterNativeAuthorRevocation(t *testing.T) {
	f := setup(t)
	s := f.huddles()
	// This is a real private observation and real independent submitted card,
	// not a synthetic history document or a fabricated per-note permission.
	observationID := uuid.New()
	notes := observations.Service{Pool: f.pool, Now: func() time.Time { return f.now }}
	if _, err := notes.Change(
		t.Context(),
		f.second.ID,
		f.event,
		f.team,
		f.revision(),
		observations.Command{
			Action: "observation",
			ID:     observationID,
			Evidence: observations.Evidence{
				Category: "demo",
				Evidence: "PRIVATE-RETIRED-REVIEW-EVIDENCE",
				Outcome:  "Review current working output",
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	version := int64(1)
	command := f.command(f.checkpoint, "submit", true)
	command.Scores[0].Evidence = []scorecards.EvidenceReference{
		{Kind: "observation", ID: &observationID, Version: &version},
	}
	if _, err := f.s.Change(
		t.Context(),
		f.second.ID,
		f.event,
		f.team,
		f.revision(),
		command,
	); err != nil {
		t.Fatal(err)
	}
	h := f.create(s, f.checkpoint)
	h = f.change(s, h, f.second.ID, Command{Action: "complete_second_review"})
	originalVersion := h.Version
	before, err := s.History(t.Context(), f.primary.ID, f.event, f.team, h.Definition.ID, nil, 100)
	if err != nil || len(before.Items) != int(h.Version) {
		t.Fatal("original intended source history denied", err)
	}
	replacement := testdb.Person(t, f.pool)
	f.exec(
		`INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'mentor')`,
		f.event,
		replacement.ID,
	)
	native := mentors.Service{Pool: f.pool}
	changeCoverage := func(command mentors.Command) {
		t.Helper()
		command.Reason = "Explicit published second-review replacement"
		if _, err := native.Change(
			t.Context(),
			f.owner.ID,
			f.event,
			f.revision(),
			command,
		); err != nil {
			t.Fatal(err)
		}
	}
	changeCoverage(
		mentors.Command{
			Action:       "profile",
			PersonID:     replacement.ID,
			Availability: []mentors.Window{{Start: f.config.LiveStart, End: f.config.LiveEnd}},
		},
	)
	for _, assignment := range []struct {
		slot   string
		person uuid.UUID
	}{{"primary", f.primary.ID}, {"second", replacement.ID}} {
		changeCoverage(
			mentors.Command{
				Action:   "assign",
				TeamID:   f.team,
				Slot:     assignment.slot,
				PersonID: assignment.person,
				Window:   mentors.Window{Start: f.config.LiveStart, End: f.config.LiveEnd},
			},
		)
	}
	changeCoverage(mentors.Command{Action: "publish"})
	definition := h.Definition
	definition.SecondReviewerID = replacement.ID
	h = f.change(
		s,
		h,
		f.owner.ID,
		Command{
			Action:     "configure",
			Definition: &definition,
			Reason:     "Explicitly review new published second and discard prior independent review binding",
		},
	)
	f.exec(
		`UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mentor'`,
		f.event,
		f.second.ID,
	)
	if _, err := s.Read(t.Context(), f.primary.ID, f.event, f.team, h.Definition.ID); err != nil {
		t.Fatal("current replacement context became unreadable", err)
	}
	// Native #99 now denies the old author's source, independently of #104.
	err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		return notes.AuthorizeEvidenceInTransaction(
			t.Context(),
			tx,
			f.second.ID,
			f.event,
			f.team,
			observationID,
			1,
		)
	})
	if !errors.Is(err, access.ErrForbidden) {
		t.Fatal("old native source author still authorized", err)
	}
	after, err := s.History(t.Context(), f.primary.ID, f.event, f.team, h.Definition.ID, nil, 1)
	if err != nil || len(after.Items) != 1 || after.Items[0].Version != h.Version ||
		after.NextCursor == nil {
		t.Fatal("current/retained accessible history cursor lost", after, err)
	}
	earlier, err := s.History(
		t.Context(),
		f.primary.ID,
		f.event,
		f.team,
		h.Definition.ID,
		after.NextCursor,
		1,
	)
	if err != nil || len(earlier.Items) != 1 || earlier.Items[0].Version >= originalVersion ||
		earlier.Items[0].Review.Card != nil {
		t.Fatal("retired native second-review source leaked or broke keyset progress", earlier, err)
	}
}
