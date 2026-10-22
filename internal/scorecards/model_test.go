package scorecards

import (
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

func TestPendingIsNotZeroAndBoundsComeFromExactRubric(t *testing.T) {
	d := rubric.Default(rubric.MentorQualification, uuid.New())
	d.Categories[0].MinScore = 0
	got, err := normalizeScores(nil, d, false)
	if err != nil || len(got) != 5 || !got[0].Pending || got[0].Value != nil {
		t.Fatal("unobserved category was not pending", err)
	}
	zero := 0
	scored := CategoryScore{
		CategoryID: d.Categories[0].ID,
		Value:      &zero,
		Evidence: []EvidenceReference{
			{Kind: "note", Note: "Demonstrated evidence supports explicit zero"},
		},
	}
	got, err = normalizeScores([]CategoryScore{scored}, d, false)
	if err != nil || got[0].Pending || got[0].Value == nil || *got[0].Value != 0 {
		t.Fatal("explicit in-range zero lost", err)
	}
	if _, err = normalizeScores([]CategoryScore{scored}, d, true); err == nil {
		t.Fatal("missing required categories finalised")
	}
	scored.Pending = true
	if _, err = normalizeScores([]CategoryScore{scored}, d, false); err == nil {
		t.Fatal("pending carried numeric zero")
	}
	scored.Pending = false
	d.Categories[0].MinScore = 1
	if _, err = normalizeScores([]CategoryScore{scored}, d, false); err == nil {
		t.Fatal("zero bypassed configured minimum")
	}
}
