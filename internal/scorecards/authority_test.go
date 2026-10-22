package scorecards

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func TestCurrentAuthorityAndRecordedPanelNeverExposeIndependentDrafts(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	cmd := f.command(f.checkpoint, "submit", true)
	card, err := f.s.Change(ctx, f.primary.ID, f.event, f.team, f.revision(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	proof := ModerationContext{
		ID:                id,
		Version:           1,
		Binding:           "synthetic-current-huddle",
		TeamID:            f.team,
		RunID:             card.RunID,
		RequiredAuthorIDs: []uuid.UUID{f.primary.ID},
		Cards:             []CardReference{card.Reference},
	}
	f.exec(
		"CREATE TABLE synthetic_moderation_contexts(event_id uuid,team_id uuid,id uuid,actor_id uuid,active boolean,document jsonb)",
	)
	raw, _ := json.Marshal(proof)
	f.exec(
		"INSERT INTO synthetic_moderation_contexts VALUES($1,$2,$3,$4,true,$5)",
		f.event,
		f.team,
		id,
		f.owner.ID,
		raw,
	)
	f.s.Moderation = recordedOwner{}
	panel, err := f.s.Panel(ctx, f.owner.ID, f.event, f.team, id)
	if err != nil || len(panel) != 1 || panel[0].Reference != card.Reference {
		t.Fatal("current recorded panel context denied", err)
	}
	if _, err = f.s.Panel(ctx, f.second.ID, f.event, f.team, id); err == nil {
		t.Fatal("unrecorded actor obtained panel context")
	}
	cmd.Action = "save"
	cmd.Version = 1
	cmd.Scores = nil
	if _, err = f.s.Change(ctx, f.primary.ID, f.event, f.team, f.revision(), cmd); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Panel(
		ctx,
		f.owner.ID,
		f.event,
		f.team,
		id,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("huddle exposed a newer independent draft or stale submitted reference", err)
	}
	f.exec(
		"INSERT INTO mentor_conflicts(event_id,mentor_id,competitor_id,kind) VALUES($1,$2,$3,'reporting')",
		f.event,
		f.primary.ID,
		f.member.ID,
	)
	if _, err = f.s.History(
		ctx,
		f.primary.ID,
		f.event,
		f.team,
		card.Reference.ID,
		nil,
		0,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("new conflict retained author history", err)
	}
	f.exec("DELETE FROM mentor_conflicts WHERE event_id=$1", f.event)
	f.exec("UPDATE events SET writes_allowed=false,stage='archived' WHERE id=$1", f.event)
	if _, err = f.s.Change(
		ctx,
		f.primary.ID,
		f.event,
		f.team,
		f.revision(),
		cmd,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("archive allowed score edits", err)
	}
	if page, err := f.s.History(
		ctx,
		f.primary.ID,
		f.event,
		f.team,
		card.Reference.ID,
		nil,
		0,
	); err != nil ||
		len(page.Items) != 2 {
		t.Fatal("archive discarded currently authorised private history", err)
	}
	f.exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mentor'",
		f.event,
		f.primary.ID,
	)
	if _, err = f.s.Read(
		ctx,
		f.primary.ID,
		f.event,
		f.team,
		uuid.Nil,
		0,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("revoked author retained private scores", err)
	}
}

func TestConfiguredWindowAndEvidenceReferencesCannotBypassNativeOwnership(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	cmd := f.command(f.checkpoint, "submit", true)
	missing := uuid.New()
	version := int64(1)
	cmd.Scores[0].Evidence = []EvidenceReference{
		{Kind: "observation", ID: &missing, Version: &version},
	}
	if _, err := f.s.Change(
		ctx,
		f.primary.ID,
		f.event,
		f.team,
		f.revision(),
		cmd,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("invented evidence reference accepted", err)
	}
	cmd = f.command(f.checkpoint, "submit", true)
	cmd.Scores[0].Evidence = nil
	if _, err := f.s.Change(
		ctx,
		f.primary.ID,
		f.event,
		f.team,
		f.revision(),
		cmd,
	); !errors.Is(
		err,
		access.ErrInvalid,
	) {
		t.Fatal("scored submitted category lacked evidence", err)
	}
	cmd.Action = "save"
	if _, err := f.s.Change(ctx, f.primary.ID, f.event, f.team, f.revision(), cmd); err != nil {
		t.Fatal("incomplete evidence draft discarded", err)
	}
	f.config.LiveStart = f.config.LiveStart.Add(time.Hour)
	raw, _ := json.Marshal(f.config)
	f.exec("UPDATE events SET configuration=$2 WHERE id=$1", f.event, raw)
	if _, err := f.s.Change(ctx, f.primary.ID, f.event, f.team, f.revision(), cmd); err == nil {
		t.Fatal("postponed live start retained score entry")
	}
	cp := f.checkpoint
	cp.Definition.OpensWallTime = "2027-03-28T01:30:00"
	cp.Definition.ClosesWallTime = "2027-03-28T03:00:00"
	if _, err := f.s.Configure(
		ctx,
		f.owner.ID,
		f.event,
		f.revision(),
		CheckpointCommand{
			Definition: cp.Definition,
			Version:    cp.Version,
			Reason:     "Reject nonexistent London clock",
		},
	); err == nil {
		t.Fatal("DST gap silently resolved")
	}
}

func TestScoreWriterWaitingBehindAssignmentRemovalRechecksAfterEventLock(t *testing.T) {
	f := setup(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	revision := f.revision()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	q := dbgen.New(tx)
	if _, err = q.LockEvent(ctx, f.event); err != nil {
		t.Fatal(err)
	}
	var holder int32
	if err = tx.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&holder); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(
		ctx,
		"UPDATE mentor_assignments SET snapshot='draft' WHERE event_id=$1 AND person_id=$2",
		f.event,
		f.primary.ID,
	); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := f.s.Change(
			ctx,
			f.primary.ID,
			f.event,
			f.team,
			revision,
			f.command(f.checkpoint, "submit", true),
		)
		done <- err
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err = f.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))", holder).
			Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err := <-done:
			t.Fatal("writer bypassed event lock", err)
		case <-ctx.Done():
			t.Fatal("writer never reached event lock")
		case <-ticker.C:
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if !errors.Is(err, access.ErrForbidden) {
			t.Fatal("removed assignment wrote scores", err)
		}
	case <-ctx.Done():
		t.Fatal("writer failed to finish")
	}
	var count int
	if err = f.pool.QueryRow(ctx, "SELECT count(*) FROM mentor_scorecard_versions").
		Scan(&count); err != nil ||
		count != 0 {
		t.Fatal("revoked writer retained score", count, err)
	}
}
