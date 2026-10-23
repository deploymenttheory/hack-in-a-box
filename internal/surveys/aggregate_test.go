package surveys

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
)

func TestAggregateSuppressesSmallCellsComplementsAndUnknownWithoutInventingZero(t *testing.T) {
	id := uuid.New()
	cfg := Configuration{
		ID:      uuid.New(),
		Version: 2,
		Definition: Definition{
			MinimumCohort: 5,
			Form: forms.Definition{
				Fields: []forms.Field{{ID: id, Label: "Self-reported learning", Type: "checkbox"}},
			},
		},
	}
	for _, tc := range []struct {
		name             string
		yes, no, unknown int
		all, question    bool
	}{
		{"empty", 0, 0, 0, true, true},
		{"small whole cohort", 4, 0, 0, true, true},
		{"small true cell", 1, 9, 0, false, true},
		{"small complementary false cell", 9, 1, 0, false, true},
		{"small unknown complement", 5, 5, 1, false, true},
		{"all unknown", 0, 0, 10, false, true},
		{"safe balanced cells", 5, 5, 0, false, false},
		{"safe actual zero", 10, 0, 0, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			responses := []Response{}
			for i := 0; i < tc.yes+tc.no+tc.unknown; i++ {
				answers := forms.Answers{}
				if i < tc.yes {
					answers[id.String()] = json.RawMessage(`true`)
				} else if i < tc.yes+tc.no {
					answers[id.String()] = json.RawMessage(`false`)
				}
				responses = append(responses, Response{Answers: answers})
			}
			out := aggregate(cfg, responses)
			if out.Suppressed != tc.all || out.Questions[0].Suppressed != tc.question {
				t.Fatalf("suppression=%+v", out)
			}
			q := out.Questions[0]
			if tc.question && (q.Answered != nil || q.Unknown != nil || len(q.Cells) != 0) {
				t.Fatal(
					"suppressed question leaked denominator or reconstructed complementary cell",
				)
			}
			if tc.all && out.Respondents != nil {
				t.Fatal("small cohort count exposed")
			}
			if !tc.question &&
				(q.Unknown == nil || *q.Unknown != tc.unknown || q.Cells[1].Count == nil || *q.Cells[1].Count != tc.no) {
				t.Fatal("known zero and unknown conflated")
			}
		})
	}
}

func TestTextNeverAutomaticallyPromotesIntoAggregate(t *testing.T) {
	id := uuid.New()
	cfg := Configuration{
		ID:      uuid.New(),
		Version: 1,
		Definition: Definition{
			MinimumCohort: 5,
			Form: forms.Definition{
				Fields: []forms.Field{{ID: id, Label: "Free text", Type: "text"}},
			},
		},
	}
	responses := []Response{}
	for range 10 {
		responses = append(
			responses,
			Response{Answers: forms.Answers{id.String(): json.RawMessage(`"PRIVATE-RAW-TEXT"`)}},
		)
	}
	out := aggregate(cfg, responses)
	if !out.Questions[0].Suppressed || out.Questions[0].Answered != nil ||
		out.ReviewedSummary != "" {
		t.Fatal("raw prose or identifying text counts automatically released")
	}
}
