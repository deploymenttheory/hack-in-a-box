package preparation

import (
	"encoding/csv"
	"strings"
	"testing"
	"time"
)

func TestHostileCSVAndOfflineHTML(t *testing.T) {
	hostile := []string{
		"=HYPERLINK(\"https://evil.invalid\")",
		" +cmd",
		"\t@SUM(1)",
		"\ufeff-42",
		"\r=1",
		"\n=2",
		"<script src=\"https://evil.invalid\">alert(1)</script>",
	}
	for _, s := range hostile[:6] {
		if got := CSVCell(s); !strings.HasPrefix(got, "'") {
			t.Errorf("unsafe cell %q: %q", s, got)
		}
	}
	r := Report{
		Event:       hostile[0],
		Audience:    "participant",
		Timezone:    "Europe/London",
		GeneratedAt: time.Now().UTC(),
	}
	for _, s := range hostile {
		r.add("task", s, "open", "", s)
	}
	b, err := r.CSV()
	if err != nil {
		t.Fatal(err)
	}
	reader := csv.NewReader(strings.NewReader(string(b)))
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if rows[0][1] != "'"+hostile[0] {
		t.Fatalf("event metadata not neutralised: %q", rows[0])
	}
	b, err = r.HTML()
	if err != nil {
		t.Fatal(err)
	}
	html := string(b)
	for _, unsafe := range []string{"<script", "src=\"https:", "href=\"http", "<link", "<img", "<iframe", "@import", "url("} {
		if strings.Contains(html, unsafe) {
			t.Errorf("offline HTML contains active content %q", unsafe)
		}
	}
	if !strings.Contains(html, "&lt;script") || !strings.Contains(html, "@media print") ||
		!strings.Contains(html, "default-src 'none'") {
		t.Fatal("run sheet lacks escaping, print styles or offline CSP")
	}
}
