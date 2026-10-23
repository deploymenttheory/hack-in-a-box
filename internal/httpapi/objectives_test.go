package httpapi

import (
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/objectives"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestObjectivesHTTPPrivateConfigurationVersionsAndRejectedResultClaims(t *testing.T) {
	pool := testdb.New(t, true)
	f := newEmbargoFixture(t, pool, time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC))
	if err := database.Migrate(t.Context(), pool, 0); err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile("../objectives/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(
		t.Context(),
		strings.Split(string(schema), "-- +goose Down")[0],
	); err != nil {
		t.Fatal(err)
	}
	native := objectives.Service{Pool: pool, Clock: func() time.Time { return f.now }}
	f.router = NewRouter(
		f.cfg,
		slog.New(slog.NewJSONHandler(&f.logs, nil)),
		Dependencies{Pool: pool, Auth: f.auth, Objectives: native},
	)
	id, measure := uuid.New(), uuid.New()
	path := f.base() + "/objectives/" + id.String()
	target := "0"
	c := objectives.Command{
		Action: "save",
		Reason: "Explicit target configuration",
		Definition: &objectives.Definition{
			Description:    "PRIVATE EARLY THEME OBJECTIVE",
			ThemeSensitive: true,
			Measures: []objectives.Measure{
				{ID: measure, Description: "Prototype count", Target: &target},
			},
		},
	}
	member := f.members[0][0]
	f.expect(f.request(dbgen.Person{}, "GET", path, nil, 0), 401)
	f.expect(f.request(member, "PUT", path, c, 0), 403)
	saved := f.request(f.owner, "PUT", path, c, 0)
	f.expect(saved, 200)
	if !strings.Contains(saved.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("planning response cacheable")
	}
	var view objectives.View
	if err = json.Unmarshal(saved.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Reference.Version != 1 || view.Definition.Measures[0].Baseline != nil ||
		*view.Definition.Measures[0].Target != "0" {
		t.Fatal("unknown baseline/explicit zero", saved.Body.String())
	}
	for _, suffix := range []string{"", "/history"} {
		denied := f.request(member, "GET", path+suffix, nil, 0)
		f.expect(denied, 403)
		if strings.Contains(denied.Body.String(), "PRIVATE EARLY") {
			t.Fatal("private objective in denial")
		}
	}
	f.expect(f.request(member, "GET", f.base()+"/objectives", nil, 0), 403)
	f.expect(f.request(member, "GET", f.base()+"/objective-owners", nil, 0), 403)
	owners := f.request(f.owner, "GET", f.base()+"/objective-owners", nil, 0)
	f.expect(owners, 200)
	if strings.Contains(owners.Body.String(), member.ID.String()) {
		t.Fatal("owner selection exposed participant directory")
	}
	f.expect(f.request(f.owner, "PUT", path, c, 0), 409)
	invalid := map[string]any{
		"action": "save",
		"reason": "Falsely claim achieved output",
		"definition": map[string]any{
			"description":     "Bad achieved result",
			"owner_id":        nil,
			"theme_sensitive": false,
			"measures":        []any{},
			"achieved_value":  0,
		},
	}
	f.expect(f.request(f.owner, "PUT", path, invalid, 1), 400)
	c.Definition.Measures[0].Baseline = &objectives.Baseline{Value: "1", Basis: "demonstrated"}
	f.expect(f.request(f.owner, "PUT", path, c, 1), 400)

	publicID, pageID := uuid.New(), uuid.New()
	publicDefinition := objectives.Definition{
		Description: "Reviewed operational objective",
		OwnerID:     &f.owner.ID,
		Measures: []objectives.Measure{
			{
				ID:          measure,
				Description: "Configured review count",
				Target:      &target,
				Baseline:    &objectives.Baseline{Value: "2", Basis: "self_report"},
			},
		},
	}
	publicVersion, err := native.Change(
		t.Context(),
		f.owner.ID,
		f.id,
		publicID,
		0,
		objectives.Command{
			Action:     "save",
			Definition: &publicDefinition,
			Reason:     "Define disclosed operation",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	cs := content.Service{Pool: pool, Clock: func() time.Time { return f.now }}
	page, err := cs.Save(
		t.Context(),
		f.owner.ID,
		f.id,
		pageID,
		0,
		content.Input{
			Kind: "page",
			Document: content.Document{
				Title:    "Reviewed public objective",
				Markdown: "Explicit reviewed disclosure",
				Audience: access.ContentAudience{Confirmed: true},
				Objectives: []objectives.Disclosure{
					{Objective: publicVersion.Reference, MeasureIDs: []uuid.UUID{measure}},
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"review", "approve", "release"} {
		page, err = cs.Action(
			t.Context(),
			f.owner.ID,
			f.id,
			pageID,
			page.Revision,
			action,
			"Explicit disclosure",
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	disclosedPath := f.base() + "/content/" + pageID.String() + "/objectives/" + publicID.String() + "?version=1"
	publicResponse := f.request(member, "GET", disclosedPath, nil, 0)
	f.expect(publicResponse, 200)
	if strings.Contains(publicResponse.Body.String(), f.owner.ID.String()) ||
		strings.Contains(publicResponse.Body.String(), "baseline") {
		t.Fatal("public objective leaked private planning fields", publicResponse.Body.String())
	}
	f.expect(f.request(member, "GET", disclosedPath[:len(disclosedPath)-1]+"2", nil, 0), 404)
	if _, err = cs.Action(
		t.Context(),
		f.owner.ID,
		f.id,
		pageID,
		page.Revision,
		"withdraw",
		"Stop future disclosure",
	); err != nil {
		t.Fatal(err)
	}
	f.expect(f.request(member, "GET", disclosedPath, nil, 0), 404)
	if strings.Contains(f.logs.String(), "PRIVATE EARLY THEME") {
		t.Fatal("private objective leaked to logs")
	}
	if _, err = pool.Exec(
		t.Context(),
		"UPDATE people SET active=false WHERE id=$1",
		f.owner.ID,
	); err != nil {
		t.Fatal(err)
	}
	denied := f.request(f.owner, "GET", path, nil, 0)
	if denied.Code != 401 && denied.Code != 403 {
		t.Fatal("inactive session configuration access", denied.Code)
	}
}
