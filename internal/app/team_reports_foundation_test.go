package app

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/results"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/teamreports"
)

func confirmResultsForTeamReport(t *testing.T, f *resultsSourceFixture) {
	t.Helper()
	f.complete(t)
	r := f.runtime
	p, err := f.service.Read(t.Context(), r.owner.ID, r.eventID)
	resultsSourceRequire(t, err)
	for _, action := range []string{"prepare", "confirm"} {
		p, err = f.service.Change(
			t.Context(),
			r.owner.ID,
			r.eventID,
			p.Revision,
			results.Command{
				RequestID:     uuid.New(),
				Version:       p.Round.Version,
				Action:        action,
				SourceBinding: p.Preview.Binding,
				Reason:        "Actual independent confirmed source for own-team report",
			},
		)
		resultsSourceRequire(t, err)
	}
	if p.Round.Confirmation == nil {
		t.Fatal("actual native result confirmation absent")
	}
}

func TestTeamReportNativeDraftReleaseCorrectionAudienceAndOptionalOwnScores(t *testing.T) {
	for _, mode := range []string{rubric.AllTeams, rubric.MentorQualification} {
		t.Run(mode, func(t *testing.T) {
			f := newResultsSourceFixture(t, mode)
			r := f.runtime
			s := teamreports.Service{Pool: r.pool, Now: r.clock.read, Results: &f.service}
			p, err := s.Read(t.Context(), r.owner.ID, r.eventID, r.teamID)
			resultsSourceRequire(t, err)
			if p.CurrentSource == nil || p.CurrentSource.ScoresAvailable ||
				p.CurrentSource.FinalScores != nil ||
				p.Latest != nil ||
				p.Receipt != nil {
				t.Fatal("unconfirmed results fabricated report source or receipt", p)
			}
			body := teamreports.Body{
				Title:      "Reviewed own-team feedback",
				Summary:    "An explicitly authored report for this team.",
				Highlights: []string{"Demonstrated a coherent workflow"},
				NextSteps:  []string{"Test the next product hypothesis"},
			}
			if _, err = s.Change(
				t.Context(),
				r.owner.ID,
				r.eventID,
				r.teamID,
				0,
				teamreports.Command{
					RequestID:     uuid.New(),
					Action:        "save",
					SourceBinding: strings.Repeat("a", 64),
					Body:          body,
					Reason:        "Attempt before actual independent confirmation",
				},
			); !errors.Is(
				err,
				access.ErrConflict,
			) {
				t.Fatal("report accepted before actual confirmation", err)
			}
			confirmResultsForTeamReport(t, f)
			p, err = s.Read(t.Context(), r.owner.ID, r.eventID, r.teamID)
			resultsSourceRequire(t, err)
			if p.CurrentSource == nil || p.CurrentSource.FinalScores.Total.Numerator != "42" ||
				p.CurrentSource.TeamID != r.teamID {
				t.Fatal("source is not exact independent own-team result", p)
			}
			source := p.CurrentSource.Binding
			save := teamreports.Command{
				RequestID:     uuid.New(),
				Action:        "save",
				SourceBinding: source,
				Body:          body,
				Reason:        "Deliberate own-team prose without scores",
			}
			p, err = s.Change(t.Context(), r.owner.ID, r.eventID, r.teamID, 0, save)
			resultsSourceRequire(t, err)
			if p.Version != 1 || p.Latest.State != "draft" || p.Latest.FinalScores != nil ||
				p.Receipt == nil ||
				p.Latest.AuthorID != r.owner.ID {
				t.Fatal("native draft lost deliberate score choice/author", p)
			}
			own, err := s.ReadReleased(t.Context(), r.competitor.ID, r.eventID, r.teamID)
			resultsSourceRequire(t, err)
			if own != nil {
				t.Fatal("draft leaked to participant", own)
			}
			release := teamreports.Command{
				RequestID:     uuid.New(),
				Action:        "release",
				SourceBinding: source,
				Reason:        "Exact version reviewed and explicitly released to captured team",
			}
			p, err = s.Change(t.Context(), r.owner.ID, r.eventID, r.teamID, 1, release)
			resultsSourceRequire(t, err)
			original := p.Receipt
			if p.Version != 2 || p.Latest.State != "released" ||
				!reflect.DeepEqual(p.Latest.Audience, []uuid.UUID{r.competitor.ID}) ||
				p.Latest.ReleasedAt == nil {
				t.Fatal("release lost exact audience/history", p)
			}
			own, err = s.ReadReleased(t.Context(), r.competitor.ID, r.eventID, r.teamID)
			resultsSourceRequire(t, err)
			if own == nil || own.Version != 2 || own.FinalScores != nil ||
				own.Body.Title != body.Title {
				t.Fatal("report release implicitly included scores", own)
			}
			if _, err = s.Read(
				t.Context(),
				r.competitor.ID,
				r.eventID,
				r.teamID,
			); !errors.Is(
				err,
				access.ErrForbidden,
			) {
				t.Fatal("participant obtained staff draft/history surface", err)
			}
			for _, person := range []uuid.UUID{r.mentor.ID, f.judges[0].ID} {
				if _, err = s.ReadReleased(
					t.Context(),
					person,
					r.eventID,
					r.teamID,
				); !errors.Is(
					err,
					access.ErrForbidden,
				) {
					t.Fatal("nonmember inherited own-team report access", err)
				}
			}
			save.RequestID = uuid.New()
			save.Body.Title = "Explicit corrected team report"
			save.IncludeFinalScores = true
			save.ScoreBinding = p.CurrentSource.FinalScores.Binding
			save.Reason = "Explicit correction now selecting only own final scores"
			p, err = s.Change(t.Context(), r.owner.ID, r.eventID, r.teamID, 2, save)
			resultsSourceRequire(t, err)
			own, err = s.ReadReleased(t.Context(), r.competitor.ID, r.eventID, r.teamID)
			resultsSourceRequire(t, err)
			if own.Version != 2 || own.FinalScores != nil {
				t.Fatal("unreleased correction changed participant projection", own)
			}
			replay, err := s.Change(t.Context(), r.owner.ID, r.eventID, r.teamID, 1, release)
			resultsSourceRequire(t, err)
			if replay.Version != 3 || !reflect.DeepEqual(replay.Receipt, original) {
				t.Fatal("original replay adopted newer report state", replay)
			}
			current, err := s.Read(t.Context(), r.owner.ID, r.eventID, r.teamID)
			resultsSourceRequire(t, err)
			if current.Receipt != nil {
				t.Fatal("current report read claimed original acknowledgement")
			}
			release.RequestID = uuid.New()
			p, err = s.Change(t.Context(), r.owner.ID, r.eventID, r.teamID, 3, release)
			resultsSourceRequire(t, err)
			own, err = s.ReadReleased(t.Context(), r.competitor.ID, r.eventID, r.teamID)
			resultsSourceRequire(t, err)
			if own == nil || own.Version != 4 || own.FinalScores == nil ||
				own.FinalScores.Total.Numerator != "42" ||
				own.FinalScores.Total.Denominator != "1" ||
				own.FinalScores.TeamID != r.teamID {
				t.Fatal("explicit own-team exact scores absent", own)
			}
			raw, err := json.Marshal(own)
			resultsSourceRequire(t, err)
			for _, private := range []string{resultsMentorPrivateNote, "PRIVATE-INDEPENDENT-JUDGE-PROSE", `"rank"`, `"awards"`, `"votes"`, `"evidence"`, `"audience"`} {
				if strings.Contains(string(raw), private) {
					t.Fatal(
						"report exposed private assessment/other recipients",
						private,
						string(raw),
					)
				}
			}
			history, err := s.History(t.Context(), r.owner.ID, r.eventID, r.teamID)
			resultsSourceRequire(t, err)
			if len(history) != 4 || history[0].Version != 4 || history[1].State != "draft" ||
				history[2].IncludeFinalScores ||
				history[2].AuthorID != r.owner.ID ||
				len(history[2].Audience) != 1 {
				t.Fatal("correction rewrote publication history", history)
			}
			snapshots := func() string {
				t.Helper()
				var raw string
				resultsSourceRequire(
					t,
					r.pool.QueryRow(t.Context(), "SELECT jsonb_build_object('versions',(SELECT jsonb_agg(to_jsonb(v) ORDER BY version)FROM team_feedback_versions v WHERE event_id=$1),'commands',(SELECT jsonb_agg(to_jsonb(c)ORDER BY request_id)FROM team_feedback_commands c WHERE event_id=$1))::text", r.eventID).
						Scan(&raw),
				)
				return raw
			}
			stored := snapshots()
			if mode == rubric.AllTeams {
				for _, sql := range []string{"UPDATE team_feedback_versions SET document='{}'", "DELETE FROM team_feedback_versions", "TRUNCATE team_feedback_versions", "UPDATE team_feedback_commands SET receipt='{}'", "DELETE FROM team_feedback_commands", "TRUNCATE team_feedback_commands"} {
					if _, err = r.pool.Exec(t.Context(), sql); err == nil {
						t.Fatal("retained report/receipt mutation accepted", sql)
					}
				}
				db := stdlib.OpenDBFromPool(r.pool)
				defer func() { _ = db.Close() }()
				provider, e := goose.NewProvider(
					goose.DialectPostgres,
					db,
					os.DirFS("../database/migrations"),
				)
				resultsSourceRequire(t, e)
				if _, err = provider.DownTo(t.Context(), 53); err == nil {
					t.Fatal("populated54 rollback erased report history")
				}
				if snapshots() != stored {
					t.Fatal("schema guard changed original report bytes")
				}
			}
			announced, err := f.service.Released(t.Context(), r.competitor.ID, r.eventID)
			resultsSourceRequire(t, err)
			if len(announced) != 0 {
				t.Fatal("report release implicitly announced outcomes", announced)
			}
			_, err = r.pool.Exec(
				t.Context(),
				"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
				r.eventID,
				r.competitor.ID,
			)
			resultsSourceRequire(t, err)
			if _, err = s.ReadReleased(
				t.Context(),
				r.competitor.ID,
				r.eventID,
				r.teamID,
			); !errors.Is(
				err,
				access.ErrForbidden,
			) {
				t.Fatal("withdrawn competitor retained report audience", err)
			}
			_, err = r.pool.Exec(
				t.Context(),
				"UPDATE competition_claims SET state='confirmed' WHERE event_id=$1 AND person_id=$2",
				r.eventID,
				r.competitor.ID,
			)
			resultsSourceRequire(t, err)
			own, err = s.ReadReleased(t.Context(), r.competitor.ID, r.eventID, r.teamID)
			resultsSourceRequire(t, err)
			if own.Version != 4 || snapshots() != stored {
				t.Fatal("restored current audience rewrote originals")
			}
			f.reopen(t, 0)
			p, err = s.Read(t.Context(), r.owner.ID, r.eventID, r.teamID)
			resultsSourceRequire(t, err)
			if p.CurrentSource == nil || p.CurrentSource.ScoresAvailable ||
				p.CurrentSource.FinalScores != nil ||
				p.Latest.FinalScores != nil {
				t.Fatal("stale result source refilled report score preview", p)
			}
			if _, err = s.ReadReleased(
				t.Context(),
				r.competitor.ID,
				r.eventID,
				r.teamID,
			); err == nil {
				t.Fatal("reopened assessment remained a current released score report")
			}
			if snapshots() != stored {
				t.Fatal("source invalidation rewrote report/ACK history")
			}
		})
	}
}
