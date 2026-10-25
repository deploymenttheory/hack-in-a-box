package app

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/deploymenttheory/hack-in-a-box/internal/results"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestResultsNativeSourcesUseIndependentExactMeansAndCurrentOrganiser(t *testing.T) {
	for _, mode := range []string{rubric.AllTeams, rubric.MentorQualification} {
		t.Run(mode, func(t *testing.T) {
			f := newResultsSourceFixture(t, mode)
			r := f.runtime
			assertBlocked := func(want string) {
				t.Helper()
				p, err := f.service.Read(t.Context(), r.owner.ID, r.eventID)
				resultsSourceRequire(t, err)
				if !slices.Contains(p.Preview.Blockers, want) || p.Preview.Binding != "" ||
					len(p.Preview.Tallies) != 0 ||
					p.Current ||
					len(p.Announcements) != 0 {
					t.Fatal("incomplete native source appeared ready", p)
				}
			}
			assertBlocked("current_required_judge_assessment_needed")
			f.card(t, 0, 6, "submit")
			f.card(t, 1, 8, "save")
			assertBlocked("current_required_judge_assessment_needed")
			f.card(t, 1, 8, "submit")
			assertBlocked("judge_deliberation_required")
			f.round(t, judging.RoundCommand{Action: "open_deliberation"})
			p, err := f.service.Read(t.Context(), r.owner.ID, r.eventID)
			resultsSourceRequire(t, err)
			if len(p.Preview.Blockers) != 0 || p.Preview.Binding == "" ||
				len(p.Preview.Tallies) != 1 ||
				p.Preview.JudgeRunID != f.judgeRun ||
				p.Current ||
				p.Round.Version != 0 ||
				len(p.Announcements) != 0 {
				t.Fatal("actual independent source missing or silently confirmed", p)
			}
			tally := p.Preview.Tallies[0]
			if tally.TeamID != r.teamID || tally.RequiredJudges != 2 ||
				len(tally.Categories) != 5 ||
				tally.Total.Numerator != "42" ||
				tally.Total.Denominator != "1" ||
				tally.Total.Display != "42.00" {
				t.Fatal("wrong weighted final judge total", tally)
			}
			for i, category := range tally.Categories {
				if category.CategoryID != p.Preview.Criteria.Categories[i].ID ||
					category.Average.Numerator != "7" ||
					category.Average.Denominator != "1" {
					t.Fatal("wrong exact category mean", category)
				}
			}
			if mode == rubric.MentorQualification {
				selection := p.Preview.Selection
				if selection == nil || selection.RunID != f.mentorRun ||
					selection.SignedBinding == "" ||
					!slices.Equal(selection.FinalistIDs, []uuid.UUID{r.teamID}) ||
					len(selection.ReserveIDs) != 0 ||
					p.Preview.FinalistsBinding == "" {
					t.Fatal("actual signed native qualification not consumed", p)
				}
			} else if p.Preview.Selection != nil || p.Preview.FinalistsBinding != "" {
				t.Fatal("all-team result invented a qualification selection", p)
			}
			raw, err := json.Marshal(p)
			resultsSourceRequire(t, err)
			for _, private := range []string{resultsMentorPrivateNote, "PRIVATE-INDEPENDENT-JUDGE-PROSE", `"scores":`, `"evidence":`, `"private_judge_guidance":`} {
				if strings.Contains(string(raw), private) {
					t.Fatal(
						"private assessment escaped organiser preparation shape",
						private,
						string(raw),
					)
				}
			}
			resultsSourceRequire(
				t,
				database.Within(t.Context(), r.pool, func(tx pgx.Tx, q *dbgen.Queries) error {
					if _, err := q.LockEvent(t.Context(), r.eventID); err != nil {
						return err
					}
					source, tallies, err := results.CalculateInTransaction(
						t.Context(),
						tx,
						&f.admin.deps.Judging,
						r.owner.ID,
						r.eventID,
						f.judgeRun,
					)
					if err != nil {
						return err
					}
					if !source.Deliberation || len(source.Cards) != 2 ||
						source.Reference != p.Preview.Reference ||
						len(tallies) != 1 ||
						tallies[0].Total != tally.Total {
						return errors.New(
							"transactional result source lost exact independent cards or means",
						)
					}
					return nil
				}),
			)
			outsider := testdb.Person(t, r.pool)
			for _, actor := range []dbgen.Person{f.judges[0], r.mentor, r.competitor, outsider} {
				if _, err = f.service.Read(
					t.Context(),
					actor.ID,
					r.eventID,
				); !errors.Is(
					err,
					access.ErrForbidden,
				) {
					t.Fatal("non-organiser read private current result preparation", actor.ID, err)
				}
			}
			organiser := testdb.Person(t, r.pool)
			q := dbgen.New(r.pool)
			resultsSourceRequire(
				t,
				q.SetMembership(
					t.Context(),
					dbgen.SetMembershipParams{
						EventID:  r.eventID,
						PersonID: organiser.ID,
						Role:     "organiser",
						Active:   true,
					},
				),
			)
			_, err = f.service.Read(t.Context(), organiser.ID, r.eventID)
			resultsSourceRequire(t, err)
			resultsSourceRequire(
				t,
				q.SetMembership(
					t.Context(),
					dbgen.SetMembershipParams{
						EventID:  r.eventID,
						PersonID: organiser.ID,
						Role:     "organiser",
						Active:   false,
					},
				),
			)
			if _, err = f.service.Read(
				t.Context(),
				organiser.ID,
				r.eventID,
			); !errors.Is(
				err,
				access.ErrForbidden,
			) {
				t.Fatal("removed organiser retained current preparation", err)
			}
		})
	}
}

func TestResultsNativeReopenAndFinalReplacementFenceOriginalSource(t *testing.T) {
	for _, mode := range []string{rubric.AllTeams, rubric.MentorQualification} {
		t.Run(mode, func(t *testing.T) {
			f := newResultsSourceFixture(t, mode)
			r := f.runtime
			f.complete(t)
			before, err := f.service.Read(t.Context(), r.owner.ID, r.eventID)
			resultsSourceRequire(t, err)
			approval := uuid.New()
			var approvalBinding string
			resultsSourceRequire(
				t,
				database.Within(t.Context(), r.pool, func(tx pgx.Tx, q *dbgen.Queries) error {
					if _, err := q.LockEvent(t.Context(), r.eventID); err != nil {
						return err
					}
					completion, _, err := results.CalculateInTransaction(
						t.Context(),
						tx,
						&f.admin.deps.Judging,
						r.owner.ID,
						r.eventID,
						f.judgeRun,
					)
					if err != nil {
						return err
					}
					approvalBinding = completion.Binding
					return f.admin.deps.Judging.RecordResultApprovalInTransaction(
						t.Context(),
						tx,
						r.owner.ID,
						r.eventID,
						f.judgeRun,
						approval,
						completion.Binding,
						"Native completion seam only; no award or publication claim",
					)
				}),
			)
			brief, err := f.admin.deps.Judging.Brief(
				t.Context(),
				f.judges[0].ID,
				r.eventID,
				f.judgeRun,
				r.teamID,
			)
			resultsSourceRequire(t, err)
			_, err = f.admin.deps.Judging.ChangeCard(
				t.Context(),
				r.owner.ID,
				r.eventID,
				f.judgeRun,
				r.teamID,
				brief.OwnCard.Version,
				judging.CardCommand{
					RequestID:            uuid.New(),
					Action:               "reopen",
					AuthorID:             f.judges[0].ID,
					SubmissionRevision:   brief.SubmissionRevision,
					RepositoryDecisionID: brief.Repository.DecisionID,
					Reason:               "Reasoned correction invalidates the old result source",
				},
			)
			resultsSourceRequire(t, err)
			reopened, err := f.service.Read(t.Context(), r.owner.ID, r.eventID)
			resultsSourceRequire(t, err)
			if reopened.Preview.Binding != "" || len(reopened.Preview.Tallies) != 0 ||
				len(reopened.Preview.Blockers) == 0 {
				t.Fatal("reopened draft retained a usable result source", reopened)
			}
			var invalidations int
			var retainedBinding string
			resultsSourceRequire(
				t,
				r.pool.QueryRow(t.Context(), "SELECT binding,(SELECT count(*) FROM judge_result_invalidations WHERE approval_id=$1) FROM judge_result_approvals WHERE id=$1", approval).
					Scan(&retainedBinding, &invalidations),
			)
			if invalidations != 1 || retainedBinding != approvalBinding {
				t.Fatal(
					"reasoned reopen did not retain and invalidate the exact native completion approval",
					invalidations,
					retainedBinding,
				)
			}
			f.card(t, 0, 7, "submit")
			f.round(t, judging.RoundCommand{Action: "open_deliberation"})
			current, err := f.service.Read(t.Context(), r.owner.ID, r.eventID)
			resultsSourceRequire(t, err)
			if current.Preview.Binding == "" || current.Preview.Binding == before.Preview.Binding ||
				current.Preview.Tallies[0].Total.Numerator != "45" {
				t.Fatal("fresh independent correction reused old source binding", current)
			}
			for _, category := range current.Preview.Tallies[0].Categories {
				if category.Average.Numerator != "15" || category.Average.Denominator != "2" {
					t.Fatal("recurring source arithmetic lost exact half", category)
				}
			}
			native, err := f.admin.deps.Submissions.Read(
				t.Context(),
				r.owner.ID,
				r.eventID,
				r.teamID,
			)
			resultsSourceRequire(t, err)
			var storedBefore string
			resultsSourceRequire(
				t,
				r.pool.QueryRow(t.Context(), "SELECT jsonb_agg(document ORDER BY author_id)::text FROM judge_cards WHERE event_id=$1 AND run_id=$2", r.eventID, f.judgeRun).
					Scan(&storedBefore),
			)
			replacement, err := f.admin.deps.Submissions.Submit(
				t.Context(),
				r.owner.ID,
				r.eventID,
				r.teamID,
				native.Revision,
				submissions.Command{
					Reason: "Explicit authorised late replacement; old cards cannot silently retarget",
					Evidence: []submissions.Evidence{
						{
							DeliverableID: f.deliverable,
							Kind:          "url",
							Label:         "Changed reviewed final",
							URL:           "https://synthetic.invalid/replaced-results-build",
						},
					},
				},
			)
			resultsSourceRequire(t, err)
			if replacement.Final == nil || replacement.Final.Revision == native.Final.Revision ||
				replacement.Outcome != "frozen" {
				t.Fatal(
					"native authorised replacement did not create a fresh frozen final",
					replacement,
				)
			}
			stale, err := f.service.Read(t.Context(), r.owner.ID, r.eventID)
			resultsSourceRequire(t, err)
			if stale.Preview.Binding != "" || len(stale.Preview.Tallies) != 0 ||
				len(stale.Preview.Blockers) == 0 {
				t.Fatal("old judge or qualification source silently adopted replacement", stale)
			}
			old, err := f.admin.deps.Judging.CardStates(
				t.Context(),
				r.owner.ID,
				r.eventID,
				f.judgeRun,
				r.teamID,
			)
			resultsSourceRequire(t, err)
			if len(old.Items) != 2 {
				t.Fatal("immutable authored cards disappeared on native source replacement", old)
			}
			for _, state := range old.Items {
				if mode == rubric.MentorQualification {
					// The replaced final invalidates the genuine signed qualification.
					// Do not demand hidden current-final metadata after that grant is lost.
					if state.SourceAvailable || state.CanReopen || state.SubmissionRevision != 0 ||
						state.RepositoryDecisionID != nil {
						t.Fatal("stale qualification retained a judge source grant", state)
					}
				} else if !state.SourceChanged || state.SubmissionRevision != replacement.Final.Revision {
					t.Fatal("current stale-card metadata did not identify new source", state)
				}
			}
			if mode == rubric.MentorQualification &&
				!slices.Contains(stale.Preview.Blockers, "current_signed_qualification_required") {
				t.Fatal("native replacement failed to invalidate signed qualification", stale)
			}
			var storedAfter string
			resultsSourceRequire(
				t,
				r.pool.QueryRow(t.Context(), "SELECT jsonb_agg(document ORDER BY author_id)::text FROM judge_cards WHERE event_id=$1 AND run_id=$2", r.eventID, f.judgeRun).
					Scan(&storedAfter),
			)
			if storedAfter != storedBefore {
				t.Fatal("current projection rewrote the original authored judge cards")
			}
		})
	}
}
