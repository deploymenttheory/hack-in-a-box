package moderation

import (
	"errors"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/feedback"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/google/uuid"
)

func TestActualFeedbackSourceOrOwnerLossInvalidatesFormalGapCompletion(t *testing.T) {
	for _, loss := range []string{"feedback_withdrawn", "action_owner_withdrawn"} {
		t.Run(loss, func(t *testing.T) {
			f, s, h := finalContext(t, true)
			native, feedbackID, task := f.nativeAction()
			task = f.actionStatus(native, feedbackID, task, "done")
			gapID := uuid.New()
			h = f.change(s, h, f.owner.ID, Command{Action: "add_gap", GapID: gapID, ActionID: task.Action.ID, ActionVersion: task.Action.Version, Reason: "Actual published action linked to formal evidence gap"})
			h = f.change(s, h, f.owner.ID, Command{Action: "resolve_gap", GapID: gapID, ActionVersion: task.Action.Version, Evidence: []scorecards.EvidenceReference{{Kind: "note", Note: "Separate explicit formal frozen-output decision"}}, Reason: "Action done plus independently reviewed formal evidence"})
			if _, err := s.Panel(t.Context(), f.second.ID, f.event, f.team, h.Definition.ID); err != nil {
				t.Fatal("current native action source unexpectedly unreadable", err)
			}
			if loss == "feedback_withdrawn" {
				page, err := native.Read(t.Context(), f.owner.ID, f.event, feedbackID)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = native.Change(t.Context(), f.owner.ID, f.event, feedbackID, page.CurrentVersion, uuid.New(), feedback.Command{Action: "withdraw"}); err != nil {
					t.Fatal(err)
				}
			} else {
				f.exec(`UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2`, f.event, f.member.ID)
			}
			if _, err := s.Change(t.Context(), f.owner.ID, f.event, f.team, h.Definition.ID, f.revision(), Command{Action: "sign_off", Version: h.Version, Reason: "Must not borrow prior action eligibility"}); !errors.Is(err, access.ErrConflict) && !errors.Is(err, access.ErrNotFound) {
				t.Fatal("stale operational support source signed final", err)
			}
			if loss == "feedback_withdrawn" {
				if _, err := s.Read(t.Context(), f.second.ID, f.event, f.team, h.Definition.ID); !errors.Is(err, access.ErrNotFound) && !errors.Is(err, access.ErrConflict) {
					t.Fatal("huddle panel bypassed native withdrawn feedback source", err)
				}
			}
			var proofs int
			if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM mentor_scorecard_signoffs WHERE event_id=$1`, f.event).Scan(&proofs); err != nil || proofs != 0 {
				t.Fatal("source loss left final proof", proofs, err)
			}
		})
	}
}
