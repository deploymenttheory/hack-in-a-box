package scorecards

import (
	"testing"
	"time"
)

func TestFinalCardSubmissionEvidenceMustIdentifyExactCurrentFrozenOutput(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	final := f.configure("mentor_final")
	f.now = f.config.SubmissionCutoff
	f.freeze()
	f.exec(
		"INSERT INTO submission_revisions(event_id,team_id,revision,actor_id,submitted_at,late,document) VALUES($1,$2,2,$3,$4,false,'{\"definitions\":[],\"evidence\":[]}')",
		f.event,
		f.team,
		f.member.ID,
		f.config.SubmissionCutoff.Add(-30*time.Second),
	)
	f.exec(
		"UPDATE submission_states SET active_revision=2 WHERE event_id=$1 AND team_id=$2",
		f.event,
		f.team,
	)
	command := f.command(final, "submit", true)
	oldVersion := int64(1)
	command.Scores[0].Evidence = []EvidenceReference{
		{Kind: "submission", ID: &f.team, Version: &oldVersion},
	}
	if _, err := f.s.Change(ctx, f.primary.ID, f.event, f.team, f.revision(), command); err == nil {
		t.Fatal("final card assessed retained submission1 while claiming current frozen2")
	}
	frozenVersion := int64(2)
	command.Scores[0].Evidence[0].Version = &frozenVersion
	card, err := f.s.Change(ctx, f.primary.ID, f.event, f.team, f.revision(), command)
	if err != nil {
		t.Fatal("exact frozen output reference denied", err)
	}
	if card.SubmissionRevision == nil || *card.SubmissionRevision != 2 ||
		*card.Scores[0].Evidence[0].Version != 2 {
		t.Fatal("frozen source mismatch")
	}
}
