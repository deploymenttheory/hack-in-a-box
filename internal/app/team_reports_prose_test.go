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

func TestTeamReportConcurrentCommandsRetainOneVersionAndExactOriginalReceipt(t *testing.T) {
	f := newResultsSourceFixture(t, rubric.AllTeams)
	r := f.runtime
	ctx := t.Context()
	s := teamreports.Service{Pool: r.pool, Now: r.clock.read, Results: &f.service}
	p, err := s.Read(ctx, r.owner.ID, r.eventID, r.teamID)
	resultsSourceRequire(t, err)
	c := teamreports.Command{
		RequestID:     uuid.New(),
		Action:        "save",
		SourceBinding: p.CurrentSource.Binding,
		Body: teamreports.Body{
			Title:      "Concurrent reviewed report",
			Summary:    "No competing draft may overwrite this exact version",
			Highlights: []string{},
			NextSteps:  []string{},
		},
		Reason: "Explicit reviewed prose command",
	}
	type outcome struct {
		page teamreports.Page
		err  error
	}
	simultaneous := func(expected int64, commands [2]teamreports.Command) [2]outcome {
		start := make(chan struct{})
		responses := make(chan outcome, 2)
		for _, command := range commands {
			go func() {
				<-start
				p, e := s.Change(ctx, r.owner.ID, r.eventID, r.teamID, expected, command)
				responses <- outcome{p, e}
			}()
		}
		close(start)
		return [2]outcome{<-responses, <-responses}
	}
	second := c
	second.RequestID = uuid.New()
	second.Body.Summary = "A different explicitly reviewed competing draft"
	attempts := simultaneous(0, [2]teamreports.Command{c, second})
	accepted, conflicts := 0, 0
	for _, result := range attempts {
		switch {
		case result.err == nil:
			accepted++
		case errors.Is(result.err, access.ErrConflict):
			conflicts++
		default:
			t.Fatal("unexpected concurrency outcome", result.err)
		}
	}
	if accepted != 1 || conflicts != 1 {
		t.Fatal("stale concurrent command overwrote draft", accepted, conflicts)
	}
	c.RequestID = uuid.New()
	duplicates := simultaneous(1, [2]teamreports.Command{c, c})
	for _, result := range duplicates {
		resultsSourceRequire(t, result.err)
		if result.page.Receipt == nil || result.page.Receipt.ExpectedVersion != 1 ||
			result.page.Receipt.Version != 2 ||
			result.page.Receipt.RequestID != c.RequestID {
			t.Fatal("duplicate command lost original acceptance", result.page)
		}
	}
	if *duplicates[0].page.Receipt != *duplicates[1].page.Receipt {
		t.Fatal("duplicate command invented different receipts")
	}
	var versions, commands int
	resultsSourceRequire(
		t,
		r.pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM team_feedback_versions WHERE event_id=$1),(SELECT count(*) FROM team_feedback_commands WHERE event_id=$1)", r.eventID).
			Scan(&versions, &commands),
	)
	if versions != 2 || commands != 2 {
		t.Fatal("duplicate receipt or stale competing write created history", versions, commands)
	}
}
