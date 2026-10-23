package moderation

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
)

func TestReopenedNativeActionRemainsReadableButNeedsExplicitFormalReResolution(t *testing.T) {
	f, s, h := finalContext(t, true)
	native, feedbackID, task := f.nativeAction()
	task = f.actionStatus(native, feedbackID, task, "done")
	gap := uuid.New()
	h = f.change(
		s,
		h,
		f.owner.ID,
		Command{
			Action:        "add_gap",
			GapID:         gap,
			ActionID:      task.Action.ID,
			ActionVersion: task.Action.Version,
			Reason:        "Current native operational action needs formal evidence",
		},
	)
	evidence := []scorecards.EvidenceReference{
		{Kind: "note", Note: "Explicit frozen-output evidence review"},
	}
	h = f.change(
		s,
		h,
		f.owner.ID,
		Command{
			Action:        "resolve_gap",
			GapID:         gap,
			ActionVersion: task.Action.Version,
			Evidence:      evidence,
			Reason:        "Explicit formal decision on operationally completed action",
		},
	)
	task = f.actionStatus(native, feedbackID, task, "open")
	current, err := s.Read(t.Context(), f.owner.ID, f.event, f.team, h.Definition.ID)
	if err != nil || current.Version != h.Version ||
		current.Gaps[0].ActionVersion != h.Gaps[0].ActionVersion {
		t.Fatal(
			"legitimate action advancement hid huddle or silently retargeted prior evidence",
			current,
			err,
		)
	}
	if cards, panelErr := s.Panel(
		t.Context(),
		f.second.ID,
		f.event,
		f.team,
		h.Definition.ID,
	); panelErr != nil ||
		len(cards) != 2 {
		t.Fatal("current intended panel cannot inspect evidence to repair gap", panelErr)
	}
	if _, err = s.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		f.team,
		h.Definition.ID,
		f.revision(),
		Command{
			Action:  "sign_off",
			Version: h.Version,
			Reason:  "Reopening must invalidate formal completeness",
		},
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("retained-reader permission bypassed current operational completeness", err)
	}
	task = f.actionStatus(native, feedbackID, task, "done")
	if _, err = s.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		f.team,
		h.Definition.ID,
		f.revision(),
		Command{
			Action:  "sign_off",
			Version: h.Version,
			Reason:  "Done again still requires explicit current-version review",
		},
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("new done status silently reapproved original action binding", err)
	}
	h = f.change(
		s,
		h,
		f.owner.ID,
		Command{
			Action:        "resolve_gap",
			GapID:         gap,
			ActionVersion: task.Action.Version,
			Evidence:      evidence,
			Reason:        "Explicit current-version re-resolution after reopen",
		},
	)
	h = f.change(
		s,
		h,
		f.owner.ID,
		Command{Action: "sign_off", Reason: "All current exact final evidence explicitly reviewed"},
	)
	if h.SignoffID == nil {
		t.Fatal("explicit source repair did not permit canonical final proof")
	}
}
