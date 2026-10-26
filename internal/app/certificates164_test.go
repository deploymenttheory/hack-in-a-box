package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/certificates"
	"github.com/deploymenttheory/hack-in-a-box/internal/results"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

// Both templates use actual frozen final submissions, native judge cards,
// confirmation and explicit award publication. No publication row is fabricated.
func TestCertificatesRequireActualPublishedAwardAndCurrentRecipient(t *testing.T) {
	for _, mode := range []string{rubric.AllTeams, rubric.MentorQualification} {
		t.Run(mode, func(t *testing.T) {
			prize := uuid.New()
			f := newResultsSourceFixtureWithSetup(
				t,
				mode,
				func(d *rubric.Definition) { d.Prizes = []rubric.Prize{{ID: prize, Name: "Published synthetic winner"}} },
				nil,
			)
			r := f.runtime
			ctx := t.Context()
			owner, eventID := r.owner.ID, r.eventID
			s := certificates.Service{Pool: r.pool, Now: r.clock.read}
			template := uuid.New()
			_, err := s.Configure(
				ctx,
				owner,
				eventID,
				certificates.SettingsCommand{
					RequestID: uuid.New(),
					Reviewed:  true,
					Templates: []certificates.Template{
						{
							ID:          template,
							Kind:        "award",
							Recognition: "Explicit published award",
							Issuer:      "Reviewed event organiser",
							Accent:      "#007e67",
						},
					},
					Reason: "Review optional award recognition",
				},
			)
			resultsSourceRequire(t, err)
			target := certificates.Target{
				PersonID:      r.competitor.ID,
				TemplateID:    template,
				PublicationID: uuid.New(),
				PrizeID:       prize,
			}
			before, err := s.Preview(ctx, owner, eventID, target)
			resultsSourceRequire(t, err)
			if before.Eligible {
				t.Fatal("unpublished award qualified")
			}
			f.complete(t)
			p, err := f.service.Read(ctx, owner, eventID)
			resultsSourceRequire(t, err)
			p, err = f.service.Change(
				ctx,
				owner,
				eventID,
				p.Revision,
				results.Command{
					RequestID:     uuid.New(),
					Version:       p.Round.Version,
					Action:        "prepare",
					SourceBinding: p.Preview.Binding,
					Reason:        "Review actual independent judge results",
				},
			)
			resultsSourceRequire(t, err)
			p, err = f.service.Change(
				ctx,
				owner,
				eventID,
				p.Revision,
				results.Command{
					RequestID:     uuid.New(),
					Version:       p.Round.Version,
					Action:        "confirm",
					SourceBinding: p.Preview.Binding,
					Awards: []results.Award{
						{PrizeID: prize, Name: "Published synthetic winner", TeamID: r.teamID},
					},
					Reason: "PRIVATE independent result confirmation",
				},
			)
			resultsSourceRequire(t, err)
			before, err = s.Preview(ctx, owner, eventID, target)
			resultsSourceRequire(t, err)
			if before.Eligible {
				t.Fatal("private confirmation qualified as publication")
			}
			p, err = f.service.Change(
				ctx,
				owner,
				eventID,
				p.Revision,
				results.Command{
					RequestID:     uuid.New(),
					Version:       p.Round.Version,
					Action:        "publish_awards",
					SourceBinding: p.Preview.Binding,
					Reason:        "Explicit native public award release",
				},
			)
			resultsSourceRequire(t, err)
			if p.Receipt == nil || p.Receipt.PublicationID == nil {
				t.Fatal("actual publication receipt absent")
			}
			target.PublicationID = *p.Receipt.PublicationID
			eligible, err := s.Preview(ctx, owner, eventID, target)
			resultsSourceRequire(t, err)
			if !eligible.Eligible || eligible.Fields.Award != "Published synthetic winner" {
				t.Fatal("actual published award not eligible", eligible)
			}
			id := uuid.New()
			v, err := s.Change(
				ctx,
				owner,
				eventID,
				id,
				certificates.Command{
					RequestID: uuid.New(),
					Action:    "issue",
					Target:    target,
					Binding:   eligible.Binding,
					Reason:    "PRIVATE certificate issuance provenance",
				},
			)
			resultsSourceRequire(t, err)
			if !v.Current {
				t.Fatal("issued award is not current")
			}
			body, err := s.Print(ctx, r.competitor.ID, eventID, id)
			resultsSourceRequire(t, err)
			for _, private := range []string{"PRIVATE", resultsMentorPrivateNote, "score", "tallies", "votes"} {
				if strings.Contains(string(body), private) {
					t.Fatal("private assessment leaked to print", private)
				}
			}
			// Private judging changes do not silently unpublish an earned award.
			f.reopen(t, 0)
			if _, err = s.Print(ctx, r.competitor.ID, eventID, id); err != nil {
				t.Fatal("private judging reopen revoked explicit historical award recognition", err)
			}
			if _, err = s.Read(
				ctx,
				f.judges[0].ID,
				eventID,
				id,
			); !errors.Is(
				err,
				access.ErrNotFound,
			) {
				t.Fatal("other person enumerated award certificate", err)
			}
			second := testdb.Event(t, r.pool, owner)
			if _, err = s.Read(ctx, owner, second, id); !errors.Is(err, access.ErrNotFound) {
				t.Fatal("cross-event certificate", err)
			}
		})
	}
}
