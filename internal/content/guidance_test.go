package content_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestGuidanceCurrentReleasedRulesAndPersonalReceipts(t *testing.T) {
	pool := testdb.New(t, true)
	f := newFixture(t, pool)
	ctx := t.Context()
	in := input("page")
	in.Document.Purpose = "rules"
	in.Document.RequireAcknowledgement = true
	p := f.act(f.approve(f.save(content.Page{}, in)), "release")
	first, err := f.s.Guidance(ctx, f.member.ID, f.id)
	if err != nil || len(first.Items) != 1 || len(first.Warnings) != 0 {
		t.Fatalf("participant guidance: %+v %v", first, err)
	}
	if _, err = f.s.Acknowledge(ctx, f.member.ID, f.id, p.ID, p.Version); err != nil {
		t.Fatal(err)
	}
	in.Document.Markdown = "Corrected rules, still private"
	draft := f.save(p, in)
	old, err := f.s.Guidance(ctx, f.member.ID, f.id)
	if err != nil || len(old.Items) != 1 || old.Items[0].Version != p.Version ||
		old.Items[0].AcknowledgedAt == nil {
		t.Fatalf("private draft replaced guidance: %+v %v", old, err)
	}
	released := f.act(f.approve(draft), "release")
	current, err := f.s.Read(ctx, f.member.ID, f.id, p.ID, false, 0)
	if err != nil || current.Version != released.Version || current.AcknowledgedAt != nil ||
		current.PreviousAcknowledgement == nil ||
		current.PreviousAcknowledgement.Version != p.Version {
		t.Fatalf("corrected receipt: %+v %v", current, err)
	}
	owner, err := f.s.Read(ctx, f.owner.ID, f.id, p.ID, false, 0)
	if err != nil || owner.PreviousAcknowledgement != nil {
		t.Fatalf("another person's receipt exposed: %+v %v", owner, err)
	}
	second := newFixture(t, pool)
	if _, err = f.s.Read(ctx, f.member.ID, second.id, p.ID, false, 0); err == nil {
		t.Fatal("cross-event rules exposed")
	}
	f.act(released, "withdraw")
	hidden, err := f.s.Guidance(ctx, f.member.ID, f.id)
	if err != nil || len(hidden.Items) != 0 {
		t.Fatalf("withdrawn guidance: %+v %v", hidden, err)
	}
}

func TestGuidanceContactsRequireRealConfigurationAndCurrentAudience(t *testing.T) {
	pool := testdb.New(t, true)
	f := newFixture(t, pool)
	ctx := t.Context()
	before := f.row()
	g, err := f.s.Guidance(ctx, f.owner.ID, f.id)
	if err != nil || len(g.Warnings) != 2 || !g.CanManage {
		t.Fatalf("missing readiness warnings: %+v %v", g, err)
	}
	if after := f.row(); after.Revision != before.Revision || after.Stage != before.Stage {
		t.Fatal("readiness warning mutated lifecycle")
	}
	for _, purpose := range []string{"support", "conduct"} {
		in := input("page")
		in.Document.Purpose = purpose
		in.Document.Contacts = []content.Contact{
			{Label: "{{Named contact}}", URL: "{{Approved contact route}}"},
		}
		draft := f.act(f.save(content.Page{}, in), "review")
		if _, err = f.s.Action(
			ctx,
			f.owner.ID,
			f.id,
			draft.ID,
			draft.Revision,
			"approve",
			"",
		); !errors.Is(
			err,
			event.ErrBlocked,
		) {
			t.Fatalf("placeholder approved: %v", err)
		}
		in.Document.Contacts = []content.Contact{
			{Label: "Synthetic event team", URL: "mailto:support@example.test"},
		}
		p := f.act(f.approve(f.save(draft, in)), "release")
		if purpose == "conduct" {
			in.Document.Audience.Confirmed = false
			in.Document.Audience.Roles = []string{"organiser"}
			p = f.act(f.approve(f.save(p, in)), "release")
		}
	}
	participant, err := f.s.Guidance(ctx, f.member.ID, f.id)
	if err != nil || len(participant.Items) != 1 || participant.Items[0].Purpose != "support" ||
		len(participant.Warnings) != 0 {
		t.Fatalf("private conduct leaked: %+v %v", participant, err)
	}
	organiser, err := f.s.Guidance(ctx, f.owner.ID, f.id)
	if err != nil || len(organiser.Items) != 2 || len(organiser.Warnings) != 1 ||
		organiser.Warnings[0].Code != "missing_conduct_contact" {
		t.Fatalf("audience readiness: %+v %v", organiser, err)
	}
	f.exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		f.id,
		f.member.ID,
	)
	if _, err = f.s.Guidance(ctx, f.member.ID, f.id); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("revoked access: %v", err)
	}
	for _, url := range []string{"javascript:alert(1)", "data:text/html,bad", "https://user:secret@example.test", "mailto:team@example.test?body=private", "mailto:a@example.test,b@example.test", "mailto:team%0d%0aBcc:other@example.test", "mailto:team%0a@example.test"} {
		in := input("page")
		in.Document.Purpose = "conduct"
		in.Document.Contacts = []content.Contact{{Label: "Unsafe", URL: url}}
		if _, err = f.s.Save(
			ctx,
			f.owner.ID,
			f.id,
			uuid.New(),
			0,
			in,
		); !errors.Is(
			err,
			access.ErrInvalid,
		) {
			t.Fatalf("unsafe contact %q: %v", url, err)
		}
	}
	in := input("challenge")
	in.Document.Purpose = "rules"
	if _, err = f.s.Save(
		ctx,
		f.owner.ID,
		f.id,
		uuid.New(),
		0,
		in,
	); !errors.Is(
		err,
		access.ErrInvalid,
	) {
		t.Fatalf("challenge relabelled guidance: %v", err)
	}
}
