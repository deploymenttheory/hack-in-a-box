package app

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/presentation"
)

func judgeOrderHTTP(t *testing.T, f judgeBrowserEdition, actor uuid.UUID) (int, []byte) {
	t.Helper()
	person, err := dbgen.New(f.runtime.pool).Person(t.Context(), actor)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		f.surface.server.URL+"/api/v1/events/"+f.runtime.eventID.String()+"/judging/presentation-order",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(f.surface.cookie(t, person))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, raw
}

func TestJudgePublishedOrderUsesNativeReleaseAndAssignmentAuthority(t *testing.T) {
	f := newJudgeBrowserEdition(t, "admin", false)
	r := f.runtime
	ctx := t.Context()
	common := content.Service{Pool: r.pool, Clock: r.clock.read}
	hidden, err := common.Save(
		ctx,
		r.owner.ID,
		r.eventID,
		uuid.New(),
		0,
		content.Input{
			Kind: "page",
			Document: content.Document{
				Title:    "PRIVATE-UNRELEASED-RESOURCE",
				Markdown: "Private resource body",
				Audience: access.ContentAudience{Confirmed: true},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"review", "approve"} {
		hidden, err = common.Action(
			ctx,
			r.owner.ID,
			r.eventID,
			hidden.ID,
			hidden.Revision,
			action,
			"Synthetic native release test",
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	cues := []presentation.Cue{
		{
			ID:              uuid.New(),
			Title:           "Published opening",
			Text:            "PRIVATE-PUBLISHED-CUE-TEXT",
			URL:             "https://example.test/private-cue-reference",
			DurationSeconds: 60,
		},
		{
			ID:              uuid.New(),
			Title:           "PRIVATE-UNRELEASED-CUE-TITLE",
			ContentID:       &hidden.ID,
			DurationSeconds: 60,
		},
		{
			ID:              uuid.New(),
			Title:           "Released challenge presentation",
			ContentID:       &r.challengeID,
			DurationSeconds: 60,
		},
	}
	page, err := f.surface.deps.Presentation.Change(
		ctx,
		r.mc.ID,
		r.eventID,
		0,
		presentation.Command{Action: "save", Cues: cues, Reason: "Native public order source"},
	)
	if err != nil {
		t.Fatal(err)
	}
	zero := 0
	page, err = f.surface.deps.Presentation.Change(
		ctx,
		r.mc.ID,
		r.eventID,
		page.Revision,
		presentation.Command{
			Action:    "activate",
			FromDraft: true,
			Index:     &zero,
			Reason:    "Publish actual room programme",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	page, err = f.surface.deps.Presentation.Change(
		ctx,
		r.mc.ID,
		r.eventID,
		page.Revision,
		presentation.Command{
			Action: "save",
			Cues: []presentation.Cue{
				{ID: uuid.New(), Title: "PRIVATE-UNPUBLISHED-DRAFT", DurationSeconds: 60},
			},
			Reason: "Retain a distinct private draft",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	status, raw := judgeOrderHTTP(t, f, f.first.ID)
	if status != 200 {
		t.Fatal("actual native judge order route", status, string(raw))
	}
	for _, forbidden := range []string{"PRIVATE-", "private-cue-reference", "\"text\"", "\"url\"", "content_id", "can_control", "pairing", "draft"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatal("public order exposed private/controller content", string(raw))
		}
	}
	var order presentation.JudgeOrder
	if err = json.Unmarshal(raw, &order); err != nil {
		t.Fatal(err)
	}
	if len(order.Items) != 2 || order.Items[0].Ordinal != 1 || !order.Items[0].Current ||
		order.Items[1].Ordinal != 2 ||
		order.Items[1].Title != "Released challenge presentation" {
		t.Fatal("native released programme order/compact ordinals", order)
	}
	if status, _ = judgeOrderHTTP(t, f, r.competitor.ID); status != 403 {
		t.Fatal("competitor gained assigned-judge order", status)
	}
	if _, err = r.pool.Exec(
		ctx,
		"INSERT INTO mentor_conflicts(event_id,mentor_id,competitor_id,kind)VALUES($1,$2,$3,'reporting')",
		r.eventID,
		f.first.ID,
		r.competitor.ID,
	); err != nil {
		t.Fatal(err)
	}
	if status, _ = judgeOrderHTTP(t, f, f.first.ID); status != 403 {
		t.Fatal("declared conflict retained order read", status)
	}
	if _, err = r.pool.Exec(
		ctx,
		"DELETE FROM mentor_conflicts WHERE event_id=$1 AND mentor_id=$2",
		r.eventID,
		f.first.ID,
	); err != nil {
		t.Fatal(err)
	}
	if status, _ = judgeOrderHTTP(t, f, r.owner.ID); status != 200 {
		t.Fatal("organiser current order denied", status)
	}
	if _, err = (presentation.Service{Pool: r.pool, Clock: r.clock.read}).ReadJudgeOrder(
		ctx,
		f.first.ID,
		r.eventID,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("unwired actual assignment owner did not fail closed", err)
	}
	override := presentation.Cue{
		ID:              uuid.New(),
		Title:           "Temporary public room notice",
		Text:            "PRIVATE-OVERRIDE-TEXT",
		DurationSeconds: 30,
	}
	page, err = f.surface.deps.Presentation.Change(
		ctx,
		r.mc.ID,
		r.eventID,
		page.Revision,
		presentation.Command{
			Action:   "override",
			Override: &override,
			Reason:   "Explicit native room override",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	order, err = f.surface.deps.Presentation.ReadJudgeOrder(ctx, f.first.ID, r.eventID)
	if err != nil || !order.OverrideActive {
		t.Fatal("native override observation missing", order, err)
	}
	for _, item := range order.Items {
		if item.Current {
			t.Fatal("published cue falsely labelled current during override", item)
		}
	}
	if _, err = common.Action(
		ctx,
		r.owner.ID,
		r.eventID,
		hidden.ID,
		hidden.Revision,
		"release",
		"Explicit current public release",
	); err != nil {
		t.Fatal(err)
	}
	order, err = f.surface.deps.Presentation.ReadJudgeOrder(ctx, f.first.ID, r.eventID)
	if err != nil || len(order.Items) != 3 || order.Items[1].Ordinal != 2 ||
		order.Items[1].Title != "PRIVATE-UNRELEASED-CUE-TITLE" {
		t.Fatal("actual public release did not update order", order, err)
	}
	if _, err = r.pool.Exec(
		ctx,
		"UPDATE judge_assignments SET active=false WHERE event_id=$1 AND person_id=$2",
		r.eventID,
		f.first.ID,
	); err != nil {
		t.Fatal(err)
	}
	if status, _ = judgeOrderHTTP(t, f, f.first.ID); status != 403 {
		t.Fatal("removed assignment retained order read", status)
	}
	if _, err = r.pool.Exec(
		ctx,
		"UPDATE judge_assignments SET active=true WHERE event_id=$1 AND person_id=$2",
		r.eventID,
		f.first.ID,
	); err != nil {
		t.Fatal(err)
	}
	if err = dbgen.New(r.pool).
		SetMembership(ctx, dbgen.SetMembershipParams{EventID: r.eventID, PersonID: f.first.ID, Role: "judge", Active: false}); err != nil {
		t.Fatal(err)
	}
	if status, _ = judgeOrderHTTP(t, f, f.first.ID); status != 403 {
		t.Fatal("removed judge role retained order read", status)
	}
	if _, err = r.pool.Exec(
		ctx,
		"UPDATE events SET writes_allowed=false,stage='archived' WHERE id=$1",
		r.eventID,
	); err != nil {
		t.Fatal(err)
	}
	order, err = f.surface.deps.Presentation.ReadJudgeOrder(ctx, r.owner.ID, r.eventID)
	if err != nil || len(order.Items) != 0 {
		t.Fatal("archive revived operational programme", order, err)
	}
}
