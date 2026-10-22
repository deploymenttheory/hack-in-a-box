package rubric_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestClonedRubricDefaultsPreserveQualificationAndRemainUnapproved(t *testing.T) {
	pool := testdb.New(t, false)
	if err := database.Migrate(t.Context(), pool, 31); err != nil {
		t.Fatal(err)
	}
	owner := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	source := rubric.Default(rubric.MentorQualification, uuid.New())
	source.Name = "Independent qualified rubric"
	categoryID := source.Categories[0].ID
	source.Prizes = []rubric.Prize{{ID: uuid.New(), Name: "Demo award", CategoryID: &categoryID}}
	source.SideQuests = []rubric.SideQuest{
		{
			ID:               uuid.New(),
			Title:            "Optional evidence",
			EvidenceGuidance: "Show supporting evidence without bonus points.",
		},
	}
	template := templates.Rubric{
		ID:                 uuid.New(),
		Name:               source.Name,
		Mode:               source.Mode,
		RequiredJudgeCount: source.RequiredJudgeCount,
		FinalistCount:      source.FinalistCount,
		ReserveCount:       source.ReserveCount,
		Prizes:             source.Prizes,
		SideQuests:         source.SideQuests,
	}
	for _, c := range source.Categories {
		template.Criteria = append(
			template.Criteria,
			templates.Criterion{
				ID:       c.ID,
				Label:    c.Label,
				Guidance: c.Guidance,
				MinScore: c.MinScore,
				MaxScore: c.MaxScore,
				Weight:   c.Weight,
				Anchors:  c.Anchors,
			},
		)
	}
	encoded, _ := json.Marshal(struct {
		Rubrics []templates.Rubric `json:"rubrics"`
	}{[]templates.Rubric{template}})
	q := dbgen.New(pool)
	if err := q.SaveEventDefinitions(
		t.Context(),
		dbgen.SaveEventDefinitionsParams{EventID: eventID, Definitions: encoded},
	); err != nil {
		t.Fatal(err)
	}
	got, err := (rubric.Service{Pool: pool}).Configuration(t.Context(), owner.ID, eventID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Approved || got.Reference.ID != uuid.Nil || got.PublishedVersion != nil ||
		got.Definition.PublicDigest() != source.PublicDigest() ||
		got.Definition.PrivateJudgeGuidance != "" {
		t.Fatal("cloned public rubric changed or acquired implicit approval/private guidance")
	}
	// Merely viewing a default must not persist a version or alter the clone.
	var versions int
	if err = pool.QueryRow(t.Context(), "SELECT count(*) FROM rubric_versions WHERE event_id=$1", eventID).
		Scan(&versions); err != nil ||
		versions != 0 {
		t.Fatal("viewing cloned defaults created an implicit scoring snapshot")
	}
	retained, err := q.EventDefinitions(t.Context(), eventID)
	if err != nil || string(retained.Definitions) != string(encoded) {
		// PostgreSQL jsonb normalizes whitespace; compare semantic values instead.
		var before, after any
		_ = json.Unmarshal(encoded, &before)
		_ = json.Unmarshal(retained.Definitions, &after)
		a, _ := json.Marshal(before)
		b, _ := json.Marshal(after)
		if err != nil || string(a) != string(b) {
			t.Fatal("viewing rubric defaults mutated independent event definitions")
		}
	}
}
