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
	"github.com/deploymenttheory/hack-in-a-box/internal/feedback"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func (f *fixture) feedback() *feedback.Service {
	return &feedback.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
}

func (f *fixture) huddles() *Service {
	return &Service{
		Pool:    f.pool,
		Clock:   func() time.Time { return f.now },
		Cards:   &f.s,
		Actions: FeedbackActions{Service: f.feedback()},
	}
}

func (f *fixture) nativeAction() (*feedback.Service, uuid.UUID, feedback.ActionView) {
	f.t.Helper()
	native := f.feedback()
	id := uuid.New()
	page, err := native.Change(
		f.t.Context(),
		f.owner.ID,
		f.event,
		id,
		0,
		uuid.New(),
		feedback.Command{
			Action: "save",
			Input: &feedback.Input{
				TeamID:   f.team,
				Category: "competition_output",
				Title:    "Reviewed output follow-up",
				Body:     "Shared support guidance, not private scores",
				Sources:  []feedback.Source{},
			},
		},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, action := range []string{"review", "publish"} {
		page, err = native.Change(
			f.t.Context(),
			f.owner.ID,
			f.event,
			id,
			page.CurrentVersion,
			uuid.New(),
			feedback.Command{Action: action},
		)
		if err != nil {
			f.t.Fatal(err)
		}
	}
	taskID := uuid.New()
	task, err := native.ChangeAction(
		f.t.Context(),
		f.owner.ID,
		f.event,
		id,
		taskID,
		0,
		uuid.New(),
		feedback.ActionCommand{
			Action: "details",
			Input: &feedback.ActionInput{
				Reviewed: true,
				Title:    "Check frozen demonstration evidence",
				Body:     "Operational action completion requires separate formal review",
				OwnerID:  f.member.ID,
				DueAt:    f.now.Add(time.Hour),
			},
		},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	return native, id, task
}

func (f *fixture) actionStatus(
	native *feedback.Service,
	feedbackID uuid.UUID,
	task feedback.ActionView,
	status string,
) feedback.ActionView {
	f.t.Helper()
	actor := f.member.ID
	if status == "open" {
		actor = f.owner.ID
	}
	next, err := native.ChangeAction(
		f.t.Context(),
		actor,
		f.event,
		feedbackID,
		task.Action.ID,
		task.Action.Version,
		uuid.New(),
		feedback.ActionCommand{Action: "status", Status: status},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	return next
}

func (f *fixture) create(s *Service, cp scorecards.Checkpoint) Huddle {
	f.t.Helper()
	id := uuid.New()
	h, err := s.Change(
		f.t.Context(),
		f.owner.ID,
		f.event,
		f.team,
		id,
		f.revision(),
		Command{
			Action: "configure",
			Definition: &Definition{
				ID:                id,
				CheckpointID:      cp.Definition.ID,
				CheckpointVersion: cp.Version,
				ModeratorID:       f.owner.ID,
				SecondReviewerID:  f.second.ID,
				ReviewDueAt:       cp.ClosesAt.Add(-time.Minute),
			},
			Reason: "Explicit intended panel and actual second coverage",
		},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	return h
}

func (f *fixture) change(s *Service, h Huddle, actor uuid.UUID, command Command) Huddle {
	f.t.Helper()
	command.Version = h.Version
	if command.Reason == "" {
		command.Reason = "Explicit huddle decision"
	}
	out, err := s.Change(
		f.t.Context(),
		actor,
		f.event,
		f.team,
		h.Definition.ID,
		f.revision(),
		command,
	)
	if err != nil {
		f.t.Fatal(err)
	}
	return out
}

func (f *fixture) submit(cp scorecards.Checkpoint, actor uuid.UUID, complete bool) scorecards.Card {
	f.t.Helper()
	card, err := f.s.Change(
		f.t.Context(),
		actor,
		f.event,
		f.team,
		f.revision(),
		f.command(cp, "submit", complete),
	)
	if err != nil {
		f.t.Fatal(err)
	}
	return card
}

func TestHuddleRequiresIndependentSubmittedReviewsAndIntendedCurrentPanel(t *testing.T) {
	f := setup(t)
	s := f.huddles()
	h := f.create(s, f.checkpoint)
	if _, err := s.Panel(
		t.Context(),
		f.owner.ID,
		f.event,
		f.team,
		h.Definition.ID,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("planned context disclosed independent drafts", err)
	}
	primary, err := f.s.Change(
		t.Context(),
		f.primary.ID,
		f.event,
		f.team,
		f.revision(),
		f.command(f.checkpoint, "save", true),
	)
	if err != nil {
		t.Fatal(err)
	}
	f.submit(f.checkpoint, f.second.ID, true)
	h = f.change(s, h, f.second.ID, Command{Action: "complete_second_review"})
	if _, err = s.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		f.team,
		h.Definition.ID,
		f.revision(),
		Command{
			Action:  "open",
			Version: h.Version,
			Reason:  "Missing primary independent submission",
		},
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("draft counted as required review", err)
	}
	command := f.command(f.checkpoint, "submit", true)
	command.Version = primary.Reference.Version
	if _, err = f.s.Change(
		t.Context(),
		f.primary.ID,
		f.event,
		f.team,
		f.revision(),
		command,
	); err != nil {
		t.Fatal(err)
	}
	h = f.change(s, h, f.owner.ID, Command{Action: "open"})
	for _, actor := range []uuid.UUID{f.owner.ID, f.primary.ID, f.second.ID} {
		cards, e := s.Panel(t.Context(), actor, f.event, f.team, h.Definition.ID)
		if e != nil || len(cards) != 2 {
			t.Fatalf("intended panel lost exact submitted context: %d %v", len(cards), e)
		}
	}
	outsider := testdb.Person(t, f.pool)
	f.exec(
		`INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'organiser')`,
		f.event,
		outsider.ID,
	)
	if _, err = s.Panel(
		t.Context(),
		outsider.ID,
		f.event,
		f.team,
		h.Definition.ID,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("generic organiser received another panel context", err)
	}
	if _, err = f.s.History(
		t.Context(),
		f.second.ID,
		f.event,
		f.team,
		primary.Reference.ID,
		nil,
		10,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("huddle exposed another author's independent draft history", err)
	}
	f.exec(
		`INSERT INTO mentor_conflicts(event_id,mentor_id,competitor_id,kind)VALUES($1,$2,$3,'reporting')`,
		f.event,
		f.second.ID,
		f.member.ID,
	)
	if _, err = s.Panel(t.Context(), f.owner.ID, f.event, f.team, h.Definition.ID); err == nil {
		t.Fatal("organiser bypassed current review conflict")
	}
}

func TestHuddlePreliveDistinctReviewerAndAbsentActionOwnerFailClosed(t *testing.T) {
	f := setup(t)
	s := f.huddles()
	h := f.create(s, f.checkpoint)
	f.now = f.config.LiveStart.Add(-time.Nanosecond)
	if _, err := s.Read(
		t.Context(),
		f.owner.ID,
		f.event,
		f.team,
		h.Definition.ID,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("prelive huddle review opened", err)
	}
	f.now = f.config.LiveStart
	d := h.Definition
	d.ID = uuid.New()
	d.SecondReviewerID = f.primary.ID
	if _, err := s.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		f.team,
		d.ID,
		f.revision(),
		Command{Action: "configure", Definition: &d, Reason: "Try primary as independent second"},
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("primary counted as distinct second", err)
	}
	f.submit(f.checkpoint, f.primary.ID, true)
	f.submit(f.checkpoint, f.second.ID, true)
	h = f.change(s, h, f.second.ID, Command{Action: "complete_second_review"})
	s.Actions = nil
	if _, err := s.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		f.team,
		h.Definition.ID,
		f.revision(),
		Command{Action: "open", Version: h.Version, Reason: "No native action owner"},
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("absent action owner treated as reviewed", err)
	}
}

func TestFrozenFinalSignoffRequiresExplicitGapAndProposalDecisions(t *testing.T) {
	f := setup(t)
	s := f.huddles()
	cp := f.configure("mentor_final")
	f.freeze()
	f.now = f.config.SubmissionCutoff
	first := f.submit(cp, f.primary.ID, true)
	f.submit(cp, f.second.ID, true)
	h := f.create(s, cp)
	h = f.change(s, h, f.second.ID, Command{Action: "complete_second_review"})
	h = f.change(s, h, f.owner.ID, Command{Action: "open"})
	native, feedbackID, task := f.nativeAction()
	task = f.actionStatus(native, feedbackID, task, "done")
	actionID, gapID, proposalID := task.Action.ID, uuid.New(), uuid.New()
	h = f.change(
		s,
		h,
		f.owner.ID,
		Command{
			Action:        "add_gap",
			GapID:         gapID,
			ActionID:      actionID,
			ActionVersion: task.Action.Version,
			Reason:        "Independent supporting evidence still needs formal review",
		},
	)
	deniedSignoff := func() {
		t.Helper()
		if _, err := s.Change(
			t.Context(),
			f.owner.ID,
			f.event,
			f.team,
			h.Definition.ID,
			f.revision(),
			Command{Action: "sign_off", Version: h.Version, Reason: "Try incomplete finalisation"},
		); !errors.Is(
			err,
			access.ErrConflict,
		) {
			t.Fatal("incomplete moderation finalised", err)
		}
	}
	deniedSignoff()
	evidence := []scorecards.EvidenceReference{
		{Kind: "note", Note: "Explicit formal review of current frozen evidence"},
	}
	h = f.change(
		s,
		h,
		f.owner.ID,
		Command{
			Action:        "resolve_gap",
			GapID:         gapID,
			ActionVersion: task.Action.Version,
			Evidence:      evidence,
			Reason:        "Native operational done plus explicit formal evidence decision",
		},
	)
	value := first.Scores[0].Value
	h = f.change(
		s,
		h,
		f.primary.ID,
		Command{
			Action:        "propose_change",
			ProposalID:    proposalID,
			Card:          first.Reference,
			CategoryID:    first.Scores[0].CategoryID,
			ProposedValue: value,
			Evidence:      evidence,
			Reason:        "Proposed score interpretation with immutable original evidence",
		},
	)
	deniedSignoff()
	h = f.change(
		s,
		h,
		f.owner.ID,
		Command{
			Action:     "accept_change",
			ProposalID: proposalID,
			Reason:     "Explicitly accept recorded proposed interpretation",
		},
	)
	task = f.actionStatus(native, feedbackID, task, "open")
	deniedSignoff()
	task = f.actionStatus(native, feedbackID, task, "done")
	h = f.change(
		s,
		h,
		f.owner.ID,
		Command{
			Action:        "resolve_gap",
			GapID:         gapID,
			ActionVersion: task.Action.Version,
			Evidence:      evidence,
			Reason:        "Explicitly re-review reopened operational action",
		},
	)
	h = f.change(
		s,
		h,
		f.owner.ID,
		Command{
			Action: "sign_off",
			Reason: "All independent final cards and current evidence reviewed",
		},
	)
	if h.State != "signed" || h.SignoffID == nil || h.SignoffContext == nil {
		t.Fatal("canonical signoff proof absent")
	}
	var signoffs int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM mentor_scorecard_signoffs WHERE event_id=$1`, f.event).
		Scan(&signoffs); err != nil ||
		signoffs != 1 {
		t.Fatal("native signoff identity missing", signoffs, err)
	}
	retained, err := f.s.History(
		t.Context(),
		f.primary.ID,
		f.event,
		f.team,
		first.Reference.ID,
		nil,
		10,
	)
	if err != nil || len(retained.Items) != 1 || binding(retained.Items[0]) != binding(first) {
		t.Fatal("moderation changed independent original", err)
	}
	if h.Proposals[0].OriginalReviewerID != first.AuthorID ||
		binding(h.Proposals[0].Original) != binding(first.Scores[0]) {
		t.Fatal("proposal lost original reviewer/value/evidence")
	}
	var historical History
	historical, err = s.History(t.Context(), f.owner.ID, f.event, f.team, h.Definition.ID, nil, 100)
	if err != nil || len(historical.Items) < 7 {
		t.Fatal("decision history missing", err)
	}
	canonical := f.s
	canonical.Moderation = s
	if err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		_, e := canonical.SignOffInTransaction(
			t.Context(),
			tx,
			f.owner.ID,
			f.event,
			f.team,
			h.Definition.ID,
			h.Cards,
		)
		return e
	}); err != nil {
		t.Fatal("unchanged real owner proof is not idempotent", err)
	}
	f.exec(`UPDATE hack_teams SET state='disbanded' WHERE event_id=$1 AND id=$2`, f.event, f.team)
	if _, err = s.Panel(t.Context(), f.owner.ID, f.event, f.team, h.Definition.ID); err == nil {
		t.Fatal("disbanded team kept assessment context")
	}
}
