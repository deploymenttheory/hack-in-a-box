package certificates

import (
	"bytes"
	"context"
	"html/template"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
)

var printable = template.Must(template.New("certificate").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>{{.Fields.Event}} — Recognition certificate</title>
<style>body{font:18px/1.6 system-ui,sans-serif;color:#111;background:white;margin:2rem}main{max-width:54rem;margin:auto;border:2px solid;padding:2rem}h1{font-size:2rem}dt{font-weight:bold}dd{margin:0 0 1rem;overflow-wrap:anywhere}@page{margin:15mm}@media print{body{margin:0}main{break-inside:avoid}}</style></head><body><main style="border-color:{{.Fields.Accent}}"><h1>{{if eq .Kind "attendance"}}Certificate of attendance{{else}}Award recognition{{end}}</h1><p>Issued by {{.Fields.Issuer}}</p><dl><dt>Recipient</dt><dd>{{.Fields.Name}}</dd><dt>Event</dt><dd>{{.Fields.Event}}</dd><dt>Recognition</dt><dd>{{.Fields.Recognition}}</dd>{{if .Fields.Award}}<dt>Published award</dt><dd>{{.Fields.Award}}</dd>{{end}}<dt>Certificate identifier</dt><dd>{{.ID}}</dd><dt>Version</dt><dd>{{.Version}}</dd><dt>Issued</dt><dd>{{.IssuedAt.Format "2006-01-02"}}</dd></dl><p>This recognises recorded event participation or a published award. It is not a qualification or accreditation.</p></main></body></html>`))

func (s Service) Print(ctx context.Context, actor, eventID, id uuid.UUID) ([]byte, error) {
	v, err := s.Read(ctx, actor, eventID, id)
	if err != nil {
		return nil, failure("print", err)
	}
	if !v.Current {
		return nil, access.ErrConflict
	}
	var b bytes.Buffer
	err = printable.Execute(&b, v.Certificate)
	return b.Bytes(), failure("print", err)
}
