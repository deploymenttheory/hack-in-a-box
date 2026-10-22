package rubric

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestDefaultsProvideBothModesAndSeparatePrivateInstructions(t *testing.T) {
	namespace := uuid.New()
	for _, mode := range []string{AllTeams, MentorQualification} {
		t.Run(mode, func(t *testing.T) {
			d := Default(mode, namespace)
			if err := d.Validate(); err != nil {
				t.Fatal(err)
			}
			if len(d.Categories) != 5 || d.RequiredJudgeCount < 1 {
				t.Fatal("default omitted five criteria or required judge completeness")
			}
			for _, c := range d.Categories {
				if c.MinScore != 1 || c.MaxScore != 10 || c.Weight != 1 || len(c.Anchors) < 2 {
					t.Fatal("default lost equal 1–10 category bounds/anchors")
				}
			}
			if mode == MentorQualification && (d.FinalistCount != 3 || d.ReserveCount != 1) {
				t.Fatal("qualification default lost three finalists and a reserve")
			}
			d.PrivateJudgeGuidance = "SYNTHETIC-PRIVATE-JUDGE-INSTRUCTIONS"
			encoded, err := json.Marshal(d.PublicDefinition)
			if err != nil || strings.Contains(string(encoded), "SYNTHETIC-PRIVATE") ||
				strings.Contains(string(encoded), "private_judge_guidance") {
				t.Fatal("attendee snapshot exposed private guidance")
			}
			digest := d.PublicDigest()
			d.PrivateJudgeGuidance = "REPLACED-PRIVATE-INSTRUCTIONS"
			if d.PublicDigest() != digest {
				t.Fatal("public digest made private instructions discoverable")
			}
			d.Categories[0].Weight = 2
			if d.PublicDigest() == digest {
				t.Fatal("changed public weighting did not invalidate snapshot digest")
			}
			again := Default(mode, namespace)
			if again.Categories[0].ID != d.Categories[0].ID {
				t.Fatal("repeated default replaced category identity")
			}
		})
	}
}

func TestInvalidRubricsCannotIntroduceUnboundedScoresOrBrokenAwardReferences(t *testing.T) {
	for _, change := range []string{"mode", "zero_weight", "reversed_bounds", "anchor_outside_bounds", "duplicate_anchor", "duplicate_category", "missing_judges", "invalid_finalists", "foreign_award"} {
		t.Run(change, func(t *testing.T) {
			d := Default(MentorQualification, uuid.New())
			switch change {
			case "mode":
				d.Mode = "automatic_ai_scores"
			case "zero_weight":
				d.Categories[0].Weight = 0
			case "reversed_bounds":
				d.Categories[0].MaxScore = d.Categories[0].MinScore
			case "anchor_outside_bounds":
				d.Categories[0].Anchors[0].Score = 100
			case "duplicate_anchor":
				d.Categories[0].Anchors = append(
					d.Categories[0].Anchors,
					d.Categories[0].Anchors[0],
				)
			case "duplicate_category":
				d.Categories = append(d.Categories, d.Categories[0])
			case "missing_judges":
				d.RequiredJudgeCount = 0
			case "invalid_finalists":
				d.FinalistCount = 0
			case "foreign_award":
				foreign := uuid.New()
				d.Prizes = []Prize{
					{ID: uuid.New(), Name: "Synthetic category award", CategoryID: &foreign},
				}
			}
			if d.Validate() == nil {
				t.Fatal("invalid judging configuration accepted")
			}
		})
	}
}
