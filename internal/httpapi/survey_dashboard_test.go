package httpapi

import (
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/surveys"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestSurveyStaffDashboardAndCSVArePrivateCurrentAuditedReads(t *testing.T) {
	pool := testdb.New(t, true)
	f := newEmbargoFixture(t, pool, time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC))
	if err := database.Migrate(t.Context(), pool, 0); err != nil {
		t.Fatal(err)
	}
	f.now = f.c.LiveEnd.Add(time.Hour)
	f.exec("UPDATE events SET stage='completed',completed_at=$2 WHERE id=$1", f.id, f.c.LiveEnd)
	native := surveys.Service{Pool: pool, Now: func() time.Time { return f.now }}
	f.router = NewRouter(
		f.cfg,
		slog.New(slog.NewJSONHandler(&f.logs, nil)),
		Dependencies{Pool: pool, Auth: f.auth, Surveys: &native},
	)
	id, question := uuid.New(), uuid.New()
	path := f.base() + "/surveys/" + id.String()
	definition := surveys.Definition{
		Form: forms.Definition{
			ID:       id,
			Name:     "Restricted response admin",
			Audience: "attendee",
			Fields: []forms.Field{
				{
					ID:       question,
					Label:    "Private explanation",
					Type:     "text",
					Required: true,
					Choices:  []string{},
				},
			},
		},
		OpensAt:       f.now.Add(-time.Minute),
		ClosesAt:      f.now.Add(time.Hour),
		MinimumCohort: 5,
		Measures:      []surveys.QuestionMeasure{},
	}
	cfg, err := native.Change(
		t.Context(),
		f.owner.ID,
		f.id,
		id,
		0,
		surveys.Command{Action: "save", Definition: &definition},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"approve", "publish"} {
		cfg, err = native.Change(
			t.Context(),
			f.owner.ID,
			f.id,
			id,
			cfg.Version,
			surveys.Command{Action: action},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	person := f.members[0][0]
	if _, err = native.Respond(
		t.Context(),
		person.ID,
		f.id,
		id,
		0,
		surveys.ResponseCommand{
			CommandID:   uuid.New(),
			FormVersion: cfg.Version,
			Answers: forms.Answers{
				question.String(): json.RawMessage(`"RESTRICTED-EXPORT-CANARY"`),
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"/dashboard", "/responses/export.csv"} {
		f.expect(f.request(dbgen.Person{}, "GET", path+suffix, nil, 0), 401)
		for _, actor := range []dbgen.Person{person, f.mentor, f.judge, f.stranger} {
			denied := f.request(actor, "GET", path+suffix, nil, 0)
			f.expect(denied, 403)
			if strings.Contains(denied.Body.String(), "RESTRICTED-EXPORT-CANARY") {
				t.Fatal("denial exposed raw response")
			}
		}
	}
	dashboard := f.request(f.owner, "GET", path+"/dashboard", nil, 0)
	f.expect(dashboard, 200)
	if strings.Contains(dashboard.Body.String(), "RESTRICTED-EXPORT-CANARY") ||
		strings.Contains(dashboard.Body.String(), person.DisplayName) ||
		!strings.Contains(dashboard.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("dashboard overfetched authorship or cached private counts")
	}
	download := f.request(f.owner, "GET", path+"/responses/export.csv", nil, 0)
	f.expect(download, 200)
	if !strings.Contains(download.Header().Get("Cache-Control"), "no-store") ||
		!strings.Contains(download.Header().Get("Content-Type"), "text/csv") ||
		!strings.Contains(download.Header().Get("Content-Disposition"), "attachment;") ||
		!strings.Contains(download.Body.String(), "RESTRICTED-EXPORT-CANARY") {
		t.Fatal("restricted CSV contract")
	}
	f.exec("UPDATE events SET writes_allowed=false,stage='archived' WHERE id=$1", f.id)
	readonly := f.request(f.owner, "GET", path+"/dashboard", nil, 0)
	f.expect(readonly, 200)
	if !strings.Contains(readonly.Body.String(), `"can_manage":false`) ||
		!strings.Contains(readonly.Body.String(), `"can_review_report":false`) {
		t.Fatal("archive capability widened")
	}
	f.expect(
		f.request(f.owner, "GET", path+"/responses/export.csv", nil, 0),
		200,
	) // audited read, not a survey mutation
	f.now = f.now.AddDate(0, 0, 100)
	f.expect(f.request(f.owner, "GET", path+"/dashboard", nil, 0), 403)
	f.expect(f.request(f.owner, "GET", path+"/responses/export.csv", nil, 0), 403)
	var audit []byte
	if err = pool.QueryRow(t.Context(), "SELECT coalesce(jsonb_agg(to_jsonb(a)),'[]') FROM audit_events a WHERE event_id=$1", f.id).
		Scan(&audit); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(f.logs.String(), "RESTRICTED-EXPORT-CANARY") ||
		strings.Contains(string(audit), "RESTRICTED-EXPORT-CANARY") {
		t.Fatal("restricted answer leaked into logs/audits")
	}
}
