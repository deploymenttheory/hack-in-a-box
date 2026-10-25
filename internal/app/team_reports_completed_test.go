package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/teamreports"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func completeReportEvent(t *testing.T, f *resultsSourceFixture) {
	t.Helper()
	r := f.runtime
	for _, stage := range []string{"assessment", "completed"} {
		event, err := f.admin.deps.Events.Read(t.Context(), r.owner.ID, r.eventID)
		resultsSourceRequire(t, err)
		_, err = f.admin.deps.Events.Transition(
			t.Context(),
			r.owner.ID,
			r.eventID,
			event.Revision,
			stage,
			"Complete actual hack before deliberate report preparation",
			false,
		)
		resultsSourceRequire(t, err)
	}
}

func TestTeamReportsCompletedReadNeverGrantsScoringWrites(t *testing.T) {
	for _, mode := range []string{rubric.AllTeams, rubric.MentorQualification} {
		t.Run(mode, func(t *testing.T) {
			f := newResultsSourceFixture(t, mode)
			r := f.runtime
			confirmResultsForTeamReport(t, f)
			s := teamreports.Service{Pool: r.pool, Now: r.clock.read, Results: &f.service}
			before, err := s.Read(t.Context(), r.owner.ID, r.eventID, r.teamID)
			resultsSourceRequire(t, err)
			completeReportEvent(t, f)
			p, err := s.Read(t.Context(), r.owner.ID, r.eventID, r.teamID)
			resultsSourceRequire(t, err)
			if p.CurrentSource == nil || p.CurrentSource.Binding != before.CurrentSource.Binding ||
				!p.CurrentSource.ScoresAvailable {
				t.Fatal("completed event lost current confirmed report read", p)
			}
			c := teamreports.Command{
				RequestID:          uuid.New(),
				Action:             "save",
				SourceBinding:      p.CurrentSource.Binding,
				IncludeFinalScores: true,
				ScoreBinding:       p.CurrentSource.FinalScores.Binding,
				Body: teamreports.Body{
					Title:      "Post-hack report",
					Summary:    "Deliberately reviewed after the hack",
					Highlights: []string{},
					NextSteps:  []string{},
				},
				Reason: "Post-completion own-team feedback",
			}
			p, err = s.Change(t.Context(), r.owner.ID, r.eventID, r.teamID, 0, c)
			resultsSourceRequire(t, err)
			p, err = s.Change(
				t.Context(),
				r.owner.ID,
				r.eventID,
				r.teamID,
				p.Version,
				teamreports.Command{
					RequestID:     uuid.New(),
					Action:        "release",
					SourceBinding: p.CurrentSource.Binding,
					Reason:        "Explicit post-hack release",
				},
			)
			resultsSourceRequire(t, err)
			own, err := s.ReadReleased(t.Context(), r.competitor.ID, r.eventID, r.teamID)
			resultsSourceRequire(t, err)
			if own == nil || own.FinalScores == nil || own.FinalScores.Total.Numerator != "42" {
				t.Fatal("completed report missing deliberately selected own scores", own)
			}
			_, err = f.admin.deps.Judging.ChangeCard(
				t.Context(),
				f.judges[0].ID,
				r.eventID,
				f.judgeRun,
				r.teamID,
				1,
				judging.CardCommand{
					RequestID: uuid.New(),
					Action:    "save",
					Reason:    "Must remain forbidden after completion",
				},
			)
			if !errors.Is(err, access.ErrForbidden) {
				t.Fatal("read seam granted a new score write", err)
			}
			var stored string
			resultsSourceRequire(
				t,
				r.pool.QueryRow(t.Context(), "SELECT jsonb_agg(document ORDER BY version)::text FROM team_feedback_versions WHERE event_id=$1", r.eventID).
					Scan(&stored),
			)
			// Current judge-role loss invalidates the source even though the publisher and recipient remain current.
			resultsSourceRequire(
				t,
				dbgen.New(r.pool).
					SetMembership(t.Context(), dbgen.SetMembershipParams{EventID: r.eventID, PersonID: f.judges[0].ID, Role: "judge", Active: false}),
			)
			p, err = s.Read(t.Context(), r.owner.ID, r.eventID, r.teamID)
			resultsSourceRequire(t, err)
			if p.CurrentSource == nil || p.CurrentSource.ScoresAvailable ||
				p.CurrentSource.FinalScores != nil ||
				p.Latest.FinalScores != nil {
				t.Fatal("closed-stage read retained revoked judge scores", p)
			}
			if _, err = s.ReadReleased(
				t.Context(),
				r.competitor.ID,
				r.eventID,
				r.teamID,
			); err == nil {
				t.Fatal("closed-stage recipient retained revoked source")
			}
			var after string
			resultsSourceRequire(
				t,
				r.pool.QueryRow(t.Context(), "SELECT jsonb_agg(document ORDER BY version)::text FROM team_feedback_versions WHERE event_id=$1", r.eventID).
					Scan(&after),
			)
			if after != stored {
				t.Fatal("authority change rewrote retained report bytes")
			}

			resultsSourceRequire(
				t,
				dbgen.New(r.pool).
					SetMembership(t.Context(), dbgen.SetMembershipParams{EventID: r.eventID, PersonID: f.judges[0].ID, Role: "judge", Active: true}),
			)
			for _, loss := range []struct {
				name, remove, restore string
				args                  []any
			}{
				{"assignment", "DELETE FROM resource_assignments WHERE event_id=$1 AND resource_id=(SELECT id FROM competition_entries WHERE event_id=$1 AND team_id=$2) AND person_id=$3", "INSERT INTO resource_assignments(event_id,resource_id,person_id)SELECT event_id,id,$3 FROM competition_entries WHERE event_id=$1 AND team_id=$2", []any{r.eventID, r.teamID, f.judges[0].ID}},
				{"conflict", "INSERT INTO mentor_conflicts(event_id,mentor_id,competitor_id,kind)VALUES($1,$2,$3,'reporting')", "DELETE FROM mentor_conflicts WHERE event_id=$1 AND mentor_id=$2 AND competitor_id=$3", []any{r.eventID, f.judges[0].ID, r.competitor.ID}},
			} {
				restored, e := s.Read(t.Context(), r.owner.ID, r.eventID, r.teamID)
				resultsSourceRequire(t, e)
				if restored.CurrentSource == nil ||
					restored.CurrentSource.Binding != before.CurrentSource.Binding {
					t.Fatal("fresh restored source unavailable", loss.name, restored)
				}
				_, e = r.pool.Exec(t.Context(), loss.remove, loss.args...)
				resultsSourceRequire(t, e)
				hidden, e := s.Read(t.Context(), r.owner.ID, r.eventID, r.teamID)
				resultsSourceRequire(t, e)
				if hidden.CurrentSource == nil || hidden.CurrentSource.ScoresAvailable ||
					hidden.CurrentSource.FinalScores != nil ||
					hidden.Latest.FinalScores != nil {
					t.Fatal("revoked current source retained scores", loss.name, hidden)
				}
				history, e := s.History(t.Context(), r.owner.ID, r.eventID, r.teamID)
				resultsSourceRequire(t, e)
				for _, record := range history {
					if record.FinalScores != nil {
						t.Fatal("history retained revoked scores", loss.name)
					}
				}
				if _, e = s.ReadReleased(
					t.Context(),
					r.competitor.ID,
					r.eventID,
					r.teamID,
				); e == nil {
					t.Fatal("recipient retained revoked source", loss.name)
				}
				_, e = r.pool.Exec(t.Context(), loss.restore, loss.args...)
				resultsSourceRequire(t, e)
			}
			resultsSourceRequire(
				t,
				r.pool.QueryRow(t.Context(), "SELECT jsonb_agg(document ORDER BY version)::text FROM team_feedback_versions WHERE event_id=$1", r.eventID).
					Scan(&after),
			)
			if after != stored {
				t.Fatal("assignment/conflict projection rewrote originals")
			}
		})
	}
}

func TestTeamReportsUnscoredPublishedTeamHasProseWithoutInventedScores(t *testing.T) {
	second := uuid.New()
	var person dbgen.Person
	f := newResultsSourceFixtureWithSetup(
		t,
		rubric.AllTeams,
		nil,
		func(f *resultsSourceFixture, _ rehearsalSurface) {
			r := f.runtime
			person = testdb.Person(t, r.pool)
			for _, statement := range []struct {
				query string
				args  []any
			}{
				{"INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')", []any{r.eventID, person.ID}},
				{"INSERT INTO hack_teams(event_id,id,name,state,frozen,published_captain_id)VALUES($1,$2,'Unscored published team','published',true,$3)", []any{r.eventID, second, person.ID}},
				{"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot)VALUES($1,$2,$3,'published')", []any{r.eventID, second, person.ID}},
			} {
				_, err := r.pool.Exec(t.Context(), statement.query, statement.args...)
				resultsSourceRequire(t, err)
			}
			resultsSourceRequire(
				t,
				dbgen.New(r.pool).
					CreateCompetitionEntry(t.Context(), dbgen.CreateCompetitionEntryParams{EventID: r.eventID, TeamID: second}),
			)
			resultsSourceRequire(
				t,
				database.Within(t.Context(), r.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
					return event.RecordTeamFacts(t.Context(), tx, r.eventID, 2, 2, 2, nil)
				}),
			)
		},
	)
	confirmResultsForTeamReport(t, f)
	completeReportEvent(t, f)
	r := f.runtime
	s := teamreports.Service{Pool: r.pool, Now: r.clock.read, Results: &f.service}
	p, err := s.Read(t.Context(), r.owner.ID, r.eventID, second)
	resultsSourceRequire(t, err)
	if p.CurrentSource == nil || p.CurrentSource.ScoresAvailable ||
		p.CurrentSource.FinalScores != nil {
		t.Fatal("unscored source absent or invented tally", p)
	}
	c := teamreports.Command{
		RequestID:     uuid.New(),
		Action:        "save",
		SourceBinding: p.CurrentSource.Binding,
		Body: teamreports.Body{
			Title:      "Your participation",
			Summary:    "A deliberate prose report without final scores",
			Highlights: []string{},
			NextSteps:  []string{},
		},
		Reason: "Own-team feedback for an unscored team",
	}
	invalid := c
	invalid.RequestID = uuid.New()
	invalid.IncludeFinalScores = true
	invalid.ScoreBinding = strings.Repeat("a", 64)
	if _, err = s.Change(
		t.Context(),
		r.owner.ID,
		r.eventID,
		second,
		0,
		invalid,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("missing score inclusion accepted", err)
	}
	p, err = s.Change(t.Context(), r.owner.ID, r.eventID, second, 0, c)
	resultsSourceRequire(t, err)
	_, err = s.Change(
		t.Context(),
		r.owner.ID,
		r.eventID,
		second,
		p.Version,
		teamreports.Command{
			RequestID:     uuid.New(),
			Action:        "release",
			SourceBinding: p.CurrentSource.Binding,
			Reason:        "Explicit prose-only release",
		},
	)
	resultsSourceRequire(t, err)
	own, err := s.ReadReleased(t.Context(), person.ID, r.eventID, second)
	resultsSourceRequire(t, err)
	if own == nil || own.FinalScores != nil {
		t.Fatal("unscored report invented own scores", own)
	}
	if _, err = s.ReadReleased(
		t.Context(),
		r.competitor.ID,
		r.eventID,
		second,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("another team read unscored report", err)
	}
}
