package moderation

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/objectives"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

// Keep the historical fixture in its native owner package. This exercises real
// scorecards, feedback actions and moderation rather than synthetic prior SQL.
func TestSurvey40PreservesActual39ObjectiveAndModerationHistory(t *testing.T) {
	pool := testdb.New(t, false)
	ctx := t.Context()
	if err := database.Migrate(ctx, pool, 39); err != nil {
		t.Fatal(err)
	}
	f := setupPool(t, pool)
	f.submit(f.checkpoint, f.primary.ID, true)
	f.submit(f.checkpoint, f.second.ID, true)
	native, feedbackID, _ := f.nativeAction()
	s := f.huddles()
	huddle := f.create(s, f.checkpoint)
	objectiveID := uuid.New()
	plans := objectives.Service{Pool: pool, Clock: func() time.Time { return f.now }}
	definition := objectives.Definition{
		Description: "Retained exact measurement",
		Measures: []objectives.Measure{
			{
				ID:          uuid.New(),
				Description: "Retained self report measure",
				Unit:        "respondent",
				Method:      "Survey",
			},
		},
	}
	objective, err := plans.Change(
		ctx,
		f.owner.ID,
		f.event,
		objectiveID,
		0,
		objectives.Command{
			Action:     "save",
			Definition: &definition,
			Reason:     "Preserve exact39 planning history",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	before := priorDataSnapshot(t, pool)
	if err = database.Migrate(ctx, pool, 40); err != nil {
		t.Fatal(err)
	}
	migrator := schemaMigrator(t, pool)
	if version, e := migrator.GetDBVersion(ctx); e != nil || version != 40 {
		t.Fatal("not real39→40", version, e)
	}
	after := priorDataSnapshot(t, pool)
	for table, value := range before {
		if after[table] != value {
			t.Fatal("39→40 changed native retained table", table)
		}
	}
	if _, err = plans.Read(
		ctx,
		f.owner.ID,
		f.event,
		objectiveID,
		objective.Reference.Version,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Read(ctx, f.owner.ID, f.event, f.team, huddle.Definition.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = native.Read(ctx, f.member.ID, f.event, feedbackID); err != nil {
		t.Fatal(err)
	}
	if err = database.Migrate(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	if err = database.CheckSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
}
