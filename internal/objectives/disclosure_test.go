package objectives_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/objectives"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestObjectiveNativeContentDisclosureAudienceEmbargoAndExactVersion(t *testing.T) {
	pool := testdb.New(t, true)

	owner, reader, outsider := testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	)
	eventID := testdb.Event(t, pool, owner.ID)
	if _, err := pool.Exec(
		t.Context(),
		"INSERT INTO event_memberships(event_id,person_id,role,active) VALUES($1,$2,'logistics',true)",
		eventID,
		reader.ID,
	); err != nil {
		t.Fatal(err)
	}
	s := objectives.Service{Pool: pool}
	cs := content.Service{Pool: pool}
	id, measure, page := uuid.New(), uuid.New(), uuid.New()
	target := "10"
	d := objectives.Definition{
		Description:    "Private theme-bearing objective",
		OwnerID:        &owner.ID,
		ThemeSensitive: true,
		Measures: []objectives.Measure{
			{
				ID:          measure,
				Description: "Demonstrated prototypes",
				Target:      &target,
				Baseline:    &objectives.Baseline{Value: "2", Basis: "self_report"},
			},
		},
	}
	v, err := s.Change(
		t.Context(),
		owner.ID,
		eventID,
		id,
		0,
		objectives.Command{Action: "save", Definition: &d, Reason: "Plan objective"},
	)
	if err != nil {
		t.Fatal(err)
	}
	doc := content.Document{
		Title:    "Reviewed objective disclosure",
		Markdown: "Explicitly selected objective and measure version",
		Audience: access.ContentAudience{Roles: []string{"logistics"}},
		Objectives: []objectives.Disclosure{
			{Objective: v.Reference, MeasureIDs: []uuid.UUID{measure}},
		},
	}
	p, err := cs.Save(
		t.Context(),
		owner.ID,
		eventID,
		page,
		0,
		content.Input{Kind: "page", Document: doc},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"review", "approve"} {
		p, err = cs.Action(
			t.Context(),
			owner.ID,
			eventID,
			page,
			p.Revision,
			action,
			"Explicit objective disclosure review",
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = cs.Action(
		t.Context(),
		owner.ID,
		eventID,
		page,
		p.Revision,
		"release",
		"Publish before common release",
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("theme-sensitive objective bypassed common release", err)
	}
	if _, err = s.Public(
		t.Context(),
		reader.ID,
		eventID,
		page,
		v.Reference,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("unreleased objective readable", err)
	}
	// A separate non-theme planning definition can be deliberately disclosed using
	// ordinary approved content before common release; secrecy is not universal.
	d.ThemeSensitive = false
	d.Description = "Improve onboarding logistics"
	v, err = s.Change(
		t.Context(),
		owner.ID,
		eventID,
		id,
		1,
		objectives.Command{
			Action:     "save",
			Definition: &d,
			Reason:     "Independent operational objective",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	doc.Objectives[0].Objective = v.Reference
	p, err = cs.Save(
		t.Context(),
		owner.ID,
		eventID,
		page,
		p.Revision,
		content.Input{Kind: "page", Document: doc},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"review", "approve", "release"} {
		p, err = cs.Action(
			t.Context(),
			owner.ID,
			eventID,
			page,
			p.Revision,
			action,
			"Reviewed operational disclosure",
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	public, err := s.Public(t.Context(), reader.ID, eventID, page, v.Reference)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(public)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), owner.ID.String()) ||
		strings.Contains(string(raw), "baseline") ||
		len(public.Measures) != 1 ||
		public.Measures[0].ID != measure {
		t.Fatalf("private planning fields leaked or wrong selected measure: %s", raw)
	}
	if _, err = s.Public(
		t.Context(),
		reader.ID,
		eventID,
		page,
		objectives.Reference{ID: id, Version: 1},
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("older undisclosed theme version readable", err)
	}
	if _, err = s.Public(
		t.Context(),
		outsider.ID,
		eventID,
		page,
		v.Reference,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("outside source audience", err)
	}
	if _, err = cs.Action(
		t.Context(),
		owner.ID,
		eventID,
		page,
		p.Revision,
		"withdraw",
		"Remove future disclosure access",
	); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Public(
		t.Context(),
		reader.ID,
		eventID,
		page,
		v.Reference,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("withdrawn source remains readable", err)
	}
}
