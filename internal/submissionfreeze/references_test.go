package submissionfreeze

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestManagedReferenceIdentityDoesNotInferHistoricalSHA(t *testing.T) {
	canonical := "https://github.com/synthetic-org/managed"
	for _, tc := range []struct {
		url  string
		want bool
	}{{canonical, true}, {canonical + ".git", true}, {canonical + "/", true}, {"https://github.com/SYNTHETIC-ORG/MANAGED", true}, {canonical + "/commit/" + strings.Repeat("a", 40), true}, {canonical + "/tree/release/build", true}, {canonical + "-other", false}, {"https://github.com.evil.test/synthetic-org/managed", false}, {"https://evil.test/synthetic-org/managed", false}, {"http://github.com/synthetic-org/managed", false}, {"https://person:token@github.com/synthetic-org/managed", false}, {canonical + "?redirect=elsewhere", false}, {canonical + "/commit/not-a-sha", false}, {canonical + "/issues/1", false}} {
		if got := managedReference(tc.url, canonical); got != tc.want {
			t.Errorf("%s: got %t", tc.url, got)
		}
	}
}

func TestUnmanagedReferenceNeedsNativeCorrectionAndExplicitAmendment(t *testing.T) {
	pool := testdb.New(t, true)
	f := newFixture(t, pool)
	s := native(*f.service)
	n, err := s.Read(t.Context(), f.member, f.event, f.team)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Submit(
		t.Context(),
		f.member,
		f.event,
		f.team,
		n.Revision,
		submissions.Command{
			Evidence: []submissions.Evidence{
				{
					DeliverableID: f.deliverable,
					Kind:          "repository",
					Label:         "Permitted external reference",
					URL:           "https://example.test/external-repository",
				},
			},
		},
	)
	if err != nil {
		t.Fatal("native accepted reference contract narrowed", err)
	}
	f.now = f.cfg.SubmissionCutoff
	f.capture()
	p := f.read(f.owner)
	if p.Status != "repository_reference_mismatch" || len(p.Source.ManagedEvidenceIDs) != 0 ||
		len(p.Source.UncapturedEvidenceIDs) != 1 ||
		p.CanReconcile ||
		p.CanAmend ||
		len(p.Observations) != 1 {
		t.Fatal("managed code silently claimed external submitted reference", p)
	}
	sourceID := p.Source.ID
	original := *p.Source.SubmissionRevision
	c := f.decision(p, "reconcile")
	if _, err = f.service.Change(t.Context(), f.owner, f.event, f.team, c); err == nil {
		t.Fatal("review alone fabricated reference equivalence")
	}
	n, err = s.Read(t.Context(), f.owner, f.event, f.team)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Submit(
		t.Context(),
		f.owner,
		f.event,
		f.team,
		n.Revision,
		submissions.Command{
			Reason: "Correct native final reference explicitly",
			Evidence: []submissions.Evidence{
				{
					DeliverableID: f.deliverable,
					Kind:          "repository",
					Label:         "Corrected managed reference",
					URL:           p.RepositoryURL,
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	p = f.read(f.owner)
	if !p.CanAmend || p.CanReconcile {
		t.Fatal(p)
	}
	ack, err := f.service.Change(t.Context(), f.owner, f.event, f.team, f.decision(p, "amend"))
	if err != nil {
		t.Fatal(err)
	}
	p = f.read(f.owner)
	if !p.BindingCurrent || p.Status != "completed" || p.Source.ID != sourceID ||
		*p.Source.SubmissionRevision != original ||
		len(p.Source.ManagedEvidenceIDs) != 0 ||
		len(ack.Decision.ManagedEvidenceIDs) != 1 {
		t.Fatal("correction erased original reference exception", p)
	}
}

func TestAdditionalExternalReferencesAndPrivateOriginalHistoryStayScoped(t *testing.T) {
	pool := testdb.New(t, true)
	f := newFixture(t, pool)
	extra := uuid.New()
	defs := templates.EmptyDefinitions()
	defs.Deliverables = []templates.Deliverable{
		{
			ID:            f.deliverable,
			Title:         "Private original repository",
			Required:      true,
			EvidenceKinds: []string{"repository"},
			Audience:      []string{"organiser"},
		},
		{
			ID:            extra,
			Title:         "Other reference",
			EvidenceKinds: []string{"repository"},
			Audience:      []string{"organiser"},
		},
	}
	raw, _ := json.Marshal(defs)
	f.exec("UPDATE event_definitions SET definitions=$2 WHERE event_id=$1", f.event, raw)
	s := native(*f.service)
	n, err := s.Read(t.Context(), f.member, f.event, f.team)
	if err != nil {
		t.Fatal(err)
	}
	url := "https://github.com/synthetic-org/freeze-" + f.team.String()
	_, err = s.Submit(
		t.Context(),
		f.member,
		f.event,
		f.team,
		n.Revision,
		submissions.Command{
			Evidence: []submissions.Evidence{
				{DeliverableID: f.deliverable, Kind: "repository", Label: "Managed", URL: url},
				{
					DeliverableID: extra,
					Kind:          "repository",
					Label:         "Other permitted reference",
					URL:           "https://example.test/uncaptured",
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.now = f.cfg.SubmissionCutoff
	f.capture()
	p := f.read(f.owner)
	first, err := f.service.Change(
		t.Context(),
		f.owner,
		f.event,
		f.team,
		f.decision(p, "reconcile"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Source.ManagedEvidenceIDs) != 1 || len(p.Source.UncapturedEvidenceIDs) != 1 {
		t.Fatal(p)
	}
	mentor := testdb.Person(t, pool).ID
	f.exec(
		"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mentor')",
		f.event,
		mentor,
	)
	f.exec("INSERT INTO policy_resources(event_id,id,kind) VALUES($1,$2,'team')", f.event, f.team)
	f.exec(
		"INSERT INTO resource_assignments(event_id,resource_id,person_id) VALUES($1,$2,$3)",
		f.event,
		f.team,
		mentor,
	)
	if _, err = f.service.Read(t.Context(), mentor, f.event, f.team); err == nil {
		t.Fatal("mentor read private original repository receipt")
	}
	defs.Deliverables[0].Audience = []string{"mentor", "judge"}
	raw, _ = json.Marshal(defs)
	f.exec("UPDATE event_definitions SET definitions=$2 WHERE event_id=$1", f.event, raw)
	n, err = s.Read(t.Context(), f.owner, f.event, f.team)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Submit(
		t.Context(),
		f.owner,
		f.event,
		f.team,
		n.Revision,
		submissions.Command{
			Reason: "Reviewed new final with assessor-visible managed reference",
			Evidence: []submissions.Evidence{
				{
					DeliverableID: f.deliverable,
					Kind:          "repository",
					Label:         "New visible final",
					URL:           url,
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	p = f.read(f.owner)
	c := f.decision(p, "amend")
	c.SHA = strings.Repeat("b", 40)
	c.ObservationIDs = nil
	if _, err = f.service.Change(t.Context(), f.owner, f.event, f.team, c); err != nil {
		t.Fatal(err)
	}
	visible := f.read(mentor)
	encoded, _ := json.Marshal(visible)
	if visible.Current == nil || visible.Current.SHA != strings.Repeat("b", 40) ||
		len(visible.Decisions) != 1 ||
		visible.Decisions[0].ID == first.Decision.ID ||
		len(visible.Observations) != 0 ||
		visible.Source.SubmissionRevision != nil ||
		len(visible.Source.UncapturedEvidenceIDs) != 0 ||
		strings.Contains(string(encoded), strings.Repeat("a", 40)) ||
		strings.Contains(string(encoded), "private decision evidence") {
		t.Fatal("private historical evidence leaked through new public final", string(encoded))
	}
}
