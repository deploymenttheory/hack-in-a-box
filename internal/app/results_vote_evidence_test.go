package app

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
)

func resultVoteEvidenceCheck(
	t *testing.T,
	f *resultsSourceFixture,
	evidence []judging.Evidence,
	targetOnly bool,
) error {
	t.Helper()
	r := f.runtime
	return database.Within(t.Context(), r.pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(t.Context(), r.eventID); err != nil {
			return err
		}
		if targetOnly {
			return f.admin.deps.Judging.AuthorizeResultTargetInTransaction(
				t.Context(),
				tx,
				f.judges[0].ID,
				r.eventID,
				f.judgeRun,
				r.teamID,
			)
		}
		return f.admin.deps.Judging.AuthorizeResultEvidenceInTransaction(
			t.Context(),
			tx,
			f.judges[0].ID,
			r.eventID,
			f.judgeRun,
			r.teamID,
			evidence,
		)
	})
}

func TestResultsVoteEvidenceUsesActualVisibleFrozenDeliverablesAndNativeNoteLimits(t *testing.T) {
	f := newResultsSourceFixture(t, rubric.AllTeams)
	r := f.runtime
	private := uuid.New()
	definitions := templates.EmptyDefinitions()
	for _, item := range []struct {
		id       uuid.UUID
		audience []string
	}{{f.deliverable, []string{"organiser", "mentor", "judge"}}, {private, []string{"organiser"}}} {
		definitions.Deliverables = append(
			definitions.Deliverables,
			templates.Deliverable{
				ID:            item.id,
				Title:         "Native evidence audience boundary",
				Required:      true,
				EvidenceKinds: []string{"url"},
				Audience:      item.audience,
			},
		)
	}
	raw, err := json.Marshal(definitions)
	resultsSourceRequire(t, err)
	resultsSourceRequire(
		t,
		dbgen.New(r.pool).
			SaveEventDefinitions(t.Context(), dbgen.SaveEventDefinitionsParams{EventID: r.eventID, Definitions: raw}),
	)
	current, err := f.admin.deps.Submissions.Read(t.Context(), r.owner.ID, r.eventID, r.teamID)
	resultsSourceRequire(t, err)
	_, err = f.admin.deps.Submissions.Submit(
		t.Context(),
		r.owner.ID,
		r.eventID,
		r.teamID,
		current.Revision,
		submissions.Command{
			Reason: "Actual reasoned final with independently restricted artifact",
			Evidence: []submissions.Evidence{
				{
					DeliverableID: f.deliverable,
					Kind:          "url",
					Label:         "Judge-visible frozen build",
					URL:           "https://synthetic.invalid/visible-final",
				},
				{
					DeliverableID: private,
					Kind:          "url",
					Label:         "Organiser-only frozen build",
					URL:           "https://synthetic.invalid/hidden-final",
				},
			},
		},
	)
	resultsSourceRequire(t, err)
	f.complete(t)
	resultsSourceRequire(t, resultVoteEvidenceCheck(t, f, nil, true))
	for _, refs := range [][]judging.Evidence{{{Note: "Independent evidence review"}}, {{DeliverableID: &f.deliverable}}, {{DeliverableID: &f.deliverable, Note: "Observed in this exact current frozen build"}}, {{Note: strings.Repeat("x", 4000)}}} {
		resultsSourceRequire(t, resultVoteEvidenceCheck(t, f, refs, false))
	}
	maximum := make([]judging.Evidence, 30)
	for i := range maximum {
		maximum[i].Note = "Explicit independently reviewed evidence"
	}
	resultsSourceRequire(t, resultVoteEvidenceCheck(t, f, maximum, false))
	unknown := uuid.New()
	for _, c := range []struct {
		name string
		refs []judging.Evidence
	}{{"empty", nil}, {"too_many", append(maximum, judging.Evidence{Note: "31st item"})}, {"blank_note", []judging.Evidence{{Note: " \t\n"}}}, {"oversize_note", []judging.Evidence{{Note: strings.Repeat("x", 4001)}}}, {"unknown_deliverable", []judging.Evidence{{DeliverableID: &unknown}}}, {"hidden_real_deliverable", []judging.Evidence{{DeliverableID: &private, Note: "An attached note does not grant this artifact"}}}} {
		t.Run(c.name, func(t *testing.T) {
			if err := resultVoteEvidenceCheck(
				t,
				f,
				c.refs,
				false,
			); !errors.Is(
				err,
				access.ErrInvalid,
			) {
				t.Fatal("invalid evidence accepted or wrong failure", err)
			}
		})
	}
	// Evidence mistakes never manufacture or revoke the target-only read grant.
	resultsSourceRequire(t, resultVoteEvidenceCheck(t, f, nil, true))
}

func TestResultsVoteEvidenceAndTargetUseCurrentNativeAuthorityAndDeliberation(t *testing.T) {
	for _, mode := range []string{rubric.AllTeams, rubric.MentorQualification} {
		t.Run(mode, func(t *testing.T) {
			f := newResultsSourceFixture(t, mode)
			r := f.runtime
			f.complete(t)
			refs := []judging.Evidence{
				{DeliverableID: &f.deliverable, Note: "Current native frozen evidence"},
			}
			assertAuthority := func(want error) {
				t.Helper()
				for _, target := range []bool{true, false} {
					err := resultVoteEvidenceCheck(t, f, refs, target)
					if want == nil {
						resultsSourceRequire(t, err)
					} else if !errors.Is(err, want) {
						t.Fatal("current target/evidence authority disagreed", target, err, want)
					}
				}
			}
			assertAuthority(nil)
			f.round(
				t,
				judging.RoundCommand{
					Action:   "assign",
					TeamID:   r.teamID,
					JudgeIDs: []uuid.UUID{f.judges[0].ID, f.judges[1].ID},
				},
			)
			assertAuthority(access.ErrConflict)
			f.round(t, judging.RoundCommand{Action: "open_deliberation"})
			assertAuthority(nil)
			f.round(
				t,
				judging.RoundCommand{
					Action:   "assign",
					TeamID:   r.teamID,
					JudgeIDs: []uuid.UUID{f.judges[1].ID},
				},
			)
			assertAuthority(access.ErrForbidden)
			f.round(
				t,
				judging.RoundCommand{
					Action:   "assign",
					TeamID:   r.teamID,
					JudgeIDs: []uuid.UUID{f.judges[0].ID, f.judges[1].ID},
				},
			)
			f.round(t, judging.RoundCommand{Action: "open_deliberation"})
			assertAuthority(nil)
			f.round(
				t,
				judging.RoundCommand{
					Action:       "declare_conflict",
					JudgeID:      f.judges[0].ID,
					CompetitorID: r.competitor.ID,
					ConflictKind: "reporting",
					Enabled:      true,
				},
			)
			assertAuthority(access.ErrForbidden)
			f.round(
				t,
				judging.RoundCommand{
					Action:       "declare_conflict",
					JudgeID:      f.judges[0].ID,
					CompetitorID: r.competitor.ID,
					ConflictKind: "reporting",
					Enabled:      false,
				},
			)
			f.round(t, judging.RoundCommand{Action: "open_deliberation"})
			assertAuthority(nil)
			q := dbgen.New(r.pool)
			resultsSourceRequire(
				t,
				q.SetMembership(
					t.Context(),
					dbgen.SetMembershipParams{
						EventID:  r.eventID,
						PersonID: f.judges[0].ID,
						Role:     "judge",
						Active:   false,
					},
				),
			)
			assertAuthority(access.ErrForbidden)
			resultsSourceRequire(
				t,
				q.SetMembership(
					t.Context(),
					dbgen.SetMembershipParams{
						EventID:  r.eventID,
						PersonID: f.judges[0].ID,
						Role:     "judge",
						Active:   true,
					},
				),
			)
			assertAuthority(nil)
			native, err := f.admin.deps.Submissions.Read(
				t.Context(),
				r.owner.ID,
				r.eventID,
				r.teamID,
			)
			resultsSourceRequire(t, err)
			_, err = f.admin.deps.Submissions.Outcome(
				t.Context(),
				r.owner.ID,
				r.eventID,
				r.teamID,
				native.Revision,
				"withdrawn",
				"Actual withdrawal cannot retain result evidence authority",
			)
			resultsSourceRequire(t, err)
			assertAuthority(access.ErrForbidden)
		})
	}
}
