package app

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/deploymenttheory/hack-in-a-box/internal/moderation"
	"github.com/deploymenttheory/hack-in-a-box/internal/qualification"
	"github.com/deploymenttheory/hack-in-a-box/internal/results"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

const resultsMentorPrivateNote = "PRIVATE-MENTOR-FINAL-PROSE-NEVER-A-RESULT"

// Owns real native publication, final submission, selection and two independent
// judge identities. Cards/deliberation remain explicit test steps so callers can
// exercise blockers and later prepare/confirmation/publication against this source.
// No provider, fake finalist policy, fabricated signoff or clock rewind is used.
type resultsSourceFixture struct {
	runtime                          *liveRehearsal
	admin, public                    rehearsalSurface
	service                          results.Service
	judges                           [2]dbgen.Person
	secondMentor                     dbgen.Person
	checkpoint                       scorecards.Checkpoint
	mentorRun, judgeRun, deliverable uuid.UUID
}

func newResultsSourceFixture(t *testing.T, mode string) *resultsSourceFixture {
	t.Helper()
	f := &resultsSourceFixture{}
	r, admin, public, run := qualificationHTTPFixtureWithCriteria(t, func(d *rubric.Definition) {
		d.Mode = mode
		d.RequiredJudgeCount = 2
		d.ReserveCount = 0
		d.FinalistCount = 0
		if mode == rubric.MentorQualification {
			d.FinalistCount = 1
		}
		d.Categories[0].Weight = 2
	}, func(r *liveRehearsal, s rehearsalSurface, run uuid.UUID) {
		f.runtime = r
		f.admin = s
		f.deliverable = uuid.New()
		defs := templates.EmptyDefinitions()
		defs.Deliverables = []templates.Deliverable{
			{
				ID:            f.deliverable,
				Title:         "Actual frozen result build",
				Required:      true,
				EvidenceKinds: []string{"url"},
				Audience:      []string{"organiser", "mentor", "judge"},
			},
		}
		raw, err := json.Marshal(defs)
		resultsSourceRequire(t, err)
		resultsSourceRequire(
			t,
			dbgen.New(r.pool).
				SaveEventDefinitions(t.Context(), dbgen.SaveEventDefinitionsParams{EventID: r.eventID, Definitions: raw}),
		)
		p, err := s.deps.Submissions.Read(t.Context(), r.competitor.ID, r.eventID, r.teamID)
		resultsSourceRequire(t, err)
		_, err = s.deps.Submissions.Submit(
			t.Context(),
			r.competitor.ID,
			r.eventID,
			r.teamID,
			p.Revision,
			submissions.Command{
				Reason: "Real formal native URL build",
				Evidence: []submissions.Evidence{
					{
						DeliverableID: f.deliverable,
						Kind:          "url",
						Label:         "Native result build",
						URL:           "https://synthetic.invalid/results-build",
					},
				},
			},
		)
		resultsSourceRequire(t, err)
		if mode == rubric.MentorQualification {
			f.prepareMentors(t, s, run)
		}
	})
	f.runtime = r
	f.admin = admin
	f.public = public
	policy, ok := f.admin.deps.Judging.Finalists.(*qualification.Service)
	if !ok || policy.Pool != r.pool || policy.Moderation != f.admin.deps.Moderation {
		t.Fatal("results fixture lacks the actual native finalist owner")
	}
	f.service = results.Service{
		Pool:          r.pool,
		Now:           r.clock.read,
		Judging:       &f.admin.deps.Judging,
		Qualification: policy,
	}
	frozen, err := f.admin.deps.Submissions.Read(t.Context(), r.owner.ID, r.eventID, r.teamID)
	resultsSourceRequire(t, err)
	if frozen.Final == nil || frozen.Outcome != "frozen" {
		t.Fatal("formal native submission did not freeze", frozen)
	}
	if mode == rubric.MentorQualification {
		f.mentorRun = run
		f.signQualification(t, policy)
		config, err := f.admin.deps.Rubrics.Configuration(t.Context(), r.owner.ID, r.eventID, 0)
		resultsSourceRequire(t, err)
		config, err = f.admin.deps.Rubrics.Change(
			t.Context(),
			r.owner.ID,
			r.eventID,
			config.Revision,
			rubric.Command{
				Action:  "create_run",
				Purpose: "judge_final",
				Version: config.Reference.Version,
				Reason:  "Fresh independent judge final after native qualification",
			},
		)
		resultsSourceRequire(t, err)
		for _, current := range config.Runs {
			if current.Purpose == "judge_final" {
				f.judgeRun = current.ID
			}
		}
	} else {
		f.judgeRun = run
	}
	if f.judgeRun == uuid.Nil {
		t.Fatal("actual judge-final run absent")
	}
	for i := range f.judges {
		f.judges[i] = testdb.Person(t, r.pool)
		resultsSourceRequire(
			t,
			dbgen.New(r.pool).
				SetMembership(t.Context(), dbgen.SetMembershipParams{EventID: r.eventID, PersonID: f.judges[i].ID, Role: "judge", Active: true}),
		)
	}
	f.round(
		t,
		judging.RoundCommand{
			Action:   "assign",
			TeamID:   r.teamID,
			JudgeIDs: []uuid.UUID{f.judges[0].ID, f.judges[1].ID},
		},
	)
	return f
}

func resultsSourceRequire(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func (f *resultsSourceFixture) revision(t *testing.T) int64 {
	t.Helper()
	p, err := f.admin.deps.Events.Read(t.Context(), f.runtime.owner.ID, f.runtime.eventID)
	resultsSourceRequire(t, err)
	return p.Revision
}

func (f *resultsSourceFixture) round(t *testing.T, c judging.RoundCommand) {
	t.Helper()
	r := f.runtime
	current, err := f.admin.deps.Judging.Round(t.Context(), r.owner.ID, r.eventID, f.judgeRun)
	resultsSourceRequire(t, err)
	c.RequestID = uuid.New()
	c.Reason = "Actual independent result-source decision"
	_, err = f.admin.deps.Judging.ChangeRound(
		t.Context(),
		r.owner.ID,
		r.eventID,
		f.judgeRun,
		current.Revision,
		c,
	)
	resultsSourceRequire(t, err)
}

func (f *resultsSourceFixture) card(t *testing.T, index, value int, action string) judging.Receipt {
	t.Helper()
	r := f.runtime
	actor := f.judges[index]
	brief, err := f.admin.deps.Judging.Brief(t.Context(), actor.ID, r.eventID, f.judgeRun, r.teamID)
	resultsSourceRequire(t, err)
	version := int64(0)
	if brief.OwnCard != nil {
		version = brief.OwnCard.Version
	}
	scores := []judging.Score{}
	for _, category := range brief.Criteria.Categories {
		v := value
		scores = append(
			scores,
			judging.Score{
				CategoryID: category.ID,
				Value:      &v,
				Evidence: []judging.Evidence{
					{DeliverableID: &f.deliverable, Note: "PRIVATE-INDEPENDENT-JUDGE-PROSE"},
				},
			},
		)
	}
	ack, err := f.admin.deps.Judging.ChangeCard(
		t.Context(),
		actor.ID,
		r.eventID,
		f.judgeRun,
		r.teamID,
		version,
		judging.CardCommand{
			RequestID:            uuid.New(),
			Action:               action,
			SubmissionRevision:   brief.SubmissionRevision,
			RepositoryDecisionID: brief.Repository.DecisionID,
			Scores:               scores,
			Reason:               "Actual independently reviewed judge result evidence",
		},
	)
	resultsSourceRequire(t, err)
	return ack
}

func (f *resultsSourceFixture) complete(t *testing.T) {
	t.Helper()
	f.card(t, 0, 6, "submit")
	f.card(t, 1, 8, "submit")
	f.round(t, judging.RoundCommand{Action: "open_deliberation"})
}

func (f *resultsSourceFixture) prepareMentors(t *testing.T, s rehearsalSurface, run uuid.UUID) {
	t.Helper()
	r := f.runtime
	p, err := s.deps.Events.Read(t.Context(), r.owner.ID, r.eventID)
	resultsSourceRequire(t, err)
	f.secondMentor = testdb.Person(t, r.pool)
	resultsSourceRequire(
		t,
		dbgen.New(r.pool).
			SetMembership(t.Context(), dbgen.SetMembershipParams{EventID: r.eventID, PersonID: f.secondMentor.ID, Role: "mentor", Active: true}),
	)
	availability, err := json.Marshal(
		[]map[string]time.Time{
			{"start": p.Configuration.LiveStart, "end": p.Configuration.LiveEnd},
		},
	)
	resultsSourceRequire(t, err)
	for _, actor := range []dbgen.Person{r.mentor, f.secondMentor} {
		_, err = r.pool.Exec(
			t.Context(),
			"INSERT INTO mentor_profiles(event_id,person_id,skills,availability)VALUES($1,$2,'{}',$3)",
			r.eventID,
			actor.ID,
			availability,
		)
		resultsSourceRequire(t, err)
	}
	_, err = r.pool.Exec(
		t.Context(),
		"INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at)VALUES($1,$2,$3,'second','published',$4,$5)",
		r.eventID,
		r.teamID,
		f.secondMentor.ID,
		p.Configuration.LiveStart,
		p.Configuration.LiveEnd,
	)
	resultsSourceRequire(t, err)
	// Native wall-time conversion includes the actual DST fold choice.
	opens, choice := testdb.WallTimeForInstant(
		t,
		p.Configuration.SubmissionCutoff,
		p.Configuration.Timezone,
	)
	closes, closeChoice := testdb.WallTimeForInstant(
		t,
		p.Configuration.LiveEnd.Add(time.Hour),
		p.Configuration.Timezone,
	)
	if choice != "" && closeChoice != "" && choice != closeChoice {
		t.Fatal("fixture checkpoint crosses incompatible DST choices")
	}
	if choice == "" {
		choice = closeChoice
	}
	f.checkpoint, err = s.deps.Scorecards.Configure(
		t.Context(),
		r.owner.ID,
		r.eventID,
		p.Revision,
		scorecards.CheckpointCommand{
			Reason: "Actual independent frozen mentor context",
			Definition: scorecards.CheckpointDefinition{
				ID:             uuid.New(),
				Name:           "Native final mentor review",
				RunID:          run,
				OpensWallTime:  opens,
				ClosesWallTime: closes,
				DSTChoice:      choice,
			},
		},
	)
	resultsSourceRequire(t, err)
}

func (f *resultsSourceFixture) signQualification(t *testing.T, owner *qualification.Service) {
	t.Helper()
	r := f.runtime
	scores := []scorecards.CategoryScore{}
	for _, category := range f.checkpoint.Criteria.Categories {
		v := 3
		scores = append(
			scores,
			scorecards.CategoryScore{
				CategoryID: category.ID,
				Value:      &v,
				Evidence: []scorecards.EvidenceReference{
					{Kind: "note", Note: resultsMentorPrivateNote},
				},
			},
		)
	}
	for _, actor := range []dbgen.Person{r.mentor, f.secondMentor} {
		_, err := f.admin.deps.Scorecards.Change(
			t.Context(),
			actor.ID,
			r.eventID,
			r.teamID,
			f.revision(t),
			scorecards.CardCommand{
				Action:            "submit",
				CheckpointID:      f.checkpoint.Definition.ID,
				CheckpointVersion: f.checkpoint.Version,
				Scores:            scores,
			},
		)
		resultsSourceRequire(t, err)
	}
	id := uuid.New()
	h, err := f.admin.deps.Moderation.Change(
		t.Context(),
		r.owner.ID,
		r.eventID,
		r.teamID,
		id,
		f.revision(t),
		moderation.Command{
			Action: "configure",
			Reason: "Actual independent moderation panel",
			Definition: &moderation.Definition{
				ID:                id,
				CheckpointID:      f.checkpoint.Definition.ID,
				CheckpointVersion: f.checkpoint.Version,
				ModeratorID:       r.owner.ID,
				SecondReviewerID:  f.secondMentor.ID,
				ReviewDueAt:       f.checkpoint.ClosesAt.Add(-time.Minute),
			},
		},
	)
	resultsSourceRequire(t, err)
	for _, step := range []struct {
		actor  uuid.UUID
		action string
	}{{f.secondMentor.ID, "complete_second_review"}, {r.owner.ID, "open"}, {r.owner.ID, "sign_off"}} {
		h, err = f.admin.deps.Moderation.Change(
			t.Context(),
			step.actor,
			r.eventID,
			r.teamID,
			id,
			f.revision(t),
			moderation.Command{
				Action:  step.action,
				Version: h.Version,
				Reason:  "Actual native signed mentor-final source",
			},
		)
		resultsSourceRequire(t, err)
	}
	p, err := owner.Read(t.Context(), r.owner.ID, r.eventID, f.mentorRun)
	resultsSourceRequire(t, err)
	change := func(c qualification.Command) {
		t.Helper()
		c.RequestID = uuid.New()
		c.Version = p.Round.Version
		c.Reason = "Explicit actual reviewed qualification source"
		p, err = owner.Change(t.Context(), r.owner.ID, r.eventID, f.mentorRun, p.Revision, c)
		resultsSourceRequire(t, err)
	}
	change(
		qualification.Command{
			Action: "configure_panel",
			Panel: &qualification.Panel{
				MentorIDs:   []uuid.UUID{r.mentor.ID, f.secondMentor.ID},
				OrganiserID: r.owner.ID,
			},
		},
	)
	if len(p.Teams) != 1 || !p.Teams[0].CanSaveCard {
		t.Fatal("native signed source not ready", p)
	}
	change(
		qualification.Command{
			Action:        "save_card",
			FreezeBinding: p.Teams[0].FreezeBinding,
			Card:          &qualification.CardInput{TeamID: r.teamID, Scores: scores},
		},
	)
	change(
		qualification.Command{
			Action:        "sign_card",
			FreezeBinding: p.Teams[0].FreezeBinding,
			TeamID:        r.teamID,
			CardVersion:   p.Teams[0].CardVersion,
		},
	)
	change(qualification.Command{Action: "propose"})
	if p.Round.Proposal == nil || len(p.Round.Proposal.Blockers) != 0 ||
		len(p.Round.Proposal.FinalistIDs) != 1 ||
		p.Round.Proposal.FinalistIDs[0] != r.teamID ||
		len(p.Round.Proposal.Ties) != 0 {
		t.Fatal("actual single-finalist proposal absent", p)
	}
	change(qualification.Command{Action: "sign_off"})
	if !p.Current || p.Round.Signoff == nil {
		t.Fatal("actual selection not current and signed", p)
	}
}
