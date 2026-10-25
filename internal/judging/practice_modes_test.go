package judging

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

func TestPracticeBeforeLiveCannotEnterEitherNativeScoringMode(t *testing.T) {
	for _, mode := range []string{rubric.AllTeams, rubric.MentorQualification} {
		t.Run(mode, func(t *testing.T) {
			f := setupMode(t, mode, false, true)
			practice, err := f.s.Practice(t.Context(), f.first.ID, f.event)
			if err != nil {
				t.Fatal(err)
			}
			if practice.Criteria.Mode != mode || practice.FictionalEntryID == f.team || practice.FictionalEntryID == uuid.Nil {
				t.Fatal("practice did not use the chosen rubric and distinct fictional entry", practice)
			}
			scores := f.scores(10)
			for i := range scores {
				scores[i].Evidence = []Evidence{{Note: "Fictional perfect practice; never a real competition score"}}
			}
			_, err = f.s.ChangePractice(t.Context(), f.first.ID, f.event, 0,
				PracticeCommand{RequestID: uuid.New(), Action: "submit", Scores: scores})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.Brief(t.Context(), f.first.ID, f.event, f.run, f.team); !errors.Is(err, access.ErrForbidden) {
				t.Fatal("practice unlocked real entry before live", err)
			}
			if _, err = f.s.ChangeCard(t.Context(), f.first.ID, f.event, f.run, f.team, 0,
				CardCommand{RequestID: uuid.New(), Action: "submit", SubmissionRevision: 1, Scores: scores}); !errors.Is(err, access.ErrForbidden) {
				t.Fatal("practice scores entered a real pre-live card", err)
			}
			var cards, practiceCards int
			if err = f.pool.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM judge_cards WHERE event_id=$1), (SELECT count(*) FROM judge_practice_cards WHERE event_id=$1)", f.event).Scan(&cards, &practiceCards); err != nil || cards != 0 || practiceCards != 1 {
				t.Fatal("practice did not persist exclusively in its isolated owner", cards, practiceCards, err)
			}
			f.now = f.dates.SubmissionCutoff
			var completion Completion
			err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
				var err error
				completion, err = f.s.CompletionInTransaction(t.Context(), tx, f.owner.ID, f.event, f.run)
				return err
			})
			want := access.ErrConflict // No real submitted judge cards exist.
			if mode == rubric.MentorQualification {
				// This isolated owner fixture intentionally has no finalist-policy
				// adapter. Practice cannot invent the required signed selection.
				// The app golden fixture exercises actual native selection/cards.
				if f.s.Finalists != nil {
					t.Fatal("practice fixture unexpectedly acquired a finalist policy")
				}
				want = access.ErrForbidden
			}
			if !errors.Is(err, want) || completion.Criteria.Mode != mode || len(completion.Cards) != 0 || completion.Binding != "" {
				t.Fatal("fictional practice bypassed the required real final gate", completion, err)
			}
		})
	}
}
