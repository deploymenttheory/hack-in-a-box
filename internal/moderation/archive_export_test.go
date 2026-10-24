package moderation

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/archives"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

func archivePanelFixture(t *testing.T, f *fixture) {
	t.Helper()
	life := event.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	cancelled, err := life.Transition(
		t.Context(),
		f.owner.ID,
		f.event,
		f.revision(),
		"cancelled",
		"Synthetic archive with recorded panel",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = life.Transition(
		t.Context(),
		f.owner.ID,
		f.event,
		cancelled.Revision,
		"archived",
		"",
		false,
	); err != nil {
		t.Fatal(err)
	}
}

func TestArchivedAssessmentExportUsesActualRecordedHuddleAndCurrentAuthors(t *testing.T) {
	f, native, h := finalContext(t, true)
	cards := f.s
	cards.Moderation = native
	service := archives.Service{
		Pool:       f.pool,
		Clock:      func() time.Time { return f.now },
		Scorecards: cards,
	}
	archivePanelFixture(t, f)
	request := archives.ExportRequest{
		Source:     "assessments",
		ResourceID: f.team,
		ContextID:  h.Definition.ID,
		Limit:      100,
	}
	result, err := service.Export(t.Context(), f.owner.ID, f.event, request)
	if err != nil {
		t.Fatal(err)
	}
	if result.AssessmentPanel == nil || result.Assessments != nil ||
		len(result.AssessmentPanel.Cards) != 2 ||
		result.AssessmentPanel.Context.ID != h.Definition.ID ||
		result.AssessmentPanel.Context.Version != h.Version {
		t.Fatal("lost exact recorded panel provenance", result)
	}
	for i, card := range result.AssessmentPanel.Cards {
		if card.Reference != result.AssessmentPanel.Context.Cards[i] || card.State != "submitted" {
			t.Fatal("panel admitted draft or another submitted version", card)
		}
	}
	var audited string
	if err = f.pool.QueryRow(t.Context(), "SELECT metadata->>'reference_id' FROM audit_events WHERE event_id=$1 AND action='archive.exported' ORDER BY occurred_at DESC LIMIT 1", f.event).
		Scan(&audited); err != nil ||
		audited != h.Definition.ID.String() {
		t.Fatal("selected recorded context missing from safe export audit", audited, err)
	}
	request.ContextID = uuid.Nil
	own, err := service.Export(t.Context(), f.primary.ID, f.event, request)
	if err != nil || own.Assessments == nil || len(own.Assessments.Items) != 1 ||
		own.Assessments.Items[0].AuthorID != f.primary.ID {
		t.Fatal("own export widened to peer cards", own, err)
	}
	request.ContextID = uuid.New()
	if _, err = service.Export(
		t.Context(),
		f.owner.ID,
		f.event,
		request,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("unknown context granted panel", err)
	}
	request.ContextID = h.Definition.ID
	f.exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mentor'",
		f.event,
		f.primary.ID,
	)
	if _, err = service.Export(t.Context(), f.owner.ID, f.event, request); err == nil {
		t.Fatal("caller borrowed broader permissions after original author lost grant")
	}
	f.exec(
		"UPDATE event_memberships SET active=true WHERE event_id=$1 AND person_id=$2 AND role='mentor'",
		f.event,
		f.primary.ID,
	)
	f.exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='organiser'",
		f.event,
		f.owner.ID,
	)
	if _, err = service.Export(
		t.Context(),
		f.owner.ID,
		f.event,
		request,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("lost caller role retained recorded panel", err)
	}
}

func TestArchivedAssessmentExportRejectsSupersededSubmittedContext(t *testing.T) {
	f, native, h := finalContext(t, true)
	current, err := native.Panel(t.Context(), f.owner.ID, f.event, f.team, h.Definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	command := f.command(h.Checkpoint, "submit", true)
	for _, card := range current {
		if card.AuthorID == f.primary.ID {
			command.Version = card.Reference.Version
		}
	}
	if _, err = f.s.Change(
		t.Context(),
		f.primary.ID,
		f.event,
		f.team,
		f.revision(),
		command,
	); err != nil {
		t.Fatal(err)
	}
	archivePanelFixture(t, f)
	cards := f.s
	cards.Moderation = native
	service := archives.Service{
		Pool:       f.pool,
		Clock:      func() time.Time { return f.now },
		Scorecards: cards,
	}
	if _, err = service.Export(
		t.Context(),
		f.owner.ID,
		f.event,
		archives.ExportRequest{
			Source:     "assessments",
			ResourceID: f.team,
			ContextID:  h.Definition.ID,
			Limit:      100,
		},
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("archival revived superseded submitted context", err)
	}
}
