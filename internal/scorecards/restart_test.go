package scorecards

import (
	"errors"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

func TestPublishedCriteriaRestartPreservesHistoricalCardsAndRequiresFreshCheckpointBinding(
	t *testing.T,
) {
	f := setup(t)
	ctx := t.Context()
	cmd := f.command(f.checkpoint, "submit", true)
	old, err := f.s.Change(ctx, f.primary.ID, f.event, f.team, f.revision(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	definition := f.criteria.Definition
	definition.Categories[0].Weight = 2
	f.rubricChange(rubric.Command{Action: "save", Definition: &definition})
	f.rubricChange(rubric.Command{Action: "approve", Version: f.criteria.Reference.Version})
	f.rubricChange(
		rubric.Command{
			Action:       "review_impact",
			Version:      f.criteria.Reference.Version,
			ImpactReview: "Restart affected native assessment runs; retain all independent original values.",
			Notice:       "Reviewed common criterion weighting update.",
		},
	)
	item, err := dbgen.New(f.pool).RubricChallenge(ctx, f.event)
	if err != nil {
		t.Fatal(err)
	}
	publisher := content.Service{Pool: f.pool, Clock: f.s.Now}
	current, err := publisher.Read(ctx, f.owner.ID, f.event, item.ID, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	document := current.Document
	document.Rubric = &f.criteria.Reference
	document.ScheduledAt = nil
	revised, err := publisher.Save(
		ctx,
		f.owner.ID,
		f.event,
		item.ID,
		current.Revision,
		content.Input{Kind: "challenge", Document: document},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"review", "approve"} {
		revised, err = publisher.Action(
			ctx,
			f.owner.ID,
			f.event,
			revised.ID,
			revised.Revision,
			action,
			"Synthetic explicit common correction",
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = publisher.Action(
		ctx,
		f.owner.ID,
		f.event,
		revised.ID,
		revised.Revision,
		"release",
		"Do not reinterpret started independent cards",
	); err == nil {
		t.Fatal("changed criteria bypassed native run restart")
	}
	previous := f.checkpoint.Definition.RunID
	f.rubricChange(
		rubric.Command{
			Action:        "restart_run",
			Version:       f.criteria.Reference.Version,
			Purpose:       "checkpoint",
			PreviousRunID: &previous,
		},
	)
	if _, err = publisher.Action(
		ctx,
		f.owner.ID,
		f.event,
		revised.ID,
		revised.Revision,
		"release",
		"Explicit restarted common criteria",
	); err != nil {
		t.Fatal(err)
	}
	cmd.Version = 1
	if _, err = f.s.Change(
		ctx,
		f.primary.ID,
		f.event,
		f.team,
		f.revision(),
		cmd,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("superseded native run accepted new scores", err)
	}
	history, err := f.s.History(ctx, f.primary.ID, f.event, f.team, old.Reference.ID, nil, 0)
	if err != nil || len(history.Items) != 1 || history.Items[0].Rubric != old.Rubric ||
		history.Items[0].Criteria.Categories[0].Weight != 1 {
		t.Fatal("retained author card silently reweighted", err)
	}
	cp := f.checkpoint
	for _, run := range f.criteria.Runs {
		if run.Purpose == "checkpoint" {
			cp.Definition.RunID = run.ID
		}
	}
	cp, err = f.s.Configure(
		ctx,
		f.owner.ID,
		f.event,
		f.revision(),
		CheckpointCommand{
			Definition: cp.Definition,
			Version:    cp.Version,
			Reason:     "Rebind configured checkpoint to explicitly restarted common criteria",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := f.s.Change(
		ctx,
		f.primary.ID,
		f.event,
		f.team,
		f.revision(),
		f.command(cp, "save", false),
	)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Reference.ID == old.Reference.ID || fresh.Rubric != f.criteria.Reference ||
		!fresh.Scores[0].Pending ||
		fresh.Scores[0].Value != nil ||
		fresh.Criteria.Categories[0].Weight != 2 {
		t.Fatal("new version carried historical scores forward")
	}
}
