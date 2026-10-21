package forms

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestValidationAndHiddenAnswerPruning(t *testing.T) {
	first, second, third := uuid.New(), uuid.New(), uuid.New()
	d := Definition{ID: uuid.New(), Name: "Attendee", Audience: "attendee", Fields: []Field{
		{
			ID:       first,
			Label:    "Choice",
			Type:     "single",
			Required: true,
			Choices:  []string{"yes", "no"},
		},
		{
			ID:          second,
			Label:       "Details",
			Type:        "text",
			Required:    true,
			Choices:     []string{},
			VisibleWhen: &Condition{FieldID: first, Equals: "yes"},
		},
		{
			ID:          third,
			Label:       "More",
			Type:        "text",
			Required:    true,
			Choices:     []string{},
			VisibleWhen: &Condition{FieldID: second, Equals: "forged"},
		},
	}}
	if err := Validate(d); err != nil {
		t.Fatal(err)
	}
	r := Check(
		d,
		Answers{
			first.String():  json.RawMessage(`"no"`),
			second.String(): json.RawMessage(`"forged"`),
			third.String():  json.RawMessage(`"secret"`),
		},
	)
	if len(r.Visible) != 1 || len(r.Answers) != 1 || len(r.Issues) != 0 {
		t.Fatalf("hidden answers retained: %+v", r)
	}
	r = Check(d, Answers{first.String(): json.RawMessage(`"yes"`)})
	if len(r.Issues) != 1 || r.Issues[0].Field != second.String() {
		t.Fatal("visible required question ignored")
	}
	d.Fields[1].VisibleWhen.Equals = "impossible"
	if Validate(d) == nil {
		t.Fatal("unreachable branch accepted")
	}
	d.Fields[1].VisibleWhen = &Condition{FieldID: third, Equals: "later"}
	if Validate(d) == nil {
		t.Fatal("forward/cyclic dependency accepted")
	}
}

func TestTypedAnswers(t *testing.T) {
	for _, tc := range []struct {
		kind, raw string
		valid     bool
	}{
		{"text", `42`, false}, {"text", `"text"`, true}, {"single", `"other"`, false}, {"single", `"a"`, true}, {"multiple", `["a","a"]`, false}, {"multiple", `["a"]`, true}, {"checkbox", `"true"`, false}, {"checkbox", `false`, true}, {"rating", `1.5`, false}, {"rating", `0`, false}, {"rating", `10`, true},
	} {
		t.Run(tc.kind+tc.raw, func(t *testing.T) {
			f := Field{
				ID:       uuid.New(),
				Label:    "Question",
				Type:     tc.kind,
				Required: true,
				Choices:  []string{"a", "b"},
			}
			if (validateAnswer(f, json.RawMessage(tc.raw)) == "") != tc.valid {
				t.Fatal("unexpected validation")
			}
		})
	}
}
