package reviewdashboard

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/feedback"
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
	"github.com/deploymenttheory/hack-in-a-box/internal/moderation"
	"github.com/deploymenttheory/hack-in-a-box/internal/observations"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestNativeScoreProposalFeedbackAndReassignmentRetainDistinctAuthorsAndReasons(t *testing.T) {
	f := setup(t)
	board := dashboard(f)
	notes := observations.Service{Pool: f.pool, Now: func() time.Time { return f.now }}
	noteID := uuid.New()
	command := observations.Command{
		Action: "observation",
		ID:     noteID,
		Evidence: observations.Evidence{
			Category: "demonstration",
			Evidence: "PRIVATE-ORIGINAL-OBSERVATION-HISTORY",
			Outcome:  "Initial observed factual outcome",
		},
	}
	page, err := notes.Change(t.Context(), f.primary.ID, f.event, f.team, f.revision(), command)
	if err != nil {
		t.Fatal(err)
	}
	first := page.Observations[0]
	f.now = f.now.Add(time.Minute)
	command.Version = first.Version
	command.Evidence.Outcome = "PRIVATE-AMENDED-OBSERVATION-HISTORY"
	page, err = notes.Change(t.Context(), f.primary.ID, f.event, f.team, f.revision(), command)
	if err != nil || page.Observations[0].AuthorID != f.primary.ID ||
		!page.Observations[0].ObservedAt.Equal(first.ObservedAt) {
		t.Fatal("observation edit reattributed original factual author/time", page, err)
	}
	if _, err = notes.Change(
		t.Context(),
		f.second.ID,
		f.event,
		f.team,
		f.revision(),
		command,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("second reviewer rewrote original author's observation", err)
	}
	history, err := notes.History(t.Context(), f.primary.ID, f.event, f.team, noteID)
	if err != nil || len(history.Observations) != 2 ||
		history.Observations[1].Evidence.Outcome != first.Evidence.Outcome {
		t.Fatal("original private evidence lost", history, err)
	}
	primary, err := f.s.Change(
		t.Context(),
		f.primary.ID,
		f.event,
		f.team,
		f.revision(),
		f.command(f.checkpoint, "submit", true),
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.s.Change(
		t.Context(),
		f.second.ID,
		f.event,
		f.team,
		f.revision(),
		f.command(f.checkpoint, "submit", true),
	)
	if err != nil {
		t.Fatal(err)
	}
	change := func(h moderation.Huddle, actor uuid.UUID, c moderation.Command) moderation.Huddle {
		t.Helper()
		c.Version = h.Version
		next, e := board.Moderation.Change(
			t.Context(),
			actor,
			f.event,
			f.team,
			h.Definition.ID,
			f.revision(),
			c,
		)
		if e != nil {
			t.Fatal(e)
		}
		return next
	}
	configure := func() moderation.Huddle {
		t.Helper()
		id := uuid.New()
		definition := moderation.Definition{
			ID:                id,
			CheckpointID:      f.checkpoint.Definition.ID,
			CheckpointVersion: f.checkpoint.Version,
			ModeratorID:       f.owner.ID,
			SecondReviewerID:  f.second.ID,
			ReviewDueAt:       f.checkpoint.ClosesAt.Add(-time.Minute),
		}
		h, e := board.Moderation.Change(
			t.Context(),
			f.owner.ID,
			f.event,
			f.team,
			id,
			f.revision(),
			moderation.Command{
				Action:     "configure",
				Definition: &definition,
				Reason:     "Explicit native independent panel and due review",
			},
		)
		if e != nil {
			t.Fatal(e)
		}
		return change(
			h,
			f.second.ID,
			moderation.Command{
				Action: "complete_second_review",
				Reason: "Own independent submitted card recorded",
			},
		)
	}
	planned := configure()
	opened := configure()
	opened = change(
		opened,
		f.owner.ID,
		moderation.Command{Action: "open", Reason: "Only after both independent submitted cards"},
	)
	value := primary.Scores[0].Value
	if value == nil {
		t.Fatal("fixture has no original independent score")
	}
	proposed := *value + 1
	proposalID := uuid.New()
	opened = change(
		opened,
		f.primary.ID,
		moderation.Command{
			Action:        "propose_change",
			ProposalID:    proposalID,
			Card:          primary.Reference,
			CategoryID:    primary.Scores[0].CategoryID,
			ProposedValue: &proposed,
			Evidence: []scorecards.EvidenceReference{
				{Kind: "note", Note: "Explicit additional demonstrated evidence"},
			},
			Reason: "EXPLICIT-PROPOSED-CHANGE-REASON",
		},
	)
	opened = change(
		opened,
		f.owner.ID,
		moderation.Command{
			Action:     "accept_change",
			ProposalID: proposalID,
			Reason:     "EXPLICIT-MODERATION-DECISION-REASON",
		},
	)
	proposal := opened.Proposals[0]
	if proposal.OriginalReviewerID != f.primary.ID || proposal.ProposedBy != f.primary.ID ||
		proposal.DecidedBy == nil ||
		*proposal.DecidedBy != f.owner.ID ||
		proposal.Reason != "EXPLICIT-PROPOSED-CHANGE-REASON" ||
		proposal.DecisionReason != "EXPLICIT-MODERATION-DECISION-REASON" ||
		proposal.Original.Value == nil ||
		*proposal.Original.Value != *value {
		t.Fatal("proposal conflated original author/evidence with formal decision", proposal)
	}
	own, err := f.s.Read(t.Context(), f.primary.ID, f.event, f.team, uuid.Nil, 50)
	if err != nil || len(own.Items) != 1 || own.Items[0].Reference != primary.Reference ||
		*own.Items[0].Scores[0].Value != *value {
		t.Fatal("accepted proposal overwrote independent original", own, err)
	}
	// Shared feedback is a deliberate new reviewed document, never an automatic
	// copy or implicit publication of the private observation/card above.
	feedbackID := uuid.New()
	shared, err := board.Feedback.Change(
		t.Context(),
		f.primary.ID,
		f.event,
		feedbackID,
		0,
		uuid.New(),
		feedback.Command{
			Action: "save",
			Input: &feedback.Input{
				TeamID:   f.team,
				Category: "competition_output",
				Title:    "Reviewed team coaching",
				Body:     "Explicitly share demonstrated recovery guidance",
				Sources:  []feedback.Source{},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"review", "publish"} {
		shared, err = board.Feedback.Change(
			t.Context(),
			f.owner.ID,
			f.event,
			feedbackID,
			shared.CurrentVersion,
			uuid.New(),
			feedback.Command{Action: action},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	if shared.Published == nil || shared.Published.AuthorID != f.primary.ID ||
		shared.Published.ReviewerID == nil ||
		*shared.Published.ReviewerID != f.owner.ID {
		t.Fatal("shared guidance lost distinct author/reviewer", shared)
	}
	attendee, err := board.Feedback.Read(t.Context(), f.member.ID, f.event, feedbackID)
	if err != nil || attendee.Published == nil || attendee.Draft != nil ||
		strings.Contains(attendee.Published.Body, "PRIVATE-") {
		t.Fatal("attendee feedback includes unreleased/private sources", attendee, err)
	}
	sharedHistory, err := board.Feedback.History(
		t.Context(),
		f.primary.ID,
		f.event,
		feedbackID,
		0,
		50,
	)
	if err != nil || len(sharedHistory.Items) != 3 ||
		sharedHistory.Items[2].AuthorID != f.primary.ID ||
		sharedHistory.Items[0].ReviewerID == nil ||
		*sharedHistory.Items[0].ReviewerID != f.owner.ID {
		t.Fatal("reviewed feedback immutable authorship history lost", sharedHistory, err)
	}
	replacement := testdb.Person(t, f.pool)
	f.exec(
		`INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'mentor')`,
		f.event,
		replacement.ID,
	)
	native := mentors.Service{Pool: f.pool}
	coverage := func(c mentors.Command) {
		t.Helper()
		c.Reason = "EXPLICIT-SECOND-REPLACEMENT-REASON"
		if _, e := native.Change(t.Context(), f.owner.ID, f.event, f.revision(), c); e != nil {
			t.Fatal(e)
		}
	}
	coverage(
		mentors.Command{
			Action:       "profile",
			PersonID:     replacement.ID,
			Availability: []mentors.Window{{Start: f.config.LiveStart, End: f.config.LiveEnd}},
		},
	)
	for _, slot := range []struct {
		slot  string
		actor uuid.UUID
	}{{"primary", f.primary.ID}, {"second", replacement.ID}} {
		coverage(
			mentors.Command{
				Action:   "assign",
				TeamID:   f.team,
				Slot:     slot.slot,
				PersonID: slot.actor,
				Window:   mentors.Window{Start: f.config.LiveStart, End: f.config.LiveEnd},
			},
		)
	}
	coverage(mentors.Command{Action: "publish"})
	definition := planned.Definition
	definition.SecondReviewerID = replacement.ID
	planned = change(
		planned,
		f.owner.ID,
		moderation.Command{
			Action:     "configure",
			Definition: &definition,
			Reason:     "Explicitly bind new published reviewer; retain former review history",
		},
	)
	if planned.Review.CompletedAt != nil || planned.Review.Card != nil ||
		planned.Review.ReviewerID != replacement.ID {
		t.Fatal("replacement inherited somebody else's independent completion", planned)
	}
	f.exec(
		`UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mentor'`,
		f.event,
		f.second.ID,
	)
	if _, err = f.s.Read(t.Context(), replacement.ID, f.event, f.team, uuid.Nil, 50); err != nil {
		t.Fatal("new native published second lacks own independent scope", err)
	}
	if _, err = f.s.History(
		t.Context(),
		replacement.ID,
		f.event,
		f.team,
		second.Reference.ID,
		nil,
		50,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("replacement received retired independent author history", err)
	}
	if _, err = board.Moderation.Panel(
		t.Context(),
		f.owner.ID,
		f.event,
		f.team,
		opened.Definition.ID,
	); !errors.Is(err, access.ErrConflict) &&
		!errors.Is(err, access.ErrForbidden) {
		t.Fatal("old source panel silently retargeted new coverage", err)
	}
	currentHistory, err := board.Moderation.History(
		t.Context(),
		f.primary.ID,
		f.event,
		f.team,
		planned.Definition.ID,
		nil,
		100,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, snapshot := range currentHistory.Items {
		if snapshot.Review.Card != nil && snapshot.Review.Card.ID == second.Reference.ID {
			t.Fatal("retired private author source escaped historical current-reader checks")
		}
	}
	var recorded int
	if err = f.pool.QueryRow(t.Context(), `SELECT count(*) FROM event_decisions WHERE event_id=$1 AND actor_id=$2 AND reason=$3 AND kind LIKE 'mentor_%'`, f.event, f.owner.ID, "EXPLICIT-SECOND-REPLACEMENT-REASON").
		Scan(&recorded); err != nil ||
		recorded != 4 {
		t.Fatal("native published reviewer changes lost actor/reason journal", recorded, err)
	}
}
