package preparation

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"html/template"
	"strings"
	"unicode"
)

// CSVCell neutralises formula prefixes even when hidden behind whitespace or
// control characters. All cells, including event names and metadata, pass here.
func CSVCell(value string) string {
	trimmed := strings.TrimLeftFunc(
		value,
		func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) || r == '\ufeff' },
	)
	if strings.ContainsAny(value, "\t\r\n") ||
		(len(trimmed) > 0 && strings.ContainsRune("=+-@", rune(trimmed[0]))) {
		return "'" + value
	}
	return value
}

func (r Report) CSV() ([]byte, error) {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	w.UseCRLF = true
	rows := make([][]string, 0, 5+len(r.Rows))
	rows = append(rows, [][]string{
		{"Event", r.Event},
		{"Audience", r.Audience},
		{"Generated at (UTC)", r.GeneratedAt.Format("2006-01-02T15:04:05Z")},
		{"Event timezone", r.Timezone},
		{"Kind", "Name", "Status", "Due (event timezone)", "Detail"},
	}...)
	for _, row := range r.Rows {
		rows = append(rows, []string{row.Kind, row.Name, row.Status, row.Due, row.Detail})
	}
	for _, row := range rows {
		for i, v := range row {
			row[i] = CSVCell(v)
		}
		if err := w.Write(row); err != nil {
			return nil, fmt.Errorf("write readiness CSV: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("flush readiness CSV: %w", err)
	}
	return b.Bytes(), nil
}

var runSheet = template.Must(template.New("run-sheet").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'"><title>{{.Event}} — {{.Audience}} run sheet</title><style>
body{font:16px system-ui,sans-serif;color:#172d29;background:white;margin:2rem;line-height:1.5}h1{line-height:1.2} .table-scroll{overflow-x:auto}table{min-width:46rem;width:100%;border-collapse:collapse;overflow-wrap:anywhere;table-layout:fixed}td,th{padding:.5rem;border:1px solid #bbb;text-align:left;vertical-align:top}th{background:#eee}small{color:#444}@media print{table{min-width:0}.table-scroll{overflow:visible}body{margin:0;font-size:10pt}thead{display:table-header-group}tr{break-inside:avoid}h1{break-after:avoid}@page{size:landscape;margin:12mm}}</style></head>
<body><h1>{{.Event}}</h1><p>{{.Audience}} run sheet · Event timezone: <strong>{{.Timezone}}</strong></p><p>Generated at {{.GeneratedAt.Format "2006-01-02 15:04:05 UTC"}}. This is a snapshot; check the live app for subsequent changes.</p><p>Admission, team freeze and challenge publication are separate gates. Formal submissions open at live start. Counts describe the selected audience's permitted view.</p><div class="table-scroll" role="region" aria-label="Run sheet details" tabindex="0"><table><thead><tr><th>Kind</th><th>Name</th><th>Status</th><th>Due (event timezone)</th><th>Detail</th></tr></thead><tbody>{{range .Rows}}<tr><td>{{.Kind}}</td><td>{{.Name}}</td><td>{{.Status}}</td><td>{{.Due}}</td><td>{{.Detail}}</td></tr>{{else}}<tr><td colspan="5">No visible work configured.</td></tr>{{end}}</tbody></table></div><p><small>Keep this downloaded snapshot private. It remains readable and printable offline; no external assets or credentials are included.</small></p></body></html>`))

func (r Report) HTML() ([]byte, error) {
	var b bytes.Buffer
	if err := runSheet.Execute(&b, r); err != nil {
		return nil, fmt.Errorf("render run sheet: %w", err)
	}
	return b.Bytes(), nil
}
