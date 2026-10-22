package automation

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func (f *fixture) approvedBrief() (content.Service, content.Page) {
	f.t.Helper()
	f.exec("UPDATE events SET published_at=$2 WHERE id=$1", f.id, f.now.Add(-time.Hour))
	service := content.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	at := f.now.Add(time.Minute)
	page, err := service.Save(
		f.t.Context(),
		f.owner.ID,
		f.id,
		uuid.New(),
		0,
		content.Input{
			Kind: "brief",
			Document: content.Document{
				Title:    "PRIVATE-SOURCE-TITLE-CANARY",
				Markdown: "Synthetic approved body",
				Audience: access.ContentAudience{
					Confirmed: true,
					Roles:     []string{},
					Teams:     []uuid.UUID{},
				},
				Attachments: []uuid.UUID{},
				ScheduledAt: &at,
			},
		},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, action := range []string{"review", "approve"} {
		page, err = service.Action(
			f.t.Context(),
			f.owner.ID,
			f.id,
			page.ID,
			page.Revision,
			action,
			"Explicit source approval",
		)
		if err != nil {
			f.t.Fatal(err)
		}
	}
	return service, page
}

func (f *fixture) publicationBinding(source content.Page) Binding {
	f.t.Helper()
	id := uuid.New()
	definition := Definition{
		ID:                     id,
		CampaignID:             f.campaignID,
		CampaignVersion:        f.campaignVersion,
		Trigger:                "brief.released",
		SourceID:               &source.ID,
		SourceVersion:          source.Version,
		MaximumLatenessSeconds: 60,
	}
	page, _ := f.change(
		Command{Action: "save", Definition: &definition, Reason: "Bind exact approved publication"},
	)
	_, preview := f.change(
		Command{
			Action:    "preview_publication",
			BindingID: id,
			Version:   page.Bindings[0].Version,
			Reason:    "Review deferred source variables",
		},
	)
	if preview == nil || len(preview.DeferredVariables) != 1 ||
		preview.DeferredVariables[0] != "brief.title" ||
		preview.GenericCampaignApproved {
		f.t.Fatal("deferred preview misrepresented generic approval")
	}
	raw, _ := json.Marshal(preview)
	if strings.Contains(string(raw), "PRIVATE-SOURCE-TITLE-CANARY") {
		f.t.Fatal("source title leaked into deferred recipient preview")
	}
	page, _ = f.change(
		Command{
			Action:        "approve_publication",
			BindingID:     id,
			Version:       page.Bindings[0].Version,
			PreviewPlanID: preview.PlanID,
			Reason:        "Explicitly approve the expected guarded transition",
		},
	)
	return page.Bindings[0]
}

func TestExpectedPublicationPlanRequiresActualReleaseAndPreservesGenericDraft(t *testing.T) {
	f := newFixture(t)
	service, source := f.approvedBrief()
	f.campaign("brief_release", "Read {{brief.title}}, {{person.name}}", false)
	f.publicationBinding(source)
	if page, readErr := service.Read(t.Context(), f.person.ID, f.id, source.ID, false, 0); readErr == nil || strings.Contains(page.Document.Title, "PRIVATE-SOURCE-TITLE-CANARY") {
		t.Fatal("typed plan granted an attendee early source access")
	}
	detail, err := f.s.Composer.Campaign(t.Context(), f.owner.ID, f.id, f.campaignID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Campaign.Campaign.Approval != nil || detail.Campaign.Status != "draft" {
		t.Fatal("typed plan forged a generic campaign approval")
	}
	f.now = f.now.Add(time.Minute)
	if len(f.maintain().Occurrences) != 0 || f.count() != 0 {
		t.Fatal("mere planned clock time emitted a release notification")
	}
	if err = service.Dispatch(t.Context(), f.id, source.ID, source.Revision); err != nil {
		t.Fatal(err)
	}
	page := f.maintain()
	if len(page.Occurrences) != 1 {
		t.Fatal("actual released source did not create one occurrence")
	}
	f.execute(page.Occurrences[0].ID)
	if f.count() != 1 {
		t.Fatalf("typed source transition did not queue current reader: %+v", f.maintain())
	}
	// Independent generic execution still refuses the unapproved campaign.
	if err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		_, e := f.s.Composer.ResolveApprovedCampaignInTransaction(
			t.Context(),
			tx,
			f.id,
			f.campaignID,
		)
		return e
	}); err == nil {
		t.Fatal("generic approval path bypassed")
	}
}

func TestChangedExpectedPublicationPlanAndUnknownVariablesBlock(t *testing.T) {
	t.Run("unknown placeholder", func(t *testing.T) {
		f := newFixture(t)
		f.approvedBrief()
		f.campaign("brief_release", "Read {{brief.title}}", false)
		sources, err := f.s.Composer.Read(t.Context(), f.owner.ID, f.id)
		if err != nil {
			t.Fatal(err)
		}
		template := sources.Templates[0]
		template.Body = "Read {{brief.title}} {{unknown.secret}}"
		if _, err = f.s.Composer.Change(
			t.Context(),
			f.owner.ID,
			f.id,
			f.revision(),
			communications.Command{
				Action:   "save_template",
				Template: &template,
				Reason:   "Unknown facts cannot enter an approved plan",
			},
		); err == nil {
			t.Fatal("unknown placeholder saved")
		}
		var plans int
		if err = f.pool.QueryRow(t.Context(), "SELECT count(*) FROM communication_publication_approvals WHERE event_id=$1", f.id).
			Scan(&plans); err != nil ||
			plans != 0 {
			t.Fatal("unknown facts became a plan", err)
		}
	})

	t.Run("changed template", func(t *testing.T) {
		f := newFixture(t)
		service, source := f.approvedBrief()
		f.campaign("brief_release", "Read {{brief.title}}", false)
		f.publicationBinding(source)
		sources, err := f.s.Composer.Read(t.Context(), f.owner.ID, f.id)
		if err != nil {
			t.Fatal(err)
		}
		template := sources.Templates[0]
		template.Body = "Changed unreviewed message {{brief.title}}"
		if _, err = f.s.Composer.Change(
			t.Context(),
			f.owner.ID,
			f.id,
			f.revision(),
			communications.Command{
				Action:   "save_template",
				Template: &template,
				Reason:   "Change body after typed approval",
			},
		); err != nil {
			t.Fatal(err)
		}
		f.now = f.now.Add(time.Minute)
		if err = service.Dispatch(t.Context(), f.id, source.ID, source.Revision); err != nil {
			t.Fatal(err)
		}
		page := f.maintain()
		f.execute(page.Occurrences[0].ID)
		if f.count() != 0 {
			t.Fatal("changed source message bypassed plan approval")
		}
	})
}
