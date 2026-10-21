// Package forms defines bounded declarative questions and authoritative response validation.
package forms

import (
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

type (
	Condition struct {
		FieldID uuid.UUID `json:"field_id"`
		Equals  string    `json:"equals"`
	}
	Field struct {
		ID          uuid.UUID  `json:"id"`
		Section     string     `json:"section,omitempty"`
		Label       string     `json:"label"`
		Type        string     `json:"type"`
		Required    bool       `json:"required"`
		Help        string     `json:"help"`
		Choices     []string   `json:"choices"`
		VisibleWhen *Condition `json:"visible_when,omitempty"`
	}
)

type (
	Definition struct {
		ID       uuid.UUID `json:"id"`
		Name     string    `json:"name"`
		Audience string    `json:"audience"`
		Fields   []Field   `json:"fields"`
	}
	Issue struct {
		Field   string `json:"field"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	Invalid struct {
		Issues []Issue `json:"issues"`
	}
)

var ErrInvalid = errors.New("form has validation errors")

func (e *Invalid) Error() string { return ErrInvalid.Error() }
func (e *Invalid) Unwrap() error { return ErrInvalid }
func invalid(id uuid.UUID, message string) error {
	return &Invalid{Issues: []Issue{{Field: id.String(), Code: "invalid_form", Message: message}}}
}

func Validate(d Definition) error {
	if d.ID == uuid.Nil || strings.TrimSpace(d.Name) == "" || len(d.Name) > 160 ||
		len(d.Fields) > 100 ||
		!slices.Contains(
			[]string{"attendee", "organiser", "mentor", "judge", "mc", "logistics", "photographer"},
			d.Audience,
		) {
		return invalid(d.ID, "Choose a name, audience and at most 100 questions.")
	}
	previous := map[uuid.UUID]Field{}
	for _, f := range d.Fields {
		if _, ok := previous[f.ID]; ok || f.ID == d.ID || f.ID == uuid.Nil ||
			strings.TrimSpace(f.Label) == "" ||
			len(f.Label) > 160 ||
			len(f.Help) > 2000 ||
			len(f.Section) > 160 ||
			!slices.Contains([]string{"text", "single", "multiple", "checkbox", "rating"}, f.Type) {
			return invalid(
				f.ID,
				"Questions need distinct identities, a label and a supported answer type.",
			)
		}
		if err := validateChoices(f); err != nil {
			return err
		}
		if f.VisibleWhen != nil {
			parent, ok := previous[f.VisibleWhen.FieldID]
			if !ok || !possible(parent, f.VisibleWhen.Equals) {
				return invalid(
					f.ID,
					"A visibility rule must match a possible answer to an earlier question.",
				)
			}
		}
		previous[f.ID] = f
	}
	return nil
}

func validateChoices(f Field) error {
	if len(f.Choices) > 100 ||
		((f.Type == "single" || f.Type == "multiple") && len(f.Choices) == 0) {
		return invalid(f.ID, "Choice questions need between one and 100 distinct options.")
	}
	seen := map[string]bool{}
	for _, choice := range f.Choices {
		if strings.TrimSpace(choice) == "" || len(choice) > 160 || seen[choice] {
			return invalid(f.ID, "Options must be non-empty, distinct and at most 160 characters.")
		}
		seen[choice] = true
	}
	return nil
}

func possible(f Field, value string) bool {
	if len(value) > 160 {
		return false
	}
	switch f.Type {
	case "single", "multiple":
		return slices.Contains(f.Choices, value)
	case "checkbox":
		return value == "true" || value == "false"
	case "rating":
		n, err := strconv.Atoi(value)
		return err == nil && n >= 1 && n <= 10
	default:
		return true
	}
}

type (
	Answers map[string]json.RawMessage
	Result  struct {
		Answers Answers     `json:"answers"`
		Visible []uuid.UUID `json:"visible"`
		Issues  []Issue     `json:"issues"`
	}
)

func matches(raw json.RawMessage, expected string) bool {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	switch v := value.(type) {
	case string:
		return v == expected
	case bool:
		return strconv.FormatBool(v) == expected
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64) == expected
	case []any:
		for _, x := range v {
			if x == expected {
				return true
			}
		}
	}
	return false
}

func validateAnswer(f Field, raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		if f.Required {
			return "Answer this required question."
		}
		return ""
	}
	switch f.Type {
	case "text", "single":
		var v string
		if json.Unmarshal(raw, &v) != nil || len(v) > 4000 {
			return "Enter text of at most 4000 characters."
		}
		if f.Required && strings.TrimSpace(v) == "" {
			return "Answer this required question."
		}
		if f.Type == "single" && v != "" && !slices.Contains(f.Choices, v) {
			return "Choose one of the listed options."
		}
	case "multiple":
		var v []string
		if json.Unmarshal(raw, &v) != nil || len(v) > 100 {
			return "Choose from the listed options."
		}
		if f.Required && len(v) == 0 {
			return "Choose at least one option."
		}
		seen := map[string]bool{}
		for _, x := range v {
			if seen[x] || !slices.Contains(f.Choices, x) {
				return "Choose distinct listed options."
			}
			seen[x] = true
		}
	case "checkbox":
		var v bool
		if json.Unmarshal(raw, &v) != nil {
			return "Choose yes or no."
		}
	case "rating":
		var v int
		if json.Unmarshal(raw, &v) != nil || v < 1 || v > 10 {
			return "Choose a whole-number rating from 1 to 10."
		}
	}
	return ""
}

// Check discards hidden/unknown answers before eligibility or persistence. A
// hidden controlling question cannot activate its descendants through forged data.
func Check(d Definition, input Answers) Result {
	result := Result{Answers: Answers{}, Visible: []uuid.UUID{}, Issues: []Issue{}}
	for _, f := range d.Fields {
		if f.VisibleWhen != nil &&
			!matches(result.Answers[f.VisibleWhen.FieldID.String()], f.VisibleWhen.Equals) {
			continue
		}
		result.Visible = append(result.Visible, f.ID)
		raw := input[f.ID.String()]
		if message := validateAnswer(f, raw); message != "" {
			result.Issues = append(
				result.Issues,
				Issue{Field: f.ID.String(), Code: "invalid_answer", Message: message},
			)
			continue
		}
		if len(raw) > 0 && string(raw) != "null" {
			result.Answers[f.ID.String()] = raw
		}
	}
	return result
}
