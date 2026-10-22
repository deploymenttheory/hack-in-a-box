package communications

import (
	"slices"
	"strings"
	"testing"
)

func TestRenderEscapesLiteralAndVariableTextWithoutRecursiveInterpolation(t *testing.T) {
	values := map[string]string{"person.name": "<script>&[x](javascript:bad) {{team.name}}"}
	for _, format := range []string{"html", "markdown", "text"} {
		output, missing := Render("<img src=x> {{person.name}} / {{team.name}}", format, values)
		if !slices.Equal(missing, []string{"team.name"}) {
			t.Fatalf("missing values: %v", missing)
		}
		if format != "text" &&
			(strings.Contains(output, "<script>") || strings.Contains(output, "<img")) {
			t.Fatalf("active markup in %s: %s", format, output)
		}
		if !strings.Contains(output, "{{team") {
			t.Fatal("variable contents were recursively interpolated")
		}
		if format == "markdown" && strings.Contains(output, "[x](javascript:") {
			t.Fatalf("active link: %s", output)
		}
	}
	for _, invalid := range []string{"{{person.email}}", "{{assessment.text}}", "{{help.question}}", "{{event.name | html}}", "{{event.name"} {
		if _, err := tokens(invalid); err == nil {
			t.Fatalf("accepted forbidden/invalid placeholder: %s", invalid)
		}
	}
}
