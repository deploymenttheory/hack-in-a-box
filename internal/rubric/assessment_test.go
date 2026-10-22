package rubric_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func (f *publicationFixture) entry() (uuid.UUID, uuid.UUID, event.Configuration) {
	f.t.Helper()
	var team uuid.UUID
	if err := f.pool.QueryRow(f.t.Context(), "SELECT team_id FROM hack_team_members WHERE event_id=$1 AND person_id=$2 AND snapshot='published'", f.eventID, f.first.ID).
		Scan(&team); err != nil {
		f.t.Fatal(err)
	}
	entry := uuid.New()
	f.exec(
		"INSERT INTO competition_entries(id,event_id,team_id) VALUES($1,$2,$3)",
		entry,
		f.eventID,
		team,
	)
	for _, resource := range []struct {
		id   uuid.UUID
		kind string
	}{{team, "team"}, {entry, "assessment"}} {
		f.exec(
			"INSERT INTO policy_resources(event_id,id,kind) VALUES($1,$2,$3)",
			f.eventID,
			resource.id,
			resource.kind,
		)
	}
	row, err := dbgen.New(f.pool).Event(f.t.Context(), f.eventID)
	if err != nil {
		f.t.Fatal(err)
	}
	var c event.Configuration
	if err = json.Unmarshal(row.Configuration, &c); err != nil {
		f.t.Fatal(err)
	}
	return team, entry, c
}

func (f *publicationFixture) assessor(role string, resource uuid.UUID) dbgen.Person {
	f.t.Helper()
	p := testdb.Person(f.t, f.pool)
	f.exec(
		"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,$3)",
		f.eventID,
		p.ID,
		role,
	)
	f.exec(
		"INSERT INTO resource_assignments(event_id,resource_id,person_id) VALUES($1,$2,$3)",
		f.eventID,
		resource,
		p.ID,
	)
	return p
}

func (f *publicationFixture) publish() content.Page {
	f.t.Helper()
	ref := f.configuration.Reference
	p := f.challenge(content.Page{}, &ref)
	f.now = f.due
	if err := f.content.Dispatch(f.t.Context(), f.eventID, p.ID, p.Revision); err != nil {
		f.t.Fatal(err)
	}
	return p
}

func (f *publicationFixture) run(purpose string) uuid.UUID {
	f.t.Helper()
	f.change(
		rubric.Command{
			Action:  "create_run",
			Version: f.configuration.Reference.Version,
			Purpose: purpose,
		},
	)
	for _, run := range f.configuration.Runs {
		if run.Purpose == purpose {
			return run.ID
		}
	}
	f.t.Fatal("created assessment run missing")
	return uuid.Nil
}

func (f *publicationFixture) start(actor, team, run uuid.UUID) (rubric.AssessmentSnapshot, error) {
	var out rubric.AssessmentSnapshot
	err := database.Within(f.t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		var err error
		out, err = f.rubric.StartAssessmentInTransaction(
			f.t.Context(),
			tx,
			actor,
			f.eventID,
			team,
			run,
		)
		return err
	})
	return out, err
}

func (f *publicationFixture) freeze(team uuid.UUID, c event.Configuration) {
	f.t.Helper()
	f.exec("INSERT INTO submission_states(event_id,team_id) VALUES($1,$2)", f.eventID, team)
	f.exec(
		"INSERT INTO submission_revisions(event_id,team_id,revision,actor_id,submitted_at,late,document) VALUES($1,$2,1,$3,$4,false,'{\"definitions\":[],\"evidence\":[]}')",
		f.eventID,
		team,
		f.first.ID,
		c.SubmissionCutoff.Add(-time.Minute),
	)
	f.exec(
		"UPDATE submission_states SET active_revision=1,outcome='frozen',frozen_at=$3 WHERE event_id=$1 AND team_id=$2",
		f.eventID,
		team,
		c.SubmissionCutoff,
	)
}

func TestAssessmentConsumerFreezesExactRunOnlyWithActualLiveAuthorityAndAtomicConsumerCommit(
	t *testing.T,
) {
	f := publicationSetup(t)
	team, _, c := f.entry()
	mentor := f.assessor("mentor", team)
	f.publish()
	run := f.run("checkpoint")
	for _, query := range []string{"DELETE FROM rubric_assessment_runs WHERE event_id=$1", "TRUNCATE rubric_assessment_runs CASCADE"} {
		var err error
		if query[0] == 'D' {
			_, err = f.pool.Exec(t.Context(), query, f.eventID)
		} else {
			_, err = f.pool.Exec(t.Context(), query)
		}
		if err == nil {
			t.Fatal("retained assessment run history was removed")
		}
	}
	f.exec("UPDATE events SET stage='live' WHERE id=$1", f.eventID)
	f.now = c.LiveStart.Add(-time.Nanosecond)
	if out, err := f.start(mentor.ID, team, run); err == nil || out.RunID != uuid.Nil {
		t.Fatal("pre-live consumer obtained rubric or started scoring")
	}
	f.now = c.LiveStart
	failedConsumer := errors.New("synthetic score transaction failure")
	err := database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		if _, err := f.rubric.StartAssessmentInTransaction(
			t.Context(),
			tx,
			mentor.ID,
			f.eventID,
			team,
			run,
		); err != nil {
			return err
		}
		return failedConsumer
	})
	if !errors.Is(err, failedConsumer) {
		t.Fatal(err)
	}
	stored, err := dbgen.New(f.pool).
		RubricRun(t.Context(), dbgen.RubricRunParams{EventID: f.eventID, ID: run})
	if err != nil || stored.StartedAt.Valid {
		t.Fatal("failed consumer transaction froze an unsubmitted assessment")
	}
	snapshot, err := f.start(mentor.ID, team, run)
	if err != nil || snapshot.Reference.Version != 1 || snapshot.SubmissionRevision != nil {
		t.Fatal("live provisional consumer lost exact rubric/run or invented final evidence")
	}
	oldReference := snapshot.Reference
	definition := f.configuration.Definition
	definition.Categories[0].Weight = 2
	f.change(rubric.Command{Action: "save", Definition: &definition})
	f.change(rubric.Command{Action: "approve", Version: f.configuration.Reference.Version})
	if _, err = f.pool.Exec(
		t.Context(),
		"UPDATE rubric_assessment_runs SET rubric_version=$3 WHERE event_id=$1 AND id=$2",
		f.eventID,
		run,
		f.configuration.Reference.Version,
	); err == nil {
		t.Fatal("scored run silently rebound to a replacement rubric")
	}
	current, err := f.content.Read(t.Context(), f.owner.ID, f.eventID, func() uuid.UUID {
		var id uuid.UUID
		if err := f.pool.QueryRow(t.Context(), "SELECT id FROM content_items WHERE event_id=$1 AND kind='challenge'", f.eventID).
			Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}(), true, 0)
	if err != nil {
		t.Fatal(err)
	}
	next := f.configuration.Reference
	replacement := f.challenge(current, &next)
	f.change(
		rubric.Command{
			Action:       "review_impact",
			Version:      next.Version,
			ImpactReview: "Explicitly restart the affected provisional assessment.",
			Notice:       "Reviewed category weighting update.",
		},
	)
	if _, err = f.content.Action(
		t.Context(),
		f.owner.ID,
		f.eventID,
		replacement.ID,
		replacement.Revision,
		"release",
		"Synthetic correction",
	); err == nil {
		t.Fatal("published criteria reinterpreted an active started run without restart")
	}
	f.change(
		rubric.Command{
			Action:        "restart_run",
			Version:       next.Version,
			Purpose:       "checkpoint",
			PreviousRunID: &run,
		},
	)
	if _, err = f.content.Action(
		t.Context(),
		f.owner.ID,
		f.eventID,
		replacement.ID,
		replacement.Revision,
		"release",
		"Synthetic correction",
	); err != nil {
		t.Fatal(err)
	}
	if _, err = f.start(mentor.ID, team, run); !errors.Is(err, access.ErrConflict) {
		t.Fatal("superseded run accepted a new assessment")
	}
	stored, err = dbgen.New(f.pool).
		RubricRun(t.Context(), dbgen.RubricRunParams{EventID: f.eventID, ID: run})
	if err != nil || stored.RubricVersion != oldReference.Version || !stored.StartedAt.Valid ||
		stored.Status != "superseded" {
		t.Fatal("explicit restart lost retained original version/freeze")
	}
}

type finalistPolicy struct{ team uuid.UUID }

func (p finalistPolicy) AuthorizeFinalistInTransaction(
	_ context.Context,
	_ pgx.Tx,
	_ uuid.UUID,
	team uuid.UUID,
) error {
	if team != p.team {
		return access.ErrForbidden
	}
	return nil
}

func TestFinalAssessmentConsumerUsesCutoffFrozenEntryRequiredJudgesAndFreshQualificationPolicy(
	t *testing.T,
) {
	for _, mode := range []string{rubric.AllTeams, rubric.MentorQualification} {
		t.Run(mode, func(t *testing.T) {
			f := publicationSetup(t)
			definition := rubric.Default(mode, f.eventID)
			definition.RequiredJudgeCount = 2
			f.change(rubric.Command{Action: "save", Definition: &definition})
			f.change(rubric.Command{Action: "approve", Version: f.configuration.Reference.Version})
			team, entry, c := f.entry()
			judge := f.assessor("judge", entry)
			other := f.assessor("judge", entry)
			f.publish()
			run := f.run("judge_final")
			f.exec("UPDATE events SET stage='live' WHERE id=$1", f.eventID)
			f.now = c.SubmissionCutoff.Add(-time.Nanosecond)
			if out, err := f.start(judge.ID, team, run); err == nil || out.RunID != uuid.Nil {
				t.Fatal("pre-cutoff final assessment disclosed rubric or opened judging")
			}
			f.now = c.SubmissionCutoff
			if _, err := f.start(judge.ID, team, run); err == nil {
				t.Fatal("final assessment accepted an unfrozen entry")
			}
			f.freeze(team, c)
			if mode == rubric.MentorQualification {
				if _, err := f.start(judge.ID, team, run); err == nil {
					t.Fatal("qualification final bypassed current finalist-selection owner")
				}
				f.rubric.Finalists = finalistPolicy{team: team}
			}
			f.exec(
				"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='judge'",
				f.eventID,
				other.ID,
			)
			if _, err := f.start(judge.ID, team, run); err == nil {
				t.Fatal("missing required judge silently opened final assessment")
			}
			f.exec(
				"UPDATE event_memberships SET active=true WHERE event_id=$1 AND person_id=$2 AND role='judge'",
				f.eventID,
				other.ID,
			)
			snapshot, err := f.start(judge.ID, team, run)
			if err != nil || snapshot.SubmissionRevision == nil ||
				*snapshot.SubmissionRevision != 1 ||
				snapshot.Purpose != "judge_final" {
				t.Fatal("post-cutoff frozen entry did not bind fresh final run/evidence")
			}
			f.exec(
				"UPDATE hack_teams SET state='disbanded' WHERE event_id=$1 AND id=$2",
				f.eventID,
				team,
			)
			if out, err := f.start(judge.ID, team, run); err == nil || out.RunID != uuid.Nil {
				t.Fatal("disbanded entry remained scoreable")
			}
		})
	}
}
