package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/teamreports"
)

// Reports are deliberately authored feedback, not a publication of results.
// A prose-only release needs no event award confirmation or judged final tally.
func TestTeamReportProseBeforeAnyConfirmationRemainsIndependentOfResults(t *testing.T) {
	for _, mode := range []string{rubric.AllTeams, rubric.MentorQualification} {
		t.Run(mode, func(t *testing.T) {
			f := newResultsSourceFixture(t, mode)
			r := f.runtime
			s := teamreports.Service{Pool: r.pool, Now: r.clock.read, Results: &f.service}
			p, err := s.Read(t.Context(), r.owner.ID, r.eventID, r.teamID)
			resultsSourceRequire(t, err)
			if p.CurrentSource == nil || p.CurrentSource.ScoresAvailable ||
				p.CurrentSource.FinalScores != nil {
				t.Fatal("unconfirmed source invented scores", p)
			}
			c := teamreports.Command{
				RequestID:     uuid.New(),
				Action:        "save",
				SourceBinding: p.CurrentSource.Binding,
				Body: teamreports.Body{
					Title:      "Authored team feedback",
					Summary:    "Independent prose before results approval",
					Highlights: []string{},
					NextSteps:  []string{},
				},
				Reason: "Reviewed prose without scores",
			}
			bad := c
			bad.RequestID = uuid.New()
			bad.IncludeFinalScores = true
			bad.ScoreBinding = strings.Repeat("a", 64)
			if _, err = s.Change(
				t.Context(),
				r.owner.ID,
				r.eventID,
				r.teamID,
				0,
				bad,
			); !errors.Is(
				err,
				access.ErrConflict,
			) {
				t.Fatal("unconfirmed scores accepted", err)
			}
			p, err = s.Change(t.Context(), r.owner.ID, r.eventID, r.teamID, 0, c)
			resultsSourceRequire(t, err)
			if p.Latest.Source.ConfirmationID != nil || p.Latest.Source.JudgeRunID != nil ||
				p.Latest.FinalScores != nil {
				t.Fatal("prose fabricated confirmation source", p)
			}
			release := teamreports.Command{
				RequestID:     uuid.New(),
				Action:        "release",
				SourceBinding: p.CurrentSource.Binding,
				Reason:        "Explicit team-only prose release",
			}
			_, err = s.Change(t.Context(), r.owner.ID, r.eventID, r.teamID, p.Version, release)
			resultsSourceRequire(t, err)
			own, err := s.ReadReleased(t.Context(), r.competitor.ID, r.eventID, r.teamID)
			resultsSourceRequire(t, err)
			if own == nil || own.FinalScores != nil || own.Version != 2 {
				t.Fatal("unconfirmed prose unavailable or invented score", own)
			}
			// A real later award confirmation and completion do not retarget or erase prose.
			confirmResultsForTeamReport(t, f)
			completeReportEvent(t, f)
			own, err = s.ReadReleased(t.Context(), r.competitor.ID, r.eventID, r.teamID)
			resultsSourceRequire(t, err)
			if own == nil || own.Version != 2 || own.FinalScores != nil {
				t.Fatal("result confirmation silently changed prose release", own)
			}
			resultsSourceRequire(
				t,
				dbgen.New(r.pool).
					SetMembership(t.Context(), dbgen.SetMembershipParams{EventID: r.eventID, PersonID: f.judges[0].ID, Role: "judge", Active: false}),
			)
			own, err = s.ReadReleased(t.Context(), r.competitor.ID, r.eventID, r.teamID)
			resultsSourceRequire(t, err)
			if own == nil || own.Version != 2 || own.FinalScores != nil {
				t.Fatal("unrelated judge revocation blocked prose", own)
			}
			resultsSourceRequire(
				t,
				dbgen.New(r.pool).
					SetMembership(t.Context(), dbgen.SetMembershipParams{EventID: r.eventID, PersonID: f.judges[0].ID, Role: "judge", Active: true}),
			)
			// Competition-source invalidation cannot turn an independent prose release
			// into score publication or remove the current member's authored feedback.
			p, err = s.Read(t.Context(), r.owner.ID, r.eventID, r.teamID)
			resultsSourceRequire(t, err)
			if p.CurrentSource == nil || !p.CurrentSource.ScoresAvailable {
				t.Fatal("confirmed source absent", p)
			}
			p, err = s.Change(
				t.Context(),
				r.owner.ID,
				r.eventID,
				r.teamID,
				p.Version,
				teamreports.Command{
					RequestID:     uuid.New(),
					Action:        "save",
					SourceBinding: p.CurrentSource.Binding,
					Body:          c.Body,
					Reason:        "Prose correction after completion",
				},
			)
			resultsSourceRequire(t, err)
			_, err = s.Change(
				t.Context(),
				r.owner.ID,
				r.eventID,
				r.teamID,
				p.Version,
				release,
			) // same request different version is not a new command
			if !errors.Is(err, access.ErrConflict) {
				t.Fatal("original release reused with altered command", err)
			}
		})
	}
}
