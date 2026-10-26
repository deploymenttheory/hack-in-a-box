package app

import (
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/results"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/google/uuid"
)

// This native-owner component isolates a boundary exposed by the full HTTP
// lifecycle. Its existing domain fixture is not full lifecycle acceptance.
func TestLifecycleResultPreparationDoesNotStartIndependentJudgeAssessment(t *testing.T) {
	for _, mode := range []string{rubric.AllTeams, rubric.MentorQualification} {
		t.Run(mode, func(t *testing.T) {
			f := newResultsSourceFixture(t, mode)
			r := f.runtime
			assertRun := func(stage string, wantStarted bool) {
				t.Helper()
				var started bool
				var cards int
				resultsSourceRequire(t, r.pool.QueryRow(t.Context(), `
SELECT started_at IS NOT NULL,
 (SELECT count(*) FROM judge_cards WHERE event_id=$1 AND run_id=$2)
FROM rubric_assessment_runs WHERE event_id=$1 AND id=$2`, r.eventID, f.judgeRun).Scan(&started, &cards))
				if started != wantStarted || cards != 0 {
					t.Fatalf("%s changed independent judge assessment: started=%t want=%t cards=%d", stage, started, wantStarted, cards)
				}
			}
			assertRun("native setup before result read", false)
			page, err := f.service.Read(t.Context(), r.owner.ID, r.eventID)
			resultsSourceRequire(t, err)
			assertRun("organiser result preview without judge cards", false)
			if page.Preview.Binding != "" || len(page.Preview.Tallies) != 0 {
				t.Fatal("preview inferred independent judge completion")
			}
			if mode == rubric.MentorQualification {
				if page.Preview.Selection == nil || page.Preview.FinalistsBinding == "" {
					t.Fatal("native qualification selection is absent")
				}
				for _, action := range []string{"prepare", "publish_finalists"} {
					page, err = f.service.Change(t.Context(), r.owner.ID, r.eventID, page.Revision, results.Command{RequestID: uuid.New(), Version: page.Round.Version, Action: action, SourceBinding: page.Preview.FinalistsBinding, Reason: "Explicit native qualification decision before fresh judging"})
					resultsSourceRequire(t, err)
					assertRun(action+" of signed finalists", false)
				}
				if len(page.Announcements) != 1 || page.Announcements[0].Kind != "finalists" {
					t.Fatal("explicit native finalist release is absent")
				}
			}
			// The intended judge's actual frozen brief still starts assessment.
			_, err = f.admin.deps.Judging.Brief(t.Context(), f.judges[0].ID, r.eventID, f.judgeRun, r.teamID)
			resultsSourceRequire(t, err)
			assertRun("intended judge opens actual frozen brief", true)
		})
	}
}
