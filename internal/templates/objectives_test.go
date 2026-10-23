package templates

import (
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/objectives"
)

func TestObjectiveTemplateCopyRemapsConfigurationAndClearsPersonOwnership(t *testing.T) {
	source := fixture()
	owner := uuid.New()
	target := "20"
	source.Definitions.Objectives = []objectives.Configuration{
		{
			ID: uuid.New(),
			Definition: objectives.Definition{
				Description:    "Private objective",
				OwnerID:        &owner,
				ThemeSensitive: true,
				Measures: []objectives.Measure{
					{ID: uuid.New(), Description: "Working examples", Target: &target},
				},
			},
		},
	}
	input := CloneInput{
		ID:        uuid.New(),
		LiveStart: source.Event.LiveStart,
		Timezone:  source.Event.Timezone,
	}
	clone, err := Remap(source, input)
	if err != nil {
		t.Fatal(err)
	}
	original := source.Definitions.Objectives[0]
	copied := clone.Definitions.Objectives[0]
	if copied.ID == original.ID ||
		copied.Definition.Measures[0].ID == original.Definition.Measures[0].ID {
		t.Fatal("objective/measure identity crossed events")
	}
	if copied.Definition.OwnerID != nil || original.Definition.OwnerID == nil {
		t.Fatal("person owner was copied or source mutated")
	}
	if !copied.Definition.ThemeSensitive || *copied.Definition.Measures[0].Target != "20" ||
		copied.Definition.Measures[0].Baseline != nil {
		t.Fatal("planning configuration changed or missing baseline fabricated")
	}
	*copied.Definition.Measures[0].Target = "10"
	if *original.Definition.Measures[0].Target != "20" {
		t.Fatal("template and clone share mutable planning data")
	}
	second, err := Remap(source, input)
	if err != nil {
		t.Fatal(err)
	}
	if second.Definitions.Objectives[0].ID != copied.ID {
		t.Fatal("idempotent clone remap changed")
	}
}

func TestObjectiveTemplateRemapsEventAndObjectiveScopedIdentityWithoutArtificialGlobalConstraint(
	t *testing.T,
) {
	source := fixture()
	sharedMeasure := uuid.New()
	// Native objective IDs are event-scoped, and measure IDs are objective-scoped.
	// Valid source configurations can therefore reuse another resource's UUID.
	source.Definitions.Objectives = []objectives.Configuration{
		{
			ID: source.Event.Schedule[0].ID,
			Definition: objectives.Definition{
				Description: "First",
				Measures: []objectives.Measure{
					{ID: sharedMeasure, Description: "Scoped first measure"},
				},
			},
		},
		{
			ID: uuid.New(),
			Definition: objectives.Definition{
				Description: "Second",
				Measures: []objectives.Measure{
					{ID: sharedMeasure, Description: "Scoped second measure"},
				},
			},
		},
	}
	clone, err := Remap(
		source,
		CloneInput{
			ID:        uuid.New(),
			LiveStart: source.Event.LiveStart,
			Timezone:  source.Event.Timezone,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	a, b := clone.Definitions.Objectives[0], clone.Definitions.Objectives[1]
	if a.ID == clone.Event.Schedule[0].ID ||
		a.Definition.Measures[0].ID == b.Definition.Measures[0].ID {
		t.Fatal("scoped identities collided after clone")
	}
}
