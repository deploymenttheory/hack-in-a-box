package surveys

import (
	"encoding/json"
	"strconv"
)

func small(n, k int) bool { return n > 0 && n < k }

// aggregate always uses the same complete closed cohort. There are no caller
// supplied role/team/answer filters, live totals, averages or identifying rows.
// A small cell OR its complement suppresses the entire question, including its
// denominator and unknown count; text never becomes aggregate output.
func aggregate(cfg Configuration, responses []Response) Report {
	n := len(responses)
	out := Report{
		SurveyID:           cfg.ID,
		SurveyVersion:      cfg.Version,
		ResponseVisibility: ResponseVisibility,
		Suppressed:         n < cfg.Definition.MinimumCohort,
		Questions:          []QuestionAggregate{},
	}
	if !out.Suppressed {
		out.Respondents = &n
	}
	for _, field := range cfg.Definition.Form.Fields {
		q := QuestionAggregate{
			QuestionID:      field.ID,
			QuestionVersion: cfg.Version,
			Label:           field.Label,
			Suppressed:      true,
			Cells:           []Cell{},
		}
		for i, m := range cfg.Definition.Measures {
			if m.QuestionID == field.ID {
				q.Measure = &cfg.Definition.Measures[i]
			}
		}
		if out.Suppressed || field.Type == "text" {
			out.Questions = append(out.Questions, q)
			continue
		}
		values := field.Choices
		switch field.Type {
		case "rating":
			values = []string{}
			for i := 1; i <= 10; i++ {
				values = append(values, strconv.Itoa(i))
			}
		case "checkbox":
			values = []string{"true", "false"}
		}
		counts := map[string]int{}
		answered := 0
		for _, response := range responses {
			raw, ok := response.Answers[field.ID.String()]
			if !ok {
				continue
			}
			answered++
			switch field.Type {
			case "single":
				var v string
				_ = json.Unmarshal(raw, &v)
				counts[v]++
			case "multiple":
				var choices []string
				_ = json.Unmarshal(raw, &choices)
				for _, v := range choices {
					counts[v]++
				}
			case "rating":
				var v int
				_ = json.Unmarshal(raw, &v)
				counts[strconv.Itoa(v)]++
			case "checkbox":
				var v bool
				_ = json.Unmarshal(raw, &v)
				counts[strconv.FormatBool(v)]++
			}
		}
		unknown := n - answered
		k := cfg.Definition.MinimumCohort
		suppress := answered < k || small(unknown, k)
		for _, v := range values {
			if small(counts[v], k) || small(answered-counts[v], k) {
				suppress = true
			}
		}
		if !suppress {
			q.Suppressed = false
			q.Answered = &answered
			q.Unknown = &unknown
			for _, v := range values {
				count := counts[v]
				q.Cells = append(q.Cells, Cell{Value: v, Count: &count})
			}
		}
		out.Questions = append(out.Questions, q)
	}
	return out
}
